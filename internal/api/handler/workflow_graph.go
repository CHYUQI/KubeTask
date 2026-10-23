package handler

import (
	stdErrors "errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/workflow"
)

// WorkflowGraphNode 是 DAG 视图里的一个节点：声明的依赖 + 运行时状态。
type WorkflowGraphNode struct {
	Name           string                       `json:"name"`
	DependsOn      []string                     `json:"dependsOn"`
	RunOnFailure   bool                         `json:"runOnFailure,omitempty"`
	Phase          kubetaskv1.WorkflowNodePhase `json:"phase"`
	TaskName       string                       `json:"taskName,omitempty"`
	StartTime      *metav1.Time                 `json:"startTime,omitempty"`
	CompletionTime *metav1.Time                 `json:"completionTime,omitempty"`
	Message        string                       `json:"message,omitempty"`
}

// WorkflowGraphEdge 是一条依赖边：From 到达终态后 To 才可能被调度。
type WorkflowGraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// WorkflowGraph 供前端渲染 DAG。
//
// Nodes / Edges 始终可用（直接来自 spec），Order / Levels 需要 spec 合法才能算出；
// spec 非法时（例如运行中被改坏）返回 ValidationError，让详情页仍能展示现场。
type WorkflowGraph struct {
	Nodes           []WorkflowGraphNode `json:"nodes"`
	Edges           []WorkflowGraphEdge `json:"edges"`
	Order           []string            `json:"order,omitempty"`
	Levels          [][]string          `json:"levels,omitempty"`
	ValidationError string              `json:"validationError,omitempty"`
}

// buildWorkflowGraph 把 Workflow 的 spec（拓扑）与 status（运行时状态）合成 DAG 视图数据。
func buildWorkflowGraph(wf *kubetaskv1.Workflow) WorkflowGraph {
	statusByNode := make(map[string]kubetaskv1.WorkflowNodeStatus, len(wf.Status.Nodes))
	for _, node := range wf.Status.Nodes {
		statusByNode[node.Name] = node
	}

	graph := WorkflowGraph{
		Nodes: make([]WorkflowGraphNode, 0, len(wf.Spec.Tasks)),
		Edges: make([]WorkflowGraphEdge, 0),
	}

	for _, task := range wf.Spec.Tasks {
		var status *kubetaskv1.WorkflowNodeStatus
		if node, ok := statusByNode[task.Name]; ok {
			status = &node
			delete(statusByNode, task.Name)
		}
		graph.Nodes = append(graph.Nodes,
			newWorkflowGraphNode(task.Name, task.DependsOn, task.RunOnFailure, status))

		for _, dep := range task.DependsOn {
			graph.Edges = append(graph.Edges, WorkflowGraphEdge{From: dep, To: task.Name})
		}
	}

	// status 里有、spec 里已经没有的节点（运行中删过节点）也带上，方便排查孤儿子 Task。
	for _, node := range wf.Status.Nodes {
		status, ok := statusByNode[node.Name]
		if !ok {
			continue
		}
		graph.Nodes = append(graph.Nodes, newWorkflowGraphNode(status.Name, nil, false, &status))
		delete(statusByNode, node.Name)
	}

	if built, err := workflow.Build(wf.Spec); err != nil {
		graph.ValidationError = workflowValidationText(err)
	} else {
		graph.Order = built.Order()
		graph.Levels = built.Levels()
	}

	return graph
}

func newWorkflowGraphNode(
	name string,
	dependsOn []string,
	runOnFailure bool,
	status *kubetaskv1.WorkflowNodeStatus,
) WorkflowGraphNode {
	node := WorkflowGraphNode{
		Name:         name,
		DependsOn:    append([]string{}, dependsOn...),
		RunOnFailure: runOnFailure,
		Phase:        kubetaskv1.NodePending,
	}

	if status != nil {
		node.Phase = status.Phase
		node.TaskName = status.TaskName
		node.StartTime = status.StartTime
		node.CompletionTime = status.CompletionTime
		node.Message = status.Message
	}
	if node.Phase == "" {
		node.Phase = kubetaskv1.NodePending
	}

	return node
}

// workflowValidationText 把校验错误转成稳定的 "Reason: Message" 文本。
func workflowValidationText(err error) string {
	if validationErr, ok := stdErrors.AsType[*workflow.ValidationError](err); ok {
		return fmt.Sprintf("%s: %s", validationErr.Reason, validationErr.Message)
	}
	return err.Error()
}
