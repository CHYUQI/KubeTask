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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TaskType 定义任务的执行模式。
// +kubebuilder:validation:Enum=Cron;OneTime;Delay
type TaskType string

const (
	TaskTypeCron    TaskType = "Cron"
	TaskTypeOneTime TaskType = "OneTime"
	TaskTypeDelay   TaskType = "Delay"
)

// ConcurrencyPolicy 定义并发执行策略。
// +kubebuilder:validation:Enum=Allow;Forbid;Replace
type ConcurrencyPolicy string

const (
	ConcurrencyAllow   ConcurrencyPolicy = "Allow"
	ConcurrencyForbid  ConcurrencyPolicy = "Forbid"
	ConcurrencyReplace ConcurrencyPolicy = "Replace"
)

// TaskPhase 表示任务当前所处的生命周期阶段。
type TaskPhase string

const (
	TaskPending   TaskPhase = "Pending"
	TaskRunning   TaskPhase = "Running"
	TaskSucceeded TaskPhase = "Succeeded"
	TaskFailed    TaskPhase = "Failed"
	TaskSuspended TaskPhase = "Suspended"
)

// ExecutionRecord 记录单次执行的摘要信息。
type ExecutionRecord struct {
	JobName   string      `json:"jobName"`
	StartTime metav1.Time `json:"startTime"`
	StopTime  metav1.Time `json:"stopTime,omitempty"`
	Phase     TaskPhase   `json:"phase"`
	ExitCode  int32       `json:"exitCode,omitempty"`
}

// TaskSpec 定义 Task 的期望状态。
type TaskSpec struct {
	// Type 指定任务的执行模式。
	// +kubebuilder:validation:Required
	Type TaskType `json:"type"`

	// Schedule 是 Cron 表达式，仅在 Type=Cron 时生效。
	// 标准 5 字段格式：分 时 日 月 周。
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// Delay 指定延迟执行的时长，仅在 Type=Delay 时生效。
	// 格式如 "30s", "5m", "1h"。
	// +optional
	Delay *metav1.Duration `json:"delay,omitempty"`

	// Image 是任务执行的容器镜像。
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Command 是容器入口命令，覆盖镜像默认 ENTRYPOINT。
	// +optional
	Command []string `json:"command,omitempty"`

	// Args 是命令的附加参数。
	// +optional
	Args []string `json:"args,omitempty"`

	// Resources 指定容器的资源请求与限制。
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Env 注入容器的环境变量。
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// BackoffLimit 是失败后最大重试次数，默认 3。
	// +kubebuilder:default=3
	// +optional
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`

	// ActiveDeadlineSeconds 限制单次执行的最大时长（秒）。
	// 超时后 Job 会被强制终止。
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`

	// TTLSecondsAfterFinished 控制执行完成后 Job 的保留时间（秒）。
	// 过期后 Job 自动清理。
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// ConcurrencyPolicy 控制同一 Task 的并发执行行为。
	// +kubebuilder:default=Allow
	// +optional
	ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`

	// Suspend 为 true 时暂停调度，但不删除已运行的 Job。
	// +optional
	Suspend *bool `json:"suspend,omitempty"`

	// ClusterName 指定目标集群名称（多集群场景预留）。
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
}

// TaskStatus 定义 Task 的观测状态。
type TaskStatus struct {
	// Phase 是任务的当前阶段。
	// +optional
	Phase TaskPhase `json:"phase,omitempty"`

	// Message 是人类可读的状态信息，通常包含最近的错误原因。
	// +optional
	Message string `json:"message,omitempty"`

	// LastScheduleTime 记录最近一次被调度的时间。
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// LastStartTime 记录最近一次执行开始的时间。
	// +optional
	LastStartTime *metav1.Time `json:"lastStartTime,omitempty"`

	// LastCompletionTime 记录最近一次执行完成的时间。
	// +optional
	LastCompletionTime *metav1.Time `json:"lastCompletionTime,omitempty"`

	// ExecutionHistory 保存最近的执行记录（最多保留 10 条）。
	// +optional
	ExecutionHistory []ExecutionRecord `json:"executionHistory,omitempty"`

	// ActiveJobs 当前正在运行的关联 Job 名称。
	// +optional
	ActiveJobs []string `json:"activeJobs,omitempty"`

	// Failed 累计失败次数。
	// +optional
	Failed int32 `json:"failed,omitempty"`

	// Succeeded 累计成功次数。
	// +optional
	Succeeded int32 `json:"succeeded,omitempty"`

	// Conditions 代表资源的当前状态，遵循 K8s API 惯例。
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// Task 是 kubetask 任务调度系统的核心 API。
// 它定义了一个可在集群中按计划、一次性或延迟执行的任务。
type Task struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec TaskSpec `json:"spec"`

	// +optional
	Status TaskStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// TaskList 是 Task 资源的列表。
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Task `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Task{}, &TaskList{})
}
