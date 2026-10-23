package workflow

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/naming"
)

const (
	testBuildNode     = "build"
	testBuildTemplate = "go-build"
	testDeployNode    = "deploy"
	testE2ENode       = "e2e"
	testGoImage       = "golang:1.27.3"
	testTestNode      = "test"
	testUnitNode      = "unit"
)

// requireReason 断言 err 是带指定 Reason 的 ValidationError。
func requireReason(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with reason %q, got nil", want)
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if verr.Reason != want {
		t.Fatalf("reason = %q, want %q (error: %v)", verr.Reason, want, err)
	}
}

// inlineSpec 返回一个合法的内联 TaskSpec，避免测试里重复样板。
func inlineSpec() *kubetaskv1.TaskSpec {
	return &kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: "busybox"}
}

func TestBuildRejectsEmptyTasks(t *testing.T) {
	_, err := Build(kubetaskv1.WorkflowSpec{})
	requireReason(t, err, ReasonNoTasks)
}

func TestBuildCollectsTasks(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testTestNode, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if graph.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", graph.Len())
	}
	if _, ok := graph.Task(testTestNode); !ok {
		t.Fatal(`Task("test") not found`)
	}
}

func TestBuildRejectsDuplicateTaskNames(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testBuildNode, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonDuplicateTaskName)
}

func TestBuildRejectsEmptyDependency(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testTestNode, DependsOn: []string{""}, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonEmptyDependency)
}

func TestBuildRejectsSelfDependency(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonSelfDependency)
}

func TestBuildRejectsUnknownDependency(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testTestNode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonUnknownDependency)
}

func TestBuildRejectsDuplicateDependency(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testTestNode, DependsOn: []string{testBuildNode, testBuildNode}, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonDuplicateDependency)
}

func TestBuildFillsDependencies(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testTestNode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := graph.Deps(testTestNode); !reflect.DeepEqual(got, []string{testBuildNode}) {
		t.Fatalf("Deps(test) = %v, want [build]", got)
	}
	if got := graph.Deps(testBuildNode); len(got) != 0 {
		t.Fatalf("Deps(build) = %v, want empty", got)
	}

	// 返回值是副本，改它不应影响 Graph 内部状态。
	got := graph.Deps(testTestNode)
	got[0] = "mutated"
	if again := graph.Deps(testTestNode); again[0] != testBuildNode {
		t.Fatalf("Graph state was mutated: %v", again)
	}
}

func TestBuildOrdersLinearGraph(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testTestNode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
			{Name: testDeployNode, DependsOn: []string{testTestNode}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := []string{testBuildNode, testTestNode, testDeployNode}
	if got := graph.Order(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Order() = %v, want %v", got, want)
	}
}

func TestBuildOrdersParallelGraph(t *testing.T) {
	// build -> unit / e2e -> deploy，中间两个节点可以并行。
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testUnitNode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
			{Name: testE2ENode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
			{Name: testDeployNode, DependsOn: []string{testUnitNode, testE2ENode}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	// 同一层的节点按名字排序，保证每次运行结果一致。
	want := []string{testBuildNode, testE2ENode, testUnitNode, testDeployNode}
	if got := graph.Order(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Order() = %v, want %v", got, want)
	}
}

func TestBuildRejectsCycles(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "a", DependsOn: []string{"b"}, TaskSpec: inlineSpec()},
			{Name: "b", DependsOn: []string{"c"}, TaskSpec: inlineSpec()},
			{Name: "c", DependsOn: []string{"a"}, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonCyclicDependency)
}

func TestBuildComputesLevelsLinearGraph(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testTestNode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
			{Name: testDeployNode, DependsOn: []string{testTestNode}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := [][]string{{testBuildNode}, {testTestNode}, {testDeployNode}}
	if got := graph.Levels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Levels() = %v, want %v", got, want)
	}
}

func TestBuildComputesLevelsParallelGraph(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testUnitNode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
			{Name: testE2ENode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
			{Name: testDeployNode, DependsOn: []string{testUnitNode, testE2ENode}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := [][]string{{testBuildNode}, {testE2ENode, testUnitNode}, {testDeployNode}}
	if got := graph.Levels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Levels() = %v, want %v", got, want)
	}

	// levels 展平后必须还原成拓扑序。
	if got, wantOrder := flattenLevels(graph.Levels()), graph.Order(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("flatten(Levels()) = %v, want Order() = %v", got, wantOrder)
	}
}

func TestBuildComputesLevelsMultipleRoots(t *testing.T) {
	// a、b 互不依赖，各自带一条链，最后在 e 汇聚。
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "a", TaskSpec: inlineSpec()},
			{Name: "b", TaskSpec: inlineSpec()},
			{Name: "c", DependsOn: []string{"a"}, TaskSpec: inlineSpec()},
			{Name: "d", DependsOn: []string{"b"}, TaskSpec: inlineSpec()},
			{Name: "e", DependsOn: []string{"c", "d"}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}
	if got := graph.Levels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Levels() = %v, want %v", got, want)
	}
}

func flattenLevels(levels [][]string) []string {
	var out []string
	for _, level := range levels {
		out = append(out, level...)
	}
	return out
}

func TestBuildRejectsEmptyTaskName(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonInvalidTask)
}

