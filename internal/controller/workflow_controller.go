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
	"context"
	stdErrors "errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/workflow"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// workflowFinalizer 保证 Workflow 被删除前先清理它创建的子 Task。
	workflowFinalizer = "kubetask.kubetask.io/workflow-finalizer"

	// workflowLabel 标记子 Task 归属的 Workflow，workflowNodeLabel 标记它对应的节点。
	workflowLabel     = "kubetask.io/workflow"
	workflowNodeLabel = "kubetask.io/workflow-node"

	// workflowCleanupRequeue 是等待子 Task 完全删除的重试间隔。
	workflowCleanupRequeue = 2 * time.Second

	// maxChildTaskNameLength 限制子 Task 名称长度。
	//
	// 推导：Task 控制器用 "<task 名>-<10 位 Unix 秒>" 创建 Job，Job 控制器再给 Pod 追加
	// "-<5 位随机后缀>"，而 Pod 名必须是不超过 63 字符的 DNS-1123 label。
	// 固定开销 1+10+1+5 = 17，因此子 Task 名最多 46 字符。
	maxChildTaskNameLength = 46
)

// errTaskNameConflict 表示 <workflow>-<node> 这个名字已被其它 Workflow 的子 Task 占用。
// 这是确定性冲突，重试不会自愈，因此直接把 Workflow 置为失败。
var errTaskNameConflict = stdErrors.New("child task name conflict")

// WorkflowReconciler reconciles a Workflow object.
//
// Workflow 只做编排：校验 DAG（复用 internal/workflow）、按依赖与 maxParallel 创建子 Task、
// 把子 Task 的状态回写到 status.nodes，最后汇总整体阶段。子 Task 的执行仍由 TaskReconciler 负责。
type WorkflowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kubetask.kubetask.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubetask.kubetask.io,resources=workflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubetask.kubetask.io,resources=workflows/finalizers,verbs=update
// +kubebuilder:rbac:groups=kubetask.kubetask.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubetask.kubetask.io,resources=tasks/status,verbs=get;update;patch

