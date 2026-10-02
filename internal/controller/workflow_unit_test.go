/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/workflow"
)

// ---------------------------------------------------------------------------
// 纯逻辑辅助函数：不依赖 envtest，覆盖调度判定的边界
// ---------------------------------------------------------------------------

func inlineSpec() *kubetaskv1.TaskSpec {
	return &kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: "busybox"}
}

func mustBuildGraph(t *testing.T, spec kubetaskv1.WorkflowSpec) *workflow.Graph {
	t.Helper()

	graph, err := workflow.Build(spec)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	return graph
}

func TestDecideNode(t *testing.T) {
	chained := []kubetaskv1.WorkflowTask{
		{Name: "build", TaskSpec: inlineSpec()},
		{Name: "test", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
	}
	runOnFailure := []kubetaskv1.WorkflowTask{
		{Name: "build", TaskSpec: inlineSpec()},
		{Name: "lint", TaskSpec: inlineSpec()},
		{Name: "notify", DependsOn: []string{"build", "lint"}, RunOnFailure: true, TaskSpec: inlineSpec()},
	}

	tests := []struct {
		name    string
		tasks   []kubetaskv1.WorkflowTask
		phases  map[string]kubetaskv1.WorkflowNodePhase
		node    string
		want    nodeAction
		wantMsg string
	}{
		{
			name:   "root node is always ready",
			tasks:  chained,
			phases: map[string]kubetaskv1.WorkflowNodePhase{},
			node:   "build",
			want:   nodeReady,
		},
		{
			name:   "dependent waits while dependency is not terminal",
			tasks:  chained,
			phases: map[string]kubetaskv1.WorkflowNodePhase{"build": kubetaskv1.NodeRunning},
			node:   "test",
			want:   nodeWait,
		},
		{
			name:   "dependent runs once dependency succeeded",
			tasks:  chained,
			phases: map[string]kubetaskv1.WorkflowNodePhase{"build": kubetaskv1.NodeSucceeded},
			node:   "test",
			want:   nodeReady,
		},
		{
			name:    "dependent is skipped when dependency failed",
			tasks:   chained,
			phases:  map[string]kubetaskv1.WorkflowNodePhase{"build": kubetaskv1.NodeFailed},
			node:    "test",
			want:    nodeSkip,
			wantMsg: `dependency "build" did not succeed`,
		},
		{
			name:  "dependent is skipped when dependency was skipped",
			tasks: chained,
			phases: map[string]kubetaskv1.WorkflowNodePhase{
				"build": kubetaskv1.NodeSkipped,
			},
			node:    "test",
			want:    nodeSkip,
			wantMsg: `dependency "build" did not succeed`,
		},
		{
			name:  "runOnFailure waits until every dependency is terminal",
			tasks: runOnFailure,
			phases: map[string]kubetaskv1.WorkflowNodePhase{
				"build": kubetaskv1.NodeFailed,
				"lint":  kubetaskv1.NodeRunning,
			},
			node: "notify",
			want: nodeWait,
		},
		{
			name:  "runOnFailure runs when a dependency failed",
			tasks: runOnFailure,
			phases: map[string]kubetaskv1.WorkflowNodePhase{
				"build": kubetaskv1.NodeFailed,
				"lint":  kubetaskv1.NodeSucceeded,
			},
			node: "notify",
			want: nodeReady,
		},
		{
			name:  "runOnFailure runs when a dependency was skipped",
			tasks: runOnFailure,
			phases: map[string]kubetaskv1.WorkflowNodePhase{
				"build": kubetaskv1.NodeSkipped,
				"lint":  kubetaskv1.NodeSucceeded,
			},
			node: "notify",
			want: nodeReady,
		},
		{
			name:  "runOnFailure is skipped when every dependency succeeded",
			tasks: runOnFailure,
			phases: map[string]kubetaskv1.WorkflowNodePhase{
				"build": kubetaskv1.NodeSucceeded,
				"lint":  kubetaskv1.NodeSucceeded,
			},
			node:    "notify",
			want:    nodeSkip,
			wantMsg: "all dependencies succeeded",
		},
		{
			name: "runOnFailure without dependencies never runs",
			tasks: []kubetaskv1.WorkflowTask{
				{Name: "notify", RunOnFailure: true, TaskSpec: inlineSpec()},
			},
			phases:  map[string]kubetaskv1.WorkflowNodePhase{},
			node:    "notify",
			want:    nodeSkip,
			wantMsg: "all dependencies succeeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := mustBuildGraph(t, kubetaskv1.WorkflowSpec{Tasks: tt.tasks})

			action, message := decideNode(graph, tt.phases, tt.node)
			if action != tt.want {
				t.Fatalf("action = %v, want %v", action, tt.want)
			}
			if message != tt.wantMsg {
				t.Fatalf("message = %q, want %q", message, tt.wantMsg)
			}
		})
	}
}

