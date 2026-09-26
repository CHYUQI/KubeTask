package workflow

import (
	"errors"
	"reflect"
	"testing"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
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
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "test", TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if graph.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", graph.Len())
	}
	if _, ok := graph.Task("test"); !ok {
		t.Fatal(`Task("test") not found`)
	}
}

func TestBuildRejectsDuplicateTaskNames(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "build", TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonDuplicateTaskName)
}

func TestBuildRejectsEmptyDependency(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "test", DependsOn: []string{""}, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonEmptyDependency)
}

func TestBuildRejectsSelfDependency(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "build", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonSelfDependency)
}

func TestBuildRejectsUnknownDependency(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "test", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonUnknownDependency)
}

func TestBuildRejectsDuplicateDependency(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "test", DependsOn: []string{"build", "build"}, TaskSpec: inlineSpec()},
		},
	}

	_, err := Build(spec)
	requireReason(t, err, ReasonDuplicateDependency)
}

func TestBuildFillsDependencies(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "test", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := graph.Deps("test"); !reflect.DeepEqual(got, []string{"build"}) {
		t.Fatalf("Deps(test) = %v, want [build]", got)
	}
	if got := graph.Deps("build"); len(got) != 0 {
		t.Fatalf("Deps(build) = %v, want empty", got)
	}

	// 返回值是副本，改它不应影响 Graph 内部状态。
	got := graph.Deps("test")
	got[0] = "mutated"
	if again := graph.Deps("test"); again[0] != "build" {
		t.Fatalf("Graph state was mutated: %v", again)
	}
}

func TestBuildOrdersLinearGraph(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "test", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
			{Name: "deploy", DependsOn: []string{"test"}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := []string{"build", "test", "deploy"}
	if got := graph.Order(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Order() = %v, want %v", got, want)
	}
}

func TestBuildOrdersParallelGraph(t *testing.T) {
	// build -> unit / e2e -> deploy，中间两个节点可以并行。
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "unit", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
			{Name: "e2e", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
			{Name: "deploy", DependsOn: []string{"unit", "e2e"}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	// 同一层的节点按名字排序，保证每次运行结果一致。
	want := []string{"build", "e2e", "unit", "deploy"}
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
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "test", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
			{Name: "deploy", DependsOn: []string{"test"}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := [][]string{{"build"}, {"test"}, {"deploy"}}
	if got := graph.Levels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Levels() = %v, want %v", got, want)
	}
}

func TestBuildComputesLevelsParallelGraph(t *testing.T) {
	spec := kubetaskv1.WorkflowSpec{
		Tasks: []kubetaskv1.WorkflowTask{
			{Name: "build", TaskSpec: inlineSpec()},
			{Name: "unit", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
			{Name: "e2e", DependsOn: []string{"build"}, TaskSpec: inlineSpec()},
			{Name: "deploy", DependsOn: []string{"unit", "e2e"}, TaskSpec: inlineSpec()},
		},
	}

	graph, err := Build(spec)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := [][]string{{"build"}, {"e2e", "unit"}, {"deploy"}}
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
