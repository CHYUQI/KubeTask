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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkflowPhase 表示 Workflow 的整体生命周期阶段。
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed
type WorkflowPhase string

const (
	WorkflowPending   WorkflowPhase = "Pending"
	WorkflowRunning   WorkflowPhase = "Running"
	WorkflowSucceeded WorkflowPhase = "Succeeded"
	WorkflowFailed    WorkflowPhase = "Failed"
)

// WorkflowNodePhase 表示单个 DAG 节点的生命周期阶段。
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Skipped
type WorkflowNodePhase string

const (
	NodePending   WorkflowNodePhase = "Pending"
	NodeRunning   WorkflowNodePhase = "Running"
	NodeSucceeded WorkflowNodePhase = "Succeeded"
	NodeFailed    WorkflowNodePhase = "Failed"
	NodeSkipped   WorkflowNodePhase = "Skipped"
)

// WorkflowSpec 定义 Workflow 的期望状态。
type WorkflowSpec struct {
	// MaxParallel 限制同时运行的节点数量，0 表示不限制。
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxParallel int `json:"maxParallel,omitempty"`

	// Tasks 声明 DAG 节点，节点名在同一个 Workflow 内唯一。
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	// +listType=map
	// +listMapKey=name
	// +required
	Tasks []WorkflowTask `json:"tasks"`

	// TaskTemplates 定义可复用的 TaskSpec 命名模板，由节点通过 template 引用。
	// +optional
	TaskTemplates map[string]TaskSpec `json:"taskTemplates,omitempty"`
}

// WorkflowTask 是 DAG 中的一个节点，template 与 taskSpec 必须且只能设置一个。
// +kubebuilder:validation:XValidation:rule="has(self.template) != has(self.taskSpec)",message="template 与 taskSpec 必须且只能设置一个"
type WorkflowTask struct {
	// Name 是节点名，同时用于拼接子 Task 名称 <workflow>-<node>。
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	Name string `json:"name"`

	// DependsOn 列出前置节点；默认要求它们全部成功后本节点才执行。
	// +listType=set
	// +kubebuilder:validation:MaxItems=50
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// Template 引用 spec.taskTemplates 中的命名模板，与 taskSpec 二选一。
	// +kubebuilder:validation:MinLength=1
	// +optional
	Template string `json:"template,omitempty"`

	// TaskSpec 内联执行规格，与 template 二选一。
	// 仅允许 OneTime 类型，且不允许 schedule / delay / suspend。
	// +kubebuilder:validation:XValidation:rule="!has(self.type) || self.type == 'OneTime'",message="taskSpec.type 必须为 OneTime"
	// +kubebuilder:validation:XValidation:rule="!has(self.schedule)",message="taskSpec 不允许设置 schedule"
	// +kubebuilder:validation:XValidation:rule="!has(self.delay)",message="taskSpec 不允许设置 delay"
	// +kubebuilder:validation:XValidation:rule="!has(self.suspend) || !self.suspend",message="taskSpec 不允许设置 suspend"
	// +optional
	TaskSpec *TaskSpec `json:"taskSpec,omitempty"`

	// RunOnFailure 表示所有依赖到达终态且至少一个失败（Failed/Skipped）时才执行本节点；
	// 若依赖全部成功，则本节点记为 Skipped。
	// +optional
	RunOnFailure bool `json:"runOnFailure,omitempty"`
}

// WorkflowNodeStatus 记录单个 DAG 节点的状态。
type WorkflowNodeStatus struct {
	// Name 是节点名。
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`

	// Phase 是节点的当前阶段。
	// +optional
	Phase WorkflowNodePhase `json:"phase,omitempty"`

	// TaskName 是本节点创建的子 Task 名称。
	// +optional
	TaskName string `json:"taskName,omitempty"`

	// StartTime 记录节点开始执行的时间。
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime 记录节点完成的时间。
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Message 是人类可读的状态说明，通常包含最近的错误原因。
	// +optional
	Message string `json:"message,omitempty"`
}

// WorkflowStatus 定义 Workflow 的观测状态。
type WorkflowStatus struct {
	// Phase 是 Workflow 的整体阶段。
	// +optional
	Phase WorkflowPhase `json:"phase,omitempty"`

	// Message 是人类可读的状态说明，通常包含最近的错误原因。
	// +optional
	Message string `json:"message,omitempty"`

	// StartTime 记录 Workflow 开始执行的时间。
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime 记录 Workflow 完成的时间。
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// ObservedGeneration 是 Controller 最近处理的 spec generation。
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions 代表资源的当前状态，遵循 K8s API 惯例。
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Nodes 保存每个 DAG 节点的状态。
	// +listType=map
	// +listMapKey=name
	// +optional
	Nodes []WorkflowNodeStatus `json:"nodes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=wf
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Workflow 定义一次扁平 DAG 编排的执行。
type Workflow struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec WorkflowSpec `json:"spec"`

	// +optional
	Status WorkflowStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WorkflowList 是 Workflow 资源的列表。
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Workflow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workflow{}, &WorkflowList{})
}