// Reconcile 把 Workflow 的期望状态推进到实际状态。
//
// 执行顺序：删除路径 -> finalizer -> 终态短路 -> spec 校验 -> 初始化 status ->
// 同步并推进节点 -> 汇总阶段 -> 一次写入 status。
func (r *WorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrlLog.FromContext(ctx)

	wf := &kubetaskv1.Workflow{}
	if err := r.Get(ctx, req.NamespacedName, wf); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Workflow", "workflow", req.Name)
		return ctrl.Result{}, err
	}

	if !wf.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, wf)
	}

	// 与 TaskReconciler 保持一致：第一次 reconcile 只登记 finalizer。
	if !controllerutil.ContainsFinalizer(wf, workflowFinalizer) {
		controllerutil.AddFinalizer(wf, workflowFinalizer)
		if err := r.Update(ctx, wf); err != nil {
			log.Error(err, "Failed to add finalizer", "workflow", wf.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 一个 Workflow CR 只运行一次：到达终态后不再重新调度。
	if isWorkflowTerminal(wf.Status.Phase) {
		switch {
		case wf.Status.ObservedGeneration == wf.Generation:
			return ctrl.Result{}, nil
		case canRetryAfterValidationFailure(wf):
			// spec 修正后允许重新调度：此前校验失败，没有创建过任何子 Task。
			log.Info("Retrying Workflow after spec fix", "workflow", wf.Name, "generation", wf.Generation)
		default:
			wf.Status.ObservedGeneration = wf.Generation
			if err := r.Status().Update(ctx, wf); err != nil {
				log.Error(err, "Failed to update Workflow status", "workflow", wf.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	log.Info("Reconciling Workflow", "workflow", wf.Name, "phase", wf.Status.Phase, "generation", wf.Generation)

	graph, err := workflow.Build(wf.Spec)
	if err != nil {
		return r.markWorkflowFailed(ctx, wf, workflowFailureMessage(err))
	}

	// 子 Task 名会被复用为 Job / Pod 名，超长时提前失败，避免创建出必然失败的 Job。
	if err := validateChildTaskNames(wf, graph); err != nil {
		return r.markWorkflowFailed(ctx, wf, workflowFailureMessage(err))
	}

	return r.reconcileGraph(ctx, wf, graph)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubetaskv1.Workflow{}).
		Owns(&kubetaskv1.Task{}).
		Named("workflow").
		Complete(r)
}

// =============================================================================
// Deletion
// =============================================================================

// handleDeletion 删除 Workflow 创建的全部子 Task，清空后摘掉 finalizer。
func (r *WorkflowReconciler) handleDeletion(ctx context.Context, wf *kubetaskv1.Workflow) (ctrl.Result, error) {
	log := ctrlLog.FromContext(ctx)

	children, err := r.listChildTasks(ctx, wf)
	if err != nil {
		return ctrl.Result{}, err
	}

	remaining := 0
	for _, child := range children {
		if !child.DeletionTimestamp.IsZero() {
			remaining++
			continue
		}
		if err := r.Delete(ctx, child, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete child Task", "workflow", wf.Name, "task", child.Name)
				return ctrl.Result{}, err
			}
			continue
		}
		log.Info("Deleted child Task", "workflow", wf.Name, "task", child.Name)
		remaining++
	}

	if remaining > 0 {
		log.Info("Waiting for child Tasks to be deleted", "workflow", wf.Name, "remaining", remaining)
		return ctrl.Result{RequeueAfter: workflowCleanupRequeue}, nil
	}

	if controllerutil.ContainsFinalizer(wf, workflowFinalizer) {
		controllerutil.RemoveFinalizer(wf, workflowFinalizer)
		if err := r.Update(ctx, wf); err != nil {
			log.Error(err, "Failed to remove finalizer", "workflow", wf.Name)
			return ctrl.Result{}, err
		}
		log.Info("Workflow deletion completed", "workflow", wf.Name)
	}

	return ctrl.Result{}, nil
}

// =============================================================================
// Validation failures
// =============================================================================

// markWorkflowFailed 把无法调度的 Workflow（spec 非法、名字冲突等）写成终态失败。
func (r *WorkflowReconciler) markWorkflowFailed(ctx context.Context, wf *kubetaskv1.Workflow, message string) (ctrl.Result, error) {
	log := ctrlLog.FromContext(ctx)

	// 同一条失败信息不重复写 status，避免无意义的 update 再触发一轮 reconcile。
	if wf.Status.Phase == kubetaskv1.WorkflowFailed &&
		wf.Status.Message == message &&
		wf.Status.ObservedGeneration == wf.Generation {
		return ctrl.Result{}, nil
	}

	now := metav1.Now()
	wf.Status.Phase = kubetaskv1.WorkflowFailed
	wf.Status.Message = message
	wf.Status.ObservedGeneration = wf.Generation
	if wf.Status.CompletionTime == nil {
		wf.Status.CompletionTime = &now
	}

	log.Info("Workflow spec rejected", "workflow", wf.Name, "message", message)
	if err := r.Status().Update(ctx, wf); err != nil {
		log.Error(err, "Failed to update Workflow status", "workflow", wf.Name)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// workflowFailureMessage 把校验错误转成 status.message：稳定的 Reason + 可读说明。
func workflowFailureMessage(err error) string {
	var validationErr *workflow.ValidationError
	if stdErrors.As(err, &validationErr) {
		return fmt.Sprintf("%s: %s", validationErr.Reason, validationErr.Message)
	}
	return err.Error()
}

// validateChildTaskNames 提前拦截会派生非法 Job / Pod 名的节点。
func validateChildTaskNames(wf *kubetaskv1.Workflow, graph *workflow.Graph) error {
	offending := make([]string, 0)
	for _, name := range graph.Order() {
		childName := childTaskName(wf.Name, name)
		if len(childName) > maxChildTaskNameLength {
			offending = append(offending, fmt.Sprintf("%s (%d chars)", childName, len(childName)))
		}
	}
	if len(offending) == 0 {
		return nil
	}

	return &workflow.ValidationError{
		Reason: workflow.ReasonInvalidTask,
		Message: fmt.Sprintf("child task names must not exceed %d characters: %s",
			maxChildTaskNameLength, strings.Join(offending, ", ")),
	}
}

// =============================================================================
// Scheduling
// =============================================================================

// reconcileGraph 推进一次 DAG：初始化 status、同步并调度节点、汇总阶段，最后只写一次 status。
func (r *WorkflowReconciler) reconcileGraph(ctx context.Context, wf *kubetaskv1.Workflow, graph *workflow.Graph) (ctrl.Result, error) {
	log := ctrlLog.FromContext(ctx)

	changed := initializeStatus(wf, graph)

	advanced, err := r.advanceNodes(ctx, wf, graph)
	if err != nil {
		return ctrl.Result{}, err
	}
	changed = advanced || changed

	if summarize(wf) {
		changed = true
	}

	if !changed {
		return ctrl.Result{}, nil
	}

	if err := r.Status().Update(ctx, wf); err != nil {
		log.Error(err, "Failed to update Workflow status", "workflow", wf.Name)
		return ctrl.Result{}, err
	}
	log.Info("Workflow status updated", "workflow", wf.Name, "phase", wf.Status.Phase)
	return ctrl.Result{}, nil
}

// initializeStatus 让 status 反映当前 spec：进入 Running、记录 generation 与开始时间，
// 并按拓扑序补齐节点（保留已有节点状态、丢弃已不在 spec 中的节点）。
func initializeStatus(wf *kubetaskv1.Workflow, graph *workflow.Graph) bool {
	changed := false

	if wf.Status.Phase != kubetaskv1.WorkflowRunning {
		wf.Status.Phase = kubetaskv1.WorkflowRunning
		// 首次调度，或"校验失败后修正 spec"的重新调度：清掉上一次的终态信息。
		if wf.Status.Message != "" {
			wf.Status.Message = ""
		}
		if wf.Status.CompletionTime != nil {
			wf.Status.CompletionTime = nil
		}
		changed = true
	}

	if wf.Status.StartTime == nil {
		now := metav1.Now()
		wf.Status.StartTime = &now
		changed = true
	}

	if wf.Status.ObservedGeneration != wf.Generation {
		wf.Status.ObservedGeneration = wf.Generation
		changed = true
	}

	existing := make(map[string]kubetaskv1.WorkflowNodeStatus, len(wf.Status.Nodes))
	for _, node := range wf.Status.Nodes {
		existing[node.Name] = node
	}

	nodes := make([]kubetaskv1.WorkflowNodeStatus, 0, graph.Len())
	for _, name := range graph.Order() {
		node, ok := existing[name]
		if !ok {
			nodes = append(nodes, kubetaskv1.WorkflowNodeStatus{Name: name, Phase: kubetaskv1.NodePending})
			continue
		}
		if node.Phase == "" {
			node.Phase = kubetaskv1.NodePending
		}
		nodes = append(nodes, node)
	}

	if !reflect.DeepEqual(nodes, wf.Status.Nodes) {
		wf.Status.Nodes = nodes
		changed = true
	}

	return changed
}

// advanceNodes 做两趟处理：
//  1. Running 节点：把子 Task 的真实阶段回写到 status.nodes，子 Task 被删除时按同名重建；
//  2. Pending 节点：按拓扑序判定"就绪 / 跳过 / 等待"，就绪节点在 maxParallel 允许时创建子 Task。
func (r *WorkflowReconciler) advanceNodes(ctx context.Context, wf *kubetaskv1.Workflow, graph *workflow.Graph) (bool, error) {
	log := ctrlLog.FromContext(ctx)

	children, err := r.listChildTasks(ctx, wf)
	if err != nil {
		return false, err
	}

	changed := false
	phases := make(map[string]kubetaskv1.WorkflowNodePhase, len(wf.Status.Nodes))
	running := 0
	for _, node := range wf.Status.Nodes {
		phases[node.Name] = node.Phase
		if node.Phase == kubetaskv1.NodeRunning {
			running++
		}
	}

	for i := range wf.Status.Nodes {
		node := &wf.Status.Nodes[i]
		if node.Phase != kubetaskv1.NodeRunning {
			continue
		}

		child := children[childTaskName(wf.Name, node.Name)]
		if child == nil {
			// 子 Task 被外部删除：用同一个确定性名字重建，保证状态收敛。
			if _, err := r.createChildTask(ctx, wf, graph, node.Name); err != nil {
				if stdErrors.Is(err, errTaskNameConflict) {
					failed := failWorkflow(wf, err.Error())
					return failed || changed, nil
				}
				return changed, err
			}
			log.Info("Recreated missing child Task", "workflow", wf.Name, "node", node.Name)
			continue
		}

		if syncNodeFromTask(node, child) {
			if node.Phase != kubetaskv1.NodeRunning {
				// 节点已经到达终态：立刻释放 maxParallel 名额，让本轮就能调度后面的 Pending 节点。
				running--
			}
			phases[node.Name] = node.Phase
			changed = true
		}
	}

	// status.nodes 已经按拓扑序排列，这里顺序遍历即可保证调度顺序确定。
	for i := range wf.Status.Nodes {
		node := &wf.Status.Nodes[i]
		if node.Phase != kubetaskv1.NodePending {
			continue
		}

		action, reason := decideNode(graph, phases, node.Name)
		switch action {
		case nodeSkip:
			markNodeSkipped(node, reason)
			phases[node.Name] = node.Phase
			changed = true
		case nodeReady:
			if wf.Spec.MaxParallel > 0 && running >= wf.Spec.MaxParallel {
				continue
			}
			child, err := r.createChildTask(ctx, wf, graph, node.Name)
			if err != nil {
				if stdErrors.Is(err, errTaskNameConflict) {
					failed := failWorkflow(wf, err.Error())
					return failed || changed, nil
				}
				return changed, err
			}
			markNodeRunning(node, child)
			// 复用的子 Task 可能已经到达终态（Controller 在创建与写状态之间重启过）。
			// 这种情况下必须立刻落终态：终态子 Task 不会再产生事件来唤醒本次调度，
			// 而它也不占用 maxParallel 名额。
			if syncNodeFromTask(node, child) {
				log.Info("Adopted finished child Task", "workflow", wf.Name, "node", node.Name,
					"task", child.Name, "phase", node.Phase)
			} else {
				running++
			}
			phases[node.Name] = node.Phase
			changed = true
			log.Info("Scheduled workflow node", "workflow", wf.Name, "node", node.Name, "task", child.Name)
		case nodeWait:
		}
	}

	return changed, nil
}

// summarize 把节点状态汇总成 Workflow 整体阶段；只有全部节点到达终态才给出结论。
func summarize(wf *kubetaskv1.Workflow) bool {
	if isWorkflowTerminal(wf.Status.Phase) {
		return false
	}

	failed := make([]string, 0)
	pending := 0
	for _, node := range wf.Status.Nodes {
		switch node.Phase {
		case kubetaskv1.NodeFailed:
			failed = append(failed, node.Name)
		case kubetaskv1.NodeSucceeded, kubetaskv1.NodeSkipped:
		default:
			pending++
		}
	}

	if pending > 0 {
		return false
	}

	now := metav1.Now()
	wf.Status.CompletionTime = &now

	if len(failed) > 0 {
		wf.Status.Phase = kubetaskv1.WorkflowFailed
		wf.Status.Message = fmt.Sprintf("nodes failed: %s", strings.Join(failed, ", "))
		return true
	}

	wf.Status.Phase = kubetaskv1.WorkflowSucceeded
	wf.Status.Message = ""
	return true
}

// =============================================================================
// Child Tasks
// =============================================================================

// childTaskName 是子 Task 的确定性名字，让重复 reconcile 天然幂等。
func childTaskName(workflowName, nodeName string) string {
	return fmt.Sprintf("%s-%s", workflowName, nodeName)
}

// childTaskSpec 取模板或内联规格的快照，并强制 Workflow 节点按一次性任务执行。
func childTaskSpec(wf *kubetaskv1.Workflow, graph *workflow.Graph, nodeName string) kubetaskv1.TaskSpec {
	task, ok := graph.Task(nodeName)
	if !ok {
		return kubetaskv1.TaskSpec{}
	}

	var spec kubetaskv1.TaskSpec
	switch {
	case task.Template != "":
		template := wf.Spec.TaskTemplates[task.Template]
		spec = *template.DeepCopy()
	case task.TaskSpec != nil:
		spec = *task.TaskSpec.DeepCopy()
	}

	// 模板/内联里残留的调度字段在这里被强制清空。
	spec.Type = kubetaskv1.TaskTypeOneTime
	spec.Schedule = ""
	spec.Delay = nil
	spec.Suspend = nil
	return spec
}

// listChildTasks 一次列出某个 Workflow 的全部子 Task（按 label 归属）。
func (r *WorkflowReconciler) listChildTasks(ctx context.Context, wf *kubetaskv1.Workflow) (map[string]*kubetaskv1.Task, error) {
	tasks := &kubetaskv1.TaskList{}
	if err := r.List(ctx, tasks, client.MatchingLabels{workflowLabel: wf.Name}); err != nil {
		return nil, err
	}

	children := make(map[string]*kubetaskv1.Task, len(tasks.Items))
	for i := range tasks.Items {
		task := &tasks.Items[i]
		children[task.Name] = task
	}
	return children, nil
}

// createChildTask 创建（或复用）节点对应的子 Task。
//
// 名字是确定性的，所以 AlreadyExists 属于正常路径：只要 owner 是本 Workflow 就直接复用；
// 若被别的 Workflow 占用则返回 errTaskNameConflict。
func (r *WorkflowReconciler) createChildTask(ctx context.Context, wf *kubetaskv1.Workflow, graph *workflow.Graph, nodeName string) (*kubetaskv1.Task, error) {
	name := childTaskName(wf.Name, nodeName)
	task := &kubetaskv1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				workflowLabel:     wf.Name,
				workflowNodeLabel: nodeName,
			},
		},
		Spec: childTaskSpec(wf, graph, nodeName),
	}

	if err := controllerutil.SetControllerReference(wf, task, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference on task %q: %w", name, err)
	}

	if err := r.Create(ctx, task); err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, err
		}

		existing := &kubetaskv1.Task{}
		if getErr := r.Get(ctx, client.ObjectKey{Name: name}, existing); getErr != nil {
			return nil, getErr
		}
		if !metav1.IsControlledBy(existing, wf) {
			return nil, fmt.Errorf("%w: task %q already exists and is not controlled by workflow %q",
				errTaskNameConflict, name, wf.Name)
		}
		return existing, nil
	}

	return task, nil
}

// =============================================================================
// Node state helpers
// =============================================================================

// nodeAction 是 Pending 节点在当前一轮 reconcile 中的处理方式。
type nodeAction int

const (
	nodeWait nodeAction = iota
	nodeReady
	nodeSkip
)

// decideNode 根据依赖状态决定 Pending 节点的动作：
//
//	runOnFailure=false：全部依赖 Succeeded 才执行；任一依赖 Failed/Skipped 立即跳过；
//	runOnFailure=true：等待全部依赖到达终态，至少一个失败才执行，全部成功则跳过。
func decideNode(graph *workflow.Graph, phases map[string]kubetaskv1.WorkflowNodePhase, name string) (nodeAction, string) {
	task, ok := graph.Task(name)
	if !ok {
		return nodeWait, ""
	}

	blockedBy := ""
	waiting := 0
	for _, dep := range graph.Deps(name) {
		switch phases[dep] {
		case kubetaskv1.NodeSucceeded:
		case kubetaskv1.NodeFailed, kubetaskv1.NodeSkipped:
			if blockedBy == "" {
				blockedBy = dep
			}
		default:
			waiting++
		}
	}

	if task.RunOnFailure {
		switch {
		case waiting > 0:
			return nodeWait, ""
		case blockedBy == "":
			return nodeSkip, "all dependencies succeeded"
		default:
			return nodeReady, ""
		}
	}

	switch {
	case blockedBy != "":
		return nodeSkip, fmt.Sprintf("dependency %q did not succeed", blockedBy)
	case waiting > 0:
		return nodeWait, ""
	default:
		return nodeReady, ""
	}
}

// markNodeRunning 记录节点开始执行。
//
// 复用的子 Task 可能早就开始过（例如 Controller 在创建 Task 与写 status 之间重启），
// 这时用子 Task 自己的开始时间，避免 status 里的时间线失真。
func markNodeRunning(node *kubetaskv1.WorkflowNodeStatus, task *kubetaskv1.Task) {
	node.Phase = kubetaskv1.NodeRunning
	node.TaskName = task.Name
	node.Message = ""
	if node.StartTime == nil {
		startTime := metav1.Now()
		if task.Status.LastStartTime != nil {
			startTime = *task.Status.LastStartTime.DeepCopy()
		}
		node.StartTime = &startTime
	}
}

// markNodeSkipped 记录节点被跳过（依赖未成功，或 runOnFailure 节点的依赖全部成功）。
func markNodeSkipped(node *kubetaskv1.WorkflowNodeStatus, reason string) {
	now := metav1.Now()
	node.Phase = kubetaskv1.NodeSkipped
	node.Message = reason
	if node.CompletionTime == nil {
		node.CompletionTime = &now
	}
}

// syncNodeFromTask 把子 Task 的终态回写到节点，返回节点是否发生变化。
func syncNodeFromTask(node *kubetaskv1.WorkflowNodeStatus, task *kubetaskv1.Task) bool {
	switch task.Status.Phase {
	case kubetaskv1.TaskSucceeded:
		if node.Phase == kubetaskv1.NodeSucceeded {
			return false
		}
		node.Phase = kubetaskv1.NodeSucceeded
		node.CompletionTime = taskCompletionTime(task)
		node.Message = ""
		return true
	case kubetaskv1.TaskFailed:
		message := taskFailureMessage(task)
		if node.Phase == kubetaskv1.NodeFailed && node.Message == message {
			return false
		}
		node.Phase = kubetaskv1.NodeFailed
		node.CompletionTime = taskCompletionTime(task)
		node.Message = message
		return true
	default:
		return false
	}
}

// taskCompletionTime 优先使用子 Task 自己记录的完成时间，缺失时退化成当前时间。
func taskCompletionTime(task *kubetaskv1.Task) *metav1.Time {
	if task.Status.LastCompletionTime != nil {
		return task.Status.LastCompletionTime.DeepCopy()
	}
	now := metav1.Now()
	return &now
}

// taskFailureMessage 取子 Task 的失败说明。
func taskFailureMessage(task *kubetaskv1.Task) string {
	if task.Status.Message != "" {
		return task.Status.Message
	}
	return fmt.Sprintf("task %s failed", task.Name)
}

// failWorkflow 把 Workflow 直接置为终态失败（确定性错误，重试也不会好转）。
func failWorkflow(wf *kubetaskv1.Workflow, message string) bool {
	if wf.Status.Phase == kubetaskv1.WorkflowFailed && wf.Status.Message == message {
		return false
	}

	now := metav1.Now()
	wf.Status.Phase = kubetaskv1.WorkflowFailed
	wf.Status.Message = message
	if wf.Status.CompletionTime == nil {
		wf.Status.CompletionTime = &now
	}
	return true
}

// isWorkflowTerminal 判断 Workflow 是否已经给出终态结论。
func isWorkflowTerminal(phase kubetaskv1.WorkflowPhase) bool {
	return phase == kubetaskv1.WorkflowSucceeded || phase == kubetaskv1.WorkflowFailed
}

// canRetryAfterValidationFailure 判断终态 Workflow 能否按新 spec 重新调度。
//
// 只有"因 spec 非法而失败、且从未创建过子 Task"的情况允许：其余保持一次性语义，
// spec 变更只同步 observedGeneration，不重新执行。
func canRetryAfterValidationFailure(wf *kubetaskv1.Workflow) bool {
	return wf.Status.Phase == kubetaskv1.WorkflowFailed && len(wf.Status.Nodes) == 0
}
