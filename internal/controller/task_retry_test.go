package controller

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

const (
	testTestNode = "test"
)

const (
	testBuildNode    = "build"
	testTestTemplate = "go-test"
)

var _ = Describe("Job retry status", func() {
	DescribeTable("records only the final Job outcome",
		func(taskType kubetaskv1.TaskType, succeeds bool) {
			name := strings.ToLower(fmt.Sprintf("retry-%s-%t", taskType, succeeds))
			defer cleanupTask(name)

			task := newOneTimeTask(name)
			if taskType == kubetaskv1.TaskTypeDelay {
				task = newDelayTask(name, "0s")
			}
			createAndInitTask(task)
			if taskType == kubetaskv1.TaskTypeCron {
				// Keep a current execution alongside a previous successful Cron run.
				task = getTask(name)
				task.Spec.Type = taskType
				task.Spec.Schedule = "0 0 1 1 *"
				Expect(k8sClient.Update(ctx, task)).To(Succeed())
				task.Status.Succeeded = 1
				task.Status.ExecutionHistory = []kubetaskv1.ExecutionRecord{{
					JobName: "previous-run", Phase: kubetaskv1.TaskSucceeded, StartTime: metav1.Now(),
				}}
				Expect(k8sClient.Status().Update(ctx, task)).To(Succeed())
			}
			previous := getTask(name).Status.DeepCopy()
			jobs := listJobs(name)
			Expect(jobs.Items).To(HaveLen(1))
			job := &jobs.Items[0]

			// No active Pod during retry backoff still means the Job is running.
			for _, active := range []int32{0, 1, 0} {
				job.Status.Failed = 2
				job.Status.Active = active
				Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
				reconcileUntilStable(newTestReconciler(), name)
				stored := getTask(name)
				Expect(stored.Status.Phase).To(Equal(kubetaskv1.TaskRunning))
				Expect(stored.Status.ActiveJobs).To(ConsistOf(job.Name))
				Expect(stored.Status.ExecutionHistory).To(Equal(previous.ExecutionHistory))
				Expect(stored.Status.Failed).To(Equal(previous.Failed))
				Expect(stored.Status.Succeeded).To(Equal(previous.Succeeded))
				Expect(stored.Status.LastCompletionTime).To(BeNil())
			}

			phase := kubetaskv1.TaskFailed
			if succeeds {
				// Even a successful Pod count is not a terminal Job condition.
				job.Status.Succeeded = 1
				Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
				reconcileUntilStable(newTestReconciler(), name)
				Expect(getTask(name).Status.Phase).To(Equal(kubetaskv1.TaskRunning))
				markJobComplete(job)
				phase = kubetaskv1.TaskSucceeded
			} else {
				job.Status.Failed = 4 // backoffLimit=3, four failed Pod attempts
				markJobFailed(job)
			}
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
			reconcileUntilStable(newTestReconciler(), name)

			stored := getTask(name)
			Expect(stored.Status.ActiveJobs).To(BeEmpty())
			Expect(stored.Status.ExecutionHistory).To(HaveLen(len(previous.ExecutionHistory) + 1))
			record := stored.Status.ExecutionHistory[len(stored.Status.ExecutionHistory)-1]
			Expect(record.JobName).To(Equal(job.Name))
			Expect(record.Phase).To(Equal(phase))
			if succeeds {
				Expect(stored.Status.Phase).To(Equal(kubetaskv1.TaskSucceeded))
				Expect(stored.Status.Succeeded).To(Equal(previous.Succeeded + 1))
				Expect(stored.Status.Failed).To(Equal(previous.Failed))
			} else {
				Expect(stored.Status.Failed).To(Equal(previous.Failed + 1))
				Expect(stored.Status.Succeeded).To(Equal(previous.Succeeded))
				if taskType != kubetaskv1.TaskTypeCron {
					Expect(stored.Status.Phase).To(Equal(kubetaskv1.TaskFailed))
				}
			}
			// Reconciliation must not duplicate records or execution counters.
			reconcileUntilStable(newTestReconciler(), name)
			Expect(getTask(name).Status).To(Equal(stored.Status))
		},
		Entry("OneTime retry succeeds", kubetaskv1.TaskTypeOneTime, true),
		Entry("OneTime retry exhausts backoff", kubetaskv1.TaskTypeOneTime, false),
		Entry("Delay retry succeeds", kubetaskv1.TaskTypeDelay, true),
		Entry("Delay retry exhausts backoff", kubetaskv1.TaskTypeDelay, false),
		Entry("Cron retry succeeds", kubetaskv1.TaskTypeCron, true),
		Entry("Cron retry exhausts backoff", kubetaskv1.TaskTypeCron, false),
	)

	DescribeTable("keeps Workflow downstream nodes waiting for the final Job outcome",
		func(succeeds bool) {
			name := fmt.Sprintf("wf-job-retry-%t", succeeds)
			defer cleanupWorkflow(name)
			// envtest has no garbage collector; clean Jobs and Task finalizers explicitly.
			defer cleanupTask(name + "-build")
			defer cleanupTask(name + "-test")
			defer cleanupTask(name + "-failure")
			wf := newWorkflow(name)
			wf.Spec.Tasks = append(wf.Spec.Tasks, kubetaskv1.WorkflowTask{
				Name: "failure", DependsOn: []string{testBuildNode}, Template: testTestTemplate, RunOnFailure: true,
			})
			createWorkflow(wf)
			reconcileWorkflow(name, 1)
			reconcileUntilStable(newTestReconciler(), name+"-build")
			jobs := listJobs(name + "-build")
			Expect(jobs.Items).To(HaveLen(1))
			job := &jobs.Items[0]
			job.Status.Failed = 2
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
			reconcileUntilStable(newTestReconciler(), name+"-build")
			reconcileWorkflow(name, 3)
			stored := getWorkflow(name)
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowRunning))
			Expect(workflowNode(stored, testBuildNode).Phase).To(Equal(kubetaskv1.NodeRunning))
			Expect(workflowNode(stored, testTestNode).Phase).To(Equal(kubetaskv1.NodePending))
			Expect(workflowNode(stored, "failure").Phase).To(Equal(kubetaskv1.NodePending))
			Expect(listWorkflowChildren(name)).To(HaveLen(1))

			next := testTestNode
			finalPhase := kubetaskv1.WorkflowSucceeded
			if succeeds {
				markJobComplete(job)
			} else {
				job.Status.Failed = 4
				markJobFailed(job)
				next = "failure"
				finalPhase = kubetaskv1.WorkflowFailed
			}
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
			reconcileUntilStable(newTestReconciler(), name+"-build")
			reconcileWorkflow(name, 3)
			stored = getWorkflow(name)
			Expect(workflowNode(stored, next).Phase).To(Equal(kubetaskv1.NodeRunning))
			if succeeds {
				Expect(workflowNode(stored, testBuildNode).Phase).To(Equal(kubetaskv1.NodeSucceeded))
				Expect(workflowNode(stored, "failure").Phase).To(Equal(kubetaskv1.NodeSkipped))
			} else {
				Expect(workflowNode(stored, testBuildNode).Phase).To(Equal(kubetaskv1.NodeFailed))
				Expect(workflowNode(stored, testTestNode).Phase).To(Equal(kubetaskv1.NodeSkipped))
			}
			reconcileUntilStable(newTestReconciler(), name+"-"+next)
			nextJobs := listJobs(name + "-" + next)
			Expect(nextJobs.Items).To(HaveLen(1))
			markJobComplete(&nextJobs.Items[0])
			Expect(k8sClient.Status().Update(ctx, &nextJobs.Items[0])).To(Succeed())
			reconcileUntilStable(newTestReconciler(), name+"-"+next)
			reconcileWorkflow(name, 3)
			Expect(getWorkflow(name).Status.Phase).To(Equal(finalPhase))
		},
		Entry("retry succeeds and ordinary downstream runs", true),
		Entry("retry exhausts and failure downstream runs", false),
	)
})

func markJobFailed(job *batchv1.Job) {
	now := metav1.Now()
	job.Status.StartTime = &now
	job.Status.Active = 0
	for _, condition := range []batchv1.JobConditionType{batchv1.JobFailureTarget, batchv1.JobFailed} {
		job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
			Type: condition, Status: corev1.ConditionTrue,
			Reason: "BackoffLimitExceeded", LastProbeTime: now, LastTransitionTime: now,
		})
	}
}