func TestBuildRejectsTaskWithoutExecutor(t *testing.T) {
	// template 和 taskSpec 都没设置，无法知道这个节点要执行什么。
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonInvalidTask)
}

func TestBuildRejectsTaskWithBothExecutors(t *testing.T) {
	// template 和 taskSpec 同时设置，存在二义性。
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, Template: testBuildTemplate, TaskSpec: inlineSpec()},
		},
		TaskTemplates: map[string]kubetaskv1.TaskSpec{
			testBuildTemplate: {Type: kubetaskv1.TaskTypeOneTime, Image: testGoImage},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonInvalidTask)
}

func TestBuildRejectsUnknownTemplate(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, Template: "missing"},
		},
		TaskTemplates: map[string]kubetaskv1.TaskSpec{
			testBuildTemplate: {Type: kubetaskv1.TaskTypeOneTime, Image: testGoImage},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonMissingTemplate)
}

func TestBuildAcceptsTemplateReference(t *testing.T) {
	// 同一个 Workflow 里允许模板引用和内联 taskSpec 混用。
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, Template: testBuildTemplate},
			{Name: testTestNode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
		},
		TaskTemplates: map[string]kubetaskv1.TaskSpec{
			testBuildTemplate: {Type: kubetaskv1.TaskTypeOneTime, Image: testGoImage},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if graph.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", graph.Len())
	}
	if got := graph.Order(); !reflect.DeepEqual(got, []string{testBuildNode, testTestNode}) {
		t.Fatalf("Order() = %v, want [build test]", got)
	}
}

func TestChildTaskName(t *testing.T) {
	if got := ChildTaskName("ci-pipeline", "unit-test"); got != "ci-pipeline-unit-test" {
		t.Fatalf("ChildTaskName() = %q, want ci-pipeline-unit-test", got)
	}
}

func TestValidateChildTaskNames(t *testing.T) {
	graph, err := Build(kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: testBuildNode, TaskSpec: inlineSpec()},
			{Name: testTestNode, DependsOn: []string{testBuildNode}, TaskSpec: inlineSpec()},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// 最长允许的 workflow 名：46 - 1 - len("build") = 40
	limit := naming.MaxTaskNameLength - 1 - len(testBuildNode)

	tests := []struct {
		name       string
		workflow   string
		wantErr    bool
		wantReason string
	}{
		{name: "short name is fine", workflow: "ci", wantErr: false},
		{name: "exactly at limit", workflow: strings.Repeat("a", limit), wantErr: false},
		{name: "one over limit", workflow: strings.Repeat("a", limit+1), wantErr: true, wantReason: ReasonInvalidTask},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateChildTaskNames(tt.workflow, graph)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr {
				requireReason(t, err, tt.wantReason)
			}
		})
	}
}