func TestValidateChildTaskNamesLengthBoundary(t *testing.T) {
	node := "build"
	limit := maxChildTaskNameLength - len(node) - 1 // 工作流名可用的最大长度

	tests := []struct {
		name       string
		workflow   string
		wantErr    bool
		wantSubstr string
	}{
		{
			name:     "exactly at the limit is accepted",
			workflow: strings.Repeat("a", limit),
			wantErr:  false,
		},
		{
			name:       "one character over the limit is rejected",
			workflow:   strings.Repeat("a", limit+1),
			wantErr:    true,
			wantSubstr: "child task names",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := kubetaskv1.WorkflowSpec{
				Tasks: []kubetaskv1.WorkflowTask{{Name: node, TaskSpec: inlineSpec()}},
			}
			graph := mustBuildGraph(t, spec)
			wf := &kubetaskv1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: tt.workflow}, Spec: spec}

			err := validateChildTaskNames(wf, graph)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

// 模板里的调度字段（type/schedule/delay/suspend）没有被 CEL 约束，
// Controller 必须在生成子 Task 时兜底清掉，否则会把 Cron/Delay 语义带进 Workflow。
func TestChildTaskSpecSanitizesTemplateFields(t *testing.T) {
	suspend := true
	delay := metav1.Duration{Duration: time.Minute}
	templateSpec := kubetaskv1.TaskSpec{
		Type:     kubetaskv1.TaskTypeCron,
		Schedule: "*/5 * * * *",
		Delay:    &delay,
		Suspend:  &suspend,
		Image:    "busybox",
		Command:  []string{"echo", "hello"},
	}
	spec := kubetaskv1.WorkflowSpec{
		Tasks:         []kubetaskv1.WorkflowTask{{Name: "build", Template: "cron-template"}},
		TaskTemplates: map[string]kubetaskv1.TaskSpec{"cron-template": templateSpec},
	}
	wf := &kubetaskv1.Workflow{Spec: spec}
	graph := mustBuildGraph(t, spec)

	got := childTaskSpec(wf, graph, "build")
	if got.Type != kubetaskv1.TaskTypeOneTime {
		t.Fatalf("type = %q, want OneTime", got.Type)
	}
	if got.Schedule != "" {
		t.Fatalf("schedule = %q, want empty", got.Schedule)
	}
	if got.Delay != nil {
		t.Fatalf("delay = %v, want nil", got.Delay)
	}
	if got.Suspend != nil {
		t.Fatalf("suspend = %v, want nil", got.Suspend)
	}
	if got.Image != "busybox" {
		t.Fatalf("image = %q, want busybox (其它字段应原样保留)", got.Image)
	}

	// 快照语义：修改返回值不能影响 Workflow spec（否则重复 reconcile 会互相污染）。
	got.Command[0] = "mutated"
	if wf.Spec.TaskTemplates["cron-template"].Command[0] != "echo" {
		t.Fatal("childTaskSpec 返回的 slice 与 spec 共享底层数组")
	}
}

func TestChildTaskSpecFromInlineTaskSpec(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{{
			Name:     "inline",
			TaskSpec: &kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: "alpine", Args: []string{"-c", "true"}},
		}},
	}
	wf := &kubetaskv1.Workflow{Spec: spec}
	graph := mustBuildGraph(t, spec)

	got := childTaskSpec(wf, graph, "inline")
	if got.Image != "alpine" || got.Type != kubetaskv1.TaskTypeOneTime {
		t.Fatalf("unexpected spec: %+v", got)
	}

	got.Args[0] = "mutated"
	if wf.Spec.Tasks[0].TaskSpec.Args[0] != "-c" {
		t.Fatal("childTaskSpec 返回的 slice 与 spec 共享底层数组")
	}
}

func TestChildTaskName(t *testing.T) {
	if got := childTaskName("ci-pipeline", "unit-test"); got != "ci-pipeline-unit-test" {
		t.Fatalf("childTaskName = %q", got)
	}
}

