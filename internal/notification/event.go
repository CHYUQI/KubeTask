// Package notification delivers best-effort Task execution failure webhooks.
package notification

import (
	"context"
	"time"
	"unicode/utf8"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

const (
	eventFailed  = "task.failed"
	eventTimeout = "task.timeout"
	messageLimit = 4096
)

// Event is the version 1 outbound webhook contract. Optional context may be
// absent when the Task has already been deleted or cannot be read.
type Event struct {
	Version    string        `json:"version"`
	ID         string        `json:"eventId"`
	Type       string        `json:"type"`
	OccurredAt *metav1.Time  `json:"occurredAt,omitempty"`
	ObservedAt metav1.Time   `json:"observedAt"`
	Task       TaskInfo      `json:"task"`
	Execution  ExecutionInfo `json:"execution"`
	Workflow   *WorkflowInfo `json:"workflow,omitempty"`
}

// TaskInfo identifies the owner of one Job execution.
type TaskInfo struct {
	Name string              `json:"name"`
	UID  types.UID           `json:"uid"`
	Type kubetaskv1.TaskType `json:"type,omitempty"`
}

// ExecutionInfo contains the observed terminal Job condition, without its spec.
type ExecutionInfo struct {
	JobName          string    `json:"jobName"`
	JobUID           types.UID `json:"jobUid"`
	Namespace        string    `json:"namespace"`
	Reason           string    `json:"reason"`
	Message          string    `json:"message"`
	MessageTruncated bool      `json:"messageTruncated,omitempty"`
}

// WorkflowInfo links a child Task back to its workflow run.
type WorkflowInfo struct {
	Name string    `json:"name"`
	UID  types.UID `json:"uid"`
	Node string    `json:"node,omitempty"`
}

func failureCondition(job *batchv1.Job) *batchv1.JobCondition {
	for i := range job.Status.Conditions {
		c := &job.Status.Conditions[i]
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return c
		}
	}
	return nil
}

func ownerIs(owner *metav1.OwnerReference, kind string) bool {
	if owner == nil || owner.Kind != kind || owner.UID == "" || owner.Name == "" {
		return false
	}
	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	return err == nil && gv.Group == kubetaskv1.GroupVersion.Group
}

func snapshot(job *batchv1.Job, observed time.Time) (Event, bool) {
	c := failureCondition(job)
	owner := metav1.GetControllerOf(job)
	if c == nil || job.UID == "" || !job.DeletionTimestamp.IsZero() || !ownerIs(owner, "Task") {
		return Event{}, false
	}
	e := Event{
		Version: "1", ID: "job:" + string(job.UID) + ":failed", Type: eventFailed,
		ObservedAt: metav1.NewTime(observed.UTC()),
		Task:       TaskInfo{Name: owner.Name, UID: owner.UID},
		Execution: ExecutionInfo{
			JobName: job.Name, JobUID: job.UID, Namespace: job.Namespace,
			Reason: c.Reason, Message: c.Message,
		},
	}
	if c.Reason == "DeadlineExceeded" {
		e.Type = eventTimeout
	}
	if !c.LastTransitionTime.IsZero() {
		e.OccurredAt = c.LastTransitionTime.DeepCopy()
	}
	if len(e.Execution.Message) > messageLimit {
		end := messageLimit
		for !utf8.RuneStart(e.Execution.Message[end]) {
			end--
		}
		e.Execution.Message = e.Execution.Message[:end]
		e.Execution.MessageTruncated = true
	}
	return e, true
}

func enrich(ctx context.Context, reader client.Reader, e *Event) {
	task := &kubetaskv1.Task{}
	if err := reader.Get(ctx, client.ObjectKey{Name: e.Task.Name}, task); err != nil || task.UID != e.Task.UID {
		return
	}
	e.Task.Type = task.Spec.Type
	owner := metav1.GetControllerOf(task)
	if ownerIs(owner, "Workflow") {
		e.Workflow = &WorkflowInfo{
			Name: owner.Name, UID: owner.UID, Node: task.Labels["kubetask.io/workflow-node"],
		}
	}
}
