package notification

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

const (
	testTaskName     = "test-task"
	testTaskUID      = "task-uid"
	testJobName      = "test-job"
	testJobUID       = "job-uid"
	testWorkflowName = "test-workflow"
	testWorkflowUID  = "workflow-uid"
	testWorkflowKind = "Workflow"
	testNode         = "export"
	testEventID      = "job:job-uid:failed"
)

func failedJob() *batchv1.Job {
	return &batchv1.Job{
		Name: testJobName, Namespace: "default", UID: testJobUID,
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: kubetaskv1.GroupVersion.String(), Kind: "Task",
			Name: testTaskName, UID: testTaskUID, Controller: new(true),
		}},
		Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers:    []corev1.Container{{Name: "task", Image: "busybox:1.37"}},
		}}},
		Status: batchv1.JobStatus{StartTime: new(metav1.Now()), Failed: 1, Conditions: []batchv1.JobCondition{{
			Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
			Reason: "BackoffLimitExceeded", Message: "retry limit reached", LastTransitionTime: metav1.Now(),
		}, {
			Type: batchv1.JobFailureTarget, Status: corev1.ConditionTrue,
			Reason: "BackoffLimitExceeded", LastTransitionTime: metav1.Now(),
		}}},
	}
}

func testEvent(t *testing.T) Event {
	t.Helper()
	e, ok := snapshot(failedJob(), time.Now())
	if !ok {
		t.Fatal("expected failure snapshot")
	}
	return e
}

func TestSnapshotEligibility(t *testing.T) {
	for _, tt := range []struct {
		name     string
		mutate   func(*batchv1.Job)
		wantType string
	}{
		{name: "failed", wantType: eventFailed},
		{name: "timeout", mutate: func(j *batchv1.Job) { j.Status.Conditions[0].Reason = "DeadlineExceeded" }, wantType: eventTimeout},
		{name: "pod retry", mutate: func(j *batchv1.Job) { j.Status.Conditions = nil; j.Status.Failed = 1 }},
		{name: "failure target", mutate: func(j *batchv1.Job) { j.Status.Conditions[0].Type = batchv1.JobFailureTarget }},
		{name: "success", mutate: func(j *batchv1.Job) { j.Status.Conditions[0].Type = batchv1.JobComplete }},
		{name: "false failure", mutate: func(j *batchv1.Job) { j.Status.Conditions[0].Status = corev1.ConditionFalse }},
		{name: "label alone", mutate: func(j *batchv1.Job) {
			j.OwnerReferences = nil
			j.Labels = map[string]string{"kubetask.io/task": testTaskName}
		}},
		{name: "foreign group", mutate: func(j *batchv1.Job) { j.OwnerReferences[0].APIVersion = "other.io/v1" }},
		{name: "wrong kind", mutate: func(j *batchv1.Job) { j.OwnerReferences[0].Kind = testWorkflowKind }},
		{name: "not controller", mutate: func(j *batchv1.Job) { j.OwnerReferences[0].Controller = new(false) }},
		{name: "deleting", mutate: func(j *batchv1.Job) { j.DeletionTimestamp = new(metav1.Now()) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			job := failedJob()
			if tt.mutate != nil {
				tt.mutate(job)
			}
			e, ok := snapshot(job, time.Now())
			if ok != (tt.wantType != "") || e.Type != tt.wantType {
				t.Fatalf("snapshot eligibility=%v type=%q, want %q", ok, e.Type, tt.wantType)
			}
			if ok && (e.ID != testEventID || e.Task.UID != testTaskUID || e.Execution.JobUID != testJobUID) {
				t.Fatalf("unexpected identity: %+v", e)
			}
		})
	}
}

func TestSnapshotCopiesConditionAndTruncatesUTF8(t *testing.T) {
	job := failedJob()
	job.Status.Conditions[0].Message = strings.Repeat("错", 2000)
	e, ok := snapshot(job, time.Now())
	if !ok || !e.Execution.MessageTruncated || len(e.Execution.Message) > messageLimit || !utf8.ValidString(e.Execution.Message) {
		t.Fatal("expected bounded valid UTF-8 message")
	}
	before := *e.OccurredAt
	job.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now().Add(time.Hour))
	if !e.OccurredAt.Equal(&before) {
		t.Fatal("snapshot still aliases informer data")
	}
	job.Status.Conditions[0].LastTransitionTime = metav1.Time{}
	e, _ = snapshot(job, time.Now())
	if e.OccurredAt != nil {
		t.Fatal("missing condition timestamp must be omitted")
	}
}

func TestEnrichmentChecksOwnerUID(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := kubetaskv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	task := &kubetaskv1.Task{
		Name: testTaskName, UID: testTaskUID,
		Labels: map[string]string{"kubetask.io/workflow-node": testNode},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: kubetaskv1.GroupVersion.String(), Kind: testWorkflowKind, Name: testWorkflowName,
			UID: testWorkflowUID, Controller: new(true),
		}},
		Spec: kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime},
	}
	for _, tt := range []struct {
		name            string
		sameUID, exists bool
	}{
		{name: "workflow child", sameUID: true, exists: true},
		{name: "same name replacement", exists: true},
		{name: "deleted task"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.exists {
				builder = builder.WithObjects(task)
			}
			e := testEvent(t)
			if !tt.sameUID {
				e.Task.UID = "previous-task-uid"
			}
			enrich(context.Background(), builder.Build(), &e)
			if tt.sameUID {
				if e.Workflow == nil || e.Workflow.UID != testWorkflowUID || e.Workflow.Node != testNode || e.Task.Type != kubetaskv1.TaskTypeOneTime {
					t.Fatalf("missing context: %+v", e)
				}
			} else if e.Workflow != nil || e.Task.Type != "" {
				t.Fatal("enriched a deleted or different Task")
			}
		})
	}
}
