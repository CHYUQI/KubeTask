// Package workflow 提供 Workflow DAG 的校验、拓扑排序与调度辅助逻辑。
package workflow

import (
	"fmt"
	"sort"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

// ValidationError 表示 Workflow spec 无法被调度。
// Reason 是稳定的错误标识，Controller 可以用它写 status/Event；
// Message 是给人看的详细说明。
type ValidationError struct {
	Reason  string
	Message string
}

// Error 实现 error 接口。
func (e *ValidationError) Error() string {
	return e.Message
}

// ValidationError.Reason 使用的稳定取值。
const (
	ReasonNoTasks             = "NoTasks"
	ReasonDuplicateTaskName   = "DuplicateTaskName"
	ReasonInvalidTask         = "InvalidTask"
	ReasonEmptyDependency     = "EmptyDependency"
	ReasonSelfDependency      = "SelfDependency"
	ReasonUnknownDependency   = "UnknownDependency"
	ReasonDuplicateDependency = "DuplicateDependency"
	ReasonMissingTemplate     = "MissingTemplate"
	ReasonCyclicDependency    = "CyclicDependency"
)

// Graph 是一个校验通过的 DAG，提供拓扑序与并行层。
type Graph struct {
	// all private fields,using method to access for safety and encapsulation
	// in golang Uppercase fields are exported, lowercase fields are private to the package
	// tasks 按名字索引每个节点，后续校验与创建子 Task 都靠它取原始定义。
	tasks map[string]kubetaskv1.WorkflowTask

	// deps 保存每个节点的直接依赖（邻接表）。
	// 例如 deps["deploy"] = ["build", "test"]。
	deps map[string][]string

	// order 是拓扑序：任意节点的依赖都排在它前面。
	order []string

	// levels 是并行层：同一层内的节点互不依赖，可以同时执行。
	levels [][]string
}

// invalidf 统一构造带 Reason 的 ValidationError。
func invalidf(reason, format string, args ...any) *ValidationError {
	return &ValidationError{Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// Build 校验 spec 并构造成可供调度的 Graph。
//
// 当前进度：校验任务列表非空、收集任务定义并拒绝重名、校验依赖并填充邻接表、
// 用 Kahn 算法计算拓扑序并检测环、计算并行层；任务字段校验在后续小步中补齐。
func Build(spec kubetaskv1.WorkflowSpec) (*Graph, error) {
	if len(spec.Tasks) == 0 {
		return nil, invalidf(ReasonNoTasks, "workflow spec has no tasks")
	}

	g := &Graph{
		tasks: make(map[string]kubetaskv1.WorkflowTask, len(spec.Tasks)),
		deps:  make(map[string][]string, len(spec.Tasks)),
	}

	for i := range spec.Tasks {
		task := spec.Tasks[i]
		if _, exists := g.tasks[task.Name]; exists {
			return nil, invalidf(ReasonDuplicateTaskName, "task name %q is declared more than once", task.Name)
		}
		g.tasks[task.Name] = task
	}

	// deps check
	for i := range spec.Tasks {
		task := spec.Tasks[i]
		seen := make(map[string]struct{}, len(task.DependsOn))
		for _, dep := range task.DependsOn {
			// empty dependency, self dependency, unknown dependency, duplicate dependency
			if dep == "" {
				return nil, invalidf(ReasonEmptyDependency, "task %q has an empty dependency", task.Name)
			}
			if dep == task.Name {
				return nil, invalidf(ReasonSelfDependency, "task %q depends on itself", task.Name)
			}
			if _, exists := g.tasks[dep]; !exists {
				return nil, invalidf(ReasonUnknownDependency, "task %q depends on unknown task %q", task.Name, dep)
			}
			if _, duplicate := seen[dep]; duplicate {
				return nil, invalidf(ReasonDuplicateDependency, "task %q lists dependency %q more than once", task.Name, dep)
			}
			seen[dep] = struct{}{}
			g.deps[task.Name] = append(g.deps[task.Name], dep)
		}
	}

	if err := g.computeOrder(); err != nil {
		return nil, err
	}
	g.computeLevels()

	return g, nil
}

// computeOrder 用 Kahn 算法计算拓扑序，同时检测依赖环。
func (g *Graph) computeOrder() error {
	// indegree counts how many dependencies each node has.
	indegree := make(map[string]int, len(g.tasks))
	for name := range g.tasks {
		indegree[name] = len(g.deps[name])
	}

	// dependents 是反向邻接表：谁依赖我。
	// deps 回答"我依赖谁"，Kahn 每处理完一个节点，需要去减少它的下游入度。
	dependents := make(map[string][]string, len(g.tasks))
	for name, deps := range g.deps {
		for _, dep := range deps {
			dependents[dep] = append(dependents[dep], name)
		}
	}
	for name := range dependents {
		sort.Strings(dependents[name])
	}

	// 第一批可以执行的节点：入度为 0。排序保证结果确定。
	// all nodes with indegree 0 can be executed in parallel, so we collect them into the current slice.
	current := make([]string, 0, len(g.tasks))
	for name, degree := range indegree {
		if degree == 0 {
			current = append(current, name)
		}
	}
	sort.Strings(current)

	// procss all the current nodes with 0 indegree
	for len(current) > 0 {
		// append 0 indegree nodes to the order slice, which is the topological order of the graph.
		g.order = append(g.order, current...)
		// next collects the next batch of nodes whose indegree drops to 0 after processing current.
		next := make([]string, 0, len(current))
		for _, name := range current {
			for _, dependent := range dependents[name] {
				indegree[dependent]--
				if indegree[dependent] == 0 {
					next = append(next, dependent)
				}
			}
		}
		sort.Strings(next)
		current = next
	}

	// 有节点始终没被处理，说明它们卡在环里。
	if len(g.order) != len(g.tasks) {
		// init stuck slice
		stuck := make([]string, 0, len(g.tasks)-len(g.order))
		for name, degree := range indegree {
			if degree > 0 {
				stuck = append(stuck, name)
			}
		}
		sort.Strings(stuck)
		return invalidf(ReasonCyclicDependency, "workflow contains a dependency cycle involving %v", stuck)
	}

	return nil
}

// computeLevels 在拓扑序上做一次动态规划，算出每个节点所在的并行层。
//
// level(节点) 的规则：
//
//	没有依赖            -> 0
//	有依赖              -> max(依赖的 level) + 1
//
// 因为 order 是拓扑序，任何依赖都排在它的使用者前面，所以扫一遍 order
// 就能把所有节点的 level 算出来，不需要重复一次 Kahn 分层。
func (g *Graph) computeLevels() {
	levelOf := make(map[string]int, len(g.tasks))
	maxLevel := -1

	for _, name := range g.order {
		level := 0
		for _, dep := range g.deps[name] {
			if candidate := levelOf[dep] + 1; candidate > level {
				level = candidate
			}
		}
		levelOf[name] = level
		if level > maxLevel {
			maxLevel = level
		}
	}

	// 按层装箱。再次遍历 order 而不是 map，保证同层内顺序确定。
	g.levels = make([][]string, maxLevel+1)
	for _, name := range g.order {
		level := levelOf[name]
		g.levels[level] = append(g.levels[level], name)
	}
}

// Len 返回图中节点数量。
func (g *Graph) Len() int {
	return len(g.tasks)
}

// Order 返回拓扑序的副本，调用方修改不会影响 Graph 内部状态。
func (g *Graph) Order() []string {
	return append([]string(nil), g.order...)
}

// Levels 返回并行层的副本。
func (g *Graph) Levels() [][]string {
	levels := make([][]string, len(g.levels))
	for i, level := range g.levels {
		levels[i] = append([]string(nil), level...)
	}
	return levels
}

// Task 按名字返回节点定义。
func (g *Graph) Task(name string) (kubetaskv1.WorkflowTask, bool) {
	task, ok := g.tasks[name]
	return task, ok
}

// Deps 返回指定节点的直接依赖副本。
func (g *Graph) Deps(name string) []string {
	return append([]string(nil), g.deps[name]...)
}