// initializeStatus 要把 status 对齐到当前 spec：进入 Running、清掉上一次失败留下的终态信息、
// 按拓扑序补齐节点，并丢弃已不在 spec 中的节点。
func TestInitializeStatus(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "test", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
		},
	}
	graph := mustBuildGraph(t, spec)

	finished := metav1.Now()
	wf := &kubetaskv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Generation: 3},
		Spec:       spec,
		Status: kubetaskv1.WorkflowStatus{
			Phase:          kubetaskv1.WorkflowFailed,
			Message:        "InvalidTask: boom",
			CompletionTime: &finished,
			Nodes: []kubetaskv1.WorkflowNodeStatus{
				{Name: "ghost", Phase: kubetaskv1.NodeSucceeded},
				{Name: "test", Phase: kubetaskv1.NodePending},
			},
		},
	}

	if !initializeStatus(wf, graph) {
		t.Fatal("initializeStatus should report a change")
	}
	if wf.Status.Phase != kubetaskv1.WorkflowRunning {
		t.Fatalf("phase = %q, want Running", wf.Status.Phase)
	}
	if wf.Status.Message != "" {
		t.Fatalf("message = %q, want empty", wf.Status.Message)
	}
	if wf.Status.CompletionTime != nil {
		t.Fatalf("completionTime = %v, want nil", wf.Status.CompletionTime)
	}
	if wf.Status.ObservedGeneration != 3 {
		t.Fatalf("observedGeneration = %d, want 3", wf.Status.ObservedGeneration)
	}
	if wf.Status.StartTime == nil {
		t.Fatal("startTime should be set")
	}

	want := []string{"build", "test"}
	if len(wf.Status.Nodes) != len(want) {
		t.Fatalf("nodes = %+v, want %v", wf.Status.Nodes, want)
	}
	for i, name := range want {
		if wf.Status.Nodes[i].Name != name {
			t.Fatalf("nodes[%d] = %q, want %q (拓扑序)", i, wf.Status.Nodes[i].Name, name)
		}
	}
	if wf.Status.Nodes[0].Phase != kubetaskv1.NodePending {
		t.Fatalf("新增节点应为 Pending, got %q", wf.Status.Nodes[0].Phase)
	}

	// 再跑一次应当是幂等的。
	if initializeStatus(wf, graph) {
		t.Fatal("initializeStatus should be idempotent for an already initialized status")
	}
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		name        string
		phase       kubetaskv1.WorkflowPhase
		nodes       []kubetaskv1.WorkflowNodeStatus
		wantPhase   kubetaskv1.WorkflowPhase
		wantChanged bool
	}{
		{
			name:        "keeps running while a node is pending",
			phase:       kubetaskv1.WorkflowRunning,
			nodes:       []kubetaskv1.WorkflowNodeStatus{{Name: "build", Phase: kubetaskv1.NodeRunning}, {Name: "test", Phase: kubetaskv1.NodePending}},
			wantPhase:   kubetaskv1.WorkflowRunning,
			wantChanged: false,
		},
		{
			name:        "succeeds when all nodes are done",
			phase:       kubetaskv1.WorkflowRunning,
			nodes:       []kubetaskv1.WorkflowNodeStatus{{Name: "build", Phase: kubetaskv1.NodeSucceeded}, {Name: "notify", Phase: kubetaskv1.NodeSkipped}},
			wantPhase:   kubetaskv1.WorkflowSucceeded,
			wantChanged: true,
		},
		{
			name:        "fails when any node failed",
			phase:       kubetaskv1.WorkflowRunning,
			nodes:       []kubetaskv1.WorkflowNodeStatus{{Name: "build", Phase: kubetaskv1.NodeFailed}, {Name: "notify", Phase: kubetaskv1.NodeSkipped}},
			wantPhase:   kubetaskv1.WorkflowFailed,
			wantChanged: true,
		},
		{
			name:        "terminal workflow is left untouched (一次性语义)",
			phase:       kubetaskv1.WorkflowSucceeded,
			nodes:       []kubetaskv1.WorkflowNodeStatus{{Name: "build", Phase: kubetaskv1.NodeSucceeded}},
			wantPhase:   kubetaskv1.WorkflowSucceeded,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &kubetaskv1.Workflow{Status: kubetaskv1.WorkflowStatus{Phase: tt.phase, Nodes: tt.nodes}}

			if got := summarize(wf); got != tt.wantChanged {
				t.Fatalf("changed = %v, want %v", got, tt.wantChanged)
			}
			if wf.Status.Phase != tt.wantPhase {
				t.Fatalf("phase = %q, want %q", wf.Status.Phase, tt.wantPhase)
			}
			if tt.wantChanged && wf.Status.CompletionTime == nil {
				t.Fatal("completionTime should be set for terminal results")
			}
			if tt.wantPhase == kubetaskv1.WorkflowFailed && !strings.Contains(wf.Status.Message, "build") {
				t.Fatalf("failure message should name the failed node, got %q", wf.Status.Message)
			}
		})
	}
}
