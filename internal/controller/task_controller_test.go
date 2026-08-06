package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

// ---------------------------------------------------------------------------
// Helper: create a Reconciler instance backed by the envtest client
// ---------------------------------------------------------------------------
func newTestReconciler() *TaskReconciler {
	return &TaskReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
}

// ---------------------------------------------------------------------------
// Helper: reconcile N times and return the last result
// ---------------------------------------------------------------------------
func reconcileUntilStable(r *TaskReconciler, taskName string) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
	for range 5 {
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
	}
}

// ---------------------------------------------------------------------------
// Helper: create a Task and run initial reconciles (Finalizer registration)
// ---------------------------------------------------------------------------
func createAndInitTask(task *kubetaskv1.Task) {
	Expect(k8sClient.Create(ctx, task)).To(Succeed())
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: task.Name}}
	r := newTestReconciler()
	// 1st reconcile: add Finalizer → update
	_, err := r.Reconcile(ctx, req)
	Expect(err).NotTo(HaveOccurred())
	// 2nd reconcile: Finalizer exists → proceed to business logic
	_, err = r.Reconcile(ctx, req)
	Expect(err).NotTo(HaveOccurred())
}

// ---------------------------------------------------------------------------
// Suite
// ---------------------------------------------------------------------------
var _ = Describe("Task Controller", func() {

	// =========================================================================
	// OneTime
	// =========================================================================
	Context("OneTime task", func() {
		taskName := "onetime-test"

		AfterEach(func() {
			cleanupTask(taskName)
		})

		It("should create a Job and set Phase=Running", func() {
			task := newOneTimeTask(taskName)
			createAndInitTask(task)

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskRunning))
			Expect(task.Status.ActiveJobs).To(HaveLen(1))
			Expect(task.Status.LastStartTime).NotTo(BeNil())

			// Verify a real Job exists
			jobs := listJobs(taskName)
			Expect(jobs.Items).To(HaveLen(1))
		})

		It("should NOT create a second Job on re-reconcile", func() {
			task := newOneTimeTask(taskName)
			createAndInitTask(task)

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			jobs := listJobs(taskName)
			Expect(jobs.Items).To(HaveLen(1), "OneTime should only run once")
		})

		It("should not re-create a Job after completion and Job cleanup", func() {
			task := newOneTimeTask(taskName)
			createAndInitTask(task)

			jobs := listJobs(taskName)
			Expect(jobs.Items).To(HaveLen(1))
			job := &jobs.Items[0]
			markJobComplete(job)
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskSucceeded))

			job.Finalizers = nil
			Expect(k8sClient.Update(ctx, job)).To(Succeed())
			_ = k8sClient.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: job.Name}, &batchv1.Job{})
				return apierrors.IsNotFound(err)
			}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

			_, err = r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(listJobs(taskName).Items).To(BeEmpty())
		})

		It("should add Finalizer on first reconcile", func() {
			task := newOneTimeTask(taskName)
			Expect(k8sClient.Create(ctx, task)).To(Succeed())

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			task = getTask(taskName)
			Expect(controllerutil.ContainsFinalizer(task, taskFinalizer)).To(BeTrue())
		})
	})

	// =========================================================================
	// Cron
	// =========================================================================
	Context("Cron task", func() {
		taskName := "cron-test"

		AfterEach(func() {
			cleanupTask(taskName)
		})

		It("should mark Failed on invalid cron expression", func() {
			task := newCronTask(taskName, "invalid-cron")
			createAndInitTask(task)

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskFailed))
			Expect(task.Status.Message).To(ContainSubstring("Invalid cron expression"))
		})

		It("should requeue when schedule is in the future", func() {
			task := &kubetaskv1.Task{
				ObjectMeta: metav1.ObjectMeta{Name: taskName},
				Spec: kubetaskv1.TaskSpec{
					Type:     kubetaskv1.TaskTypeCron,
					Image:    "busybox:latest",
					Command:  []string{"echo", "cron"},
					Schedule: "0 0 1 1 *",
				},
			}
			createAndInitTask(task)

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			jobs := listJobs(taskName)
			Expect(jobs.Items).To(BeEmpty(), "should not create Job before schedule")
		})

		It("should set LastScheduleTime when Job is triggered", func() {
			task := newOneTimeTask(taskName)
			task.Spec.Type = kubetaskv1.TaskTypeCron
			task.Spec.Schedule = "* * * * *"
			createAndInitTask(task)

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			// "* * * * *" fires every minute - nextTime is the next minute boundary
			// If RequeueAfter is set, we haven't reached the boundary yet
			// If RequeueAfter is 0, the job was created
			if result.RequeueAfter == 0 {
				task = getTask(taskName)
				Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskRunning))
				Expect(task.Status.LastScheduleTime).NotTo(BeNil())
			} else {
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			}
		})

		It("should sync Job status into Task status", func() {
			task := newCronTask(taskName, "0 0 1 1 *")
			createAndInitTask(task)

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      taskName + "-sync",
					Namespace: "default",
					Labels:    map[string]string{"kubetask.io/task": taskName},
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers:    []corev1.Container{{Name: "task", Image: "busybox"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())
			markJobComplete(job)
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskSucceeded))
			Expect(task.Status.Succeeded).To(Equal(int32(1)))
			Expect(task.Status.LastCompletionTime).NotTo(BeNil())
			Expect(task.Status.ExecutionHistory).To(HaveLen(1))
		})
	})

	// =========================================================================
	// Delay
	// =========================================================================
	Context("Delay task", func() {
		It("should requeue when delay not yet passed", func() {
			taskName := "delay-future"
			defer cleanupTask(taskName)

			task := newDelayTask(taskName, "1h")
			createAndInitTask(task)

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("should create Job when delay has passed", func() {
			taskName := "delay-now"
			defer cleanupTask(taskName)

			task := newDelayTask(taskName, "0s")
			createAndInitTask(task)

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskRunning))
			Expect(task.Status.ActiveJobs).To(HaveLen(1))
		})

		It("should mark Failed when delay field is missing", func() {
			taskName := "delay-missing"
			defer cleanupTask(taskName)

			task := newOneTimeTask(taskName)
			task.Spec.Type = kubetaskv1.TaskTypeDelay
			task.Spec.Delay = nil
			createAndInitTask(task)

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskFailed))
			Expect(task.Status.Message).To(ContainSubstring("requires spec.delay"))
		})

		It("should NOT create a second Job for delay task", func() {
			taskName := "delay-once"
			defer cleanupTask(taskName)

			task := newDelayTask(taskName, "0s")
			createAndInitTask(task)

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			jobs := listJobs(taskName)
			Expect(jobs.Items).To(HaveLen(1), "Delay should only execute once")
		})

		It("should not re-create a Job after completion and Job cleanup", func() {
			taskName := "delay-finished"
			defer cleanupTask(taskName)

			task := newDelayTask(taskName, "0s")
			createAndInitTask(task)

			jobs := listJobs(taskName)
			Expect(jobs.Items).To(HaveLen(1))
			job := &jobs.Items[0]
			markJobComplete(job)
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskSucceeded))

			job.Finalizers = nil
			Expect(k8sClient.Update(ctx, job)).To(Succeed())
			_ = k8sClient.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: job.Name}, &batchv1.Job{})
				return apierrors.IsNotFound(err)
			}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

			_, err = r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(listJobs(taskName).Items).To(BeEmpty())
		})
	})

	// =========================================================================
	// Suspend
	// =========================================================================
	Context("Suspend", func() {
		taskName := "suspend-test"

		AfterEach(func() {
			cleanupTask(taskName)
		})

		It("should set Phase=Suspended and not create Job", func() {
			task := newOneTimeTask(taskName)
			task.Spec.Suspend = boolPtr(true)
			Expect(k8sClient.Create(ctx, task)).To(Succeed())

			reconcileUntilStable(newTestReconciler(), taskName)

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskSuspended))

			jobs := listJobs(taskName)
			Expect(jobs.Items).To(BeEmpty(), "suspended task should not create Jobs")
		})
	})

	// =========================================================================
	// Deletion
	// =========================================================================
	Context("Deletion", func() {
		taskName := "delete-test"

		AfterEach(func() {
			cleanupTask(taskName)
		})

		It("should remove Finalizer after cleaning up Jobs", func() {
			task := newOneTimeTask(taskName)
			createAndInitTask(task)

			Expect(k8sClient.Delete(ctx, task)).To(Succeed())

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}

			// First reconciliation after delete: should delete the job
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Wait briefly then reconcile again
			time.Sleep(100 * time.Millisecond)

			_, err = r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Finalizer should be gone, Task should be fully deleted
			task = &kubetaskv1.Task{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: taskName}, task)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	// =========================================================================
	// constructJob
	// =========================================================================
	Context("constructJob", func() {
		It("should build a valid Job from Task spec", func() {
			task := &kubetaskv1.Task{
				ObjectMeta: metav1.ObjectMeta{Name: "construct-test"},
				Spec: kubetaskv1.TaskSpec{
					Type:                    kubetaskv1.TaskTypeOneTime,
					Image:                   "alpine:3.20",
					Command:                 []string{"/bin/sh", "-c"},
					Args:                    []string{"echo done"},
					BackoffLimit:            int32Ptr(5),
					ActiveDeadlineSeconds:   int64Ptr(300),
					TTLSecondsAfterFinished: int32Ptr(60),
					Resources:               corev1.ResourceRequirements{},
				},
			}

			r := newTestReconciler()
			job, err := r.constructJob(task)
			Expect(err).NotTo(HaveOccurred())

			Expect(job.Name).To(ContainSubstring("construct-test-"))
			Expect(job.Spec.Template.Spec.Containers[0].Image).To(Equal("alpine:3.20"))
			Expect(job.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"/bin/sh", "-c"}))
			Expect(job.Spec.Template.Spec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))
			Expect(*job.Spec.BackoffLimit).To(Equal(int32(5)))
			Expect(*job.Spec.ActiveDeadlineSeconds).To(Equal(int64(300)))
			Expect(*job.Spec.TTLSecondsAfterFinished).To(Equal(int32(60)))

			// OwnerReference should point to the Task
			Expect(job.OwnerReferences).To(HaveLen(1))
			Expect(job.OwnerReferences[0].Name).To(Equal("construct-test"))
			Expect(job.OwnerReferences[0].Controller).NotTo(BeNil())
			Expect(*job.OwnerReferences[0].Controller).To(BeTrue())
		})

		It("should use default BackoffLimit when not specified", func() {
			task := &kubetaskv1.Task{
				ObjectMeta: metav1.ObjectMeta{Name: "default-bol"},
				Spec: kubetaskv1.TaskSpec{
					Type:  kubetaskv1.TaskTypeOneTime,
					Image: "busybox",
				},
			}

			r := newTestReconciler()
			job, err := r.constructJob(task)
			Expect(err).NotTo(HaveOccurred())
			Expect(*job.Spec.BackoffLimit).To(Equal(int32(3)))
		})
	})

	// =========================================================================
	// syncJobStatus
	// =========================================================================
	Context("syncJobStatus", func() {
		It("should keep Phase=Running while Job is active", func() {
			taskName := "sync-active"
			defer cleanupTask(taskName)

			task := newOneTimeTask(taskName)
			createAndInitTask(task)

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskRunning))
		})

		It("should update Succeeded counter when Job completes", func() {
			taskName := "sync-succeed"
			defer cleanupTask(taskName)

			task := newOneTimeTask(taskName)
			createAndInitTask(task)

			jobs := listJobs(taskName)
			Expect(jobs.Items).To(HaveLen(1))

			job := &jobs.Items[0]
			markJobComplete(job)
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskSucceeded))
			Expect(task.Status.Succeeded).To(Equal(int32(1)))
			Expect(task.Status.LastCompletionTime).NotTo(BeNil())
			Expect(task.Status.ExecutionHistory).To(HaveLen(1))
			Expect(task.Status.ExecutionHistory[0].Phase).To(Equal(kubetaskv1.TaskSucceeded))
		})

		It("should update Failed counter when Job fails", func() {
			taskName := "sync-fail"
			defer cleanupTask(taskName)

			task := newOneTimeTask(taskName)
			createAndInitTask(task)

			jobs := listJobs(taskName)
			Expect(jobs.Items).To(HaveLen(1))

			job := &jobs.Items[0]
			job.Status.Failed = 1
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			task = getTask(taskName)
			Expect(task.Status.Phase).To(Equal(kubetaskv1.TaskFailed))
			Expect(task.Status.Failed).To(Equal(int32(1)))
			Expect(task.Status.ExecutionHistory).To(HaveLen(1))
			Expect(task.Status.ExecutionHistory[0].Phase).To(Equal(kubetaskv1.TaskFailed))
		})

		It("should trim ExecutionHistory to max 10 entries", func() {
			taskName := "sync-trim"
			defer cleanupTask(taskName)

			task := newOneTimeTask(taskName)
			createAndInitTask(task)

			task = getTask(taskName)
			now := metav1.Now()
			for range 12 {
				task.Status.ExecutionHistory = append(task.Status.ExecutionHistory, kubetaskv1.ExecutionRecord{
					JobName:   "old-job",
					Phase:     kubetaskv1.TaskSucceeded,
					StartTime: now,
				})
			}
			Expect(k8sClient.Status().Update(ctx, task)).To(Succeed())

			jobs := listJobs(taskName)
			Expect(jobs.Items).To(HaveLen(1))
			job := &jobs.Items[0]
			markJobComplete(job)
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: taskName}}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			task = getTask(taskName)
			Expect(len(task.Status.ExecutionHistory)).To(BeNumerically("<=", 10))
		})
	})

	// =========================================================================
	// IsNotFound
	// =========================================================================
	Context("IsNotFound", func() {
		It("should return nil without error for non-existent Task", func() {
			r := newTestReconciler()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "does-not-exist"}}
			result, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})
})

// =============================================================================
// Test helpers
// =============================================================================

func newOneTimeTask(name string) *kubetaskv1.Task {
	return &kubetaskv1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kubetaskv1.TaskSpec{
			Type:    kubetaskv1.TaskTypeOneTime,
			Image:   "busybox:latest",
			Command: []string{"echo", "hello"},
		},
	}
}

func newCronTask(name, schedule string) *kubetaskv1.Task {
	return &kubetaskv1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kubetaskv1.TaskSpec{
			Type:     kubetaskv1.TaskTypeCron,
			Image:    "busybox:latest",
			Command:  []string{"echo", "cron"},
			Schedule: schedule,
		},
	}
}

func newDelayTask(name, delay string) *kubetaskv1.Task {
	d, _ := time.ParseDuration(delay)
	return &kubetaskv1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kubetaskv1.TaskSpec{
			Type:    kubetaskv1.TaskTypeDelay,
			Image:   "busybox:latest",
			Command: []string{"echo", "delayed"},
			Delay:   &metav1.Duration{Duration: d},
		},
	}
}

func getTask(name string) *kubetaskv1.Task {
	task := &kubetaskv1.Task{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, task)
	Expect(err).NotTo(HaveOccurred())
	return task
}

func listJobs(taskName string) *batchv1.JobList {
	jobs := &batchv1.JobList{}
	err := k8sClient.List(ctx, jobs, client.MatchingLabels{"kubetask.io/task": taskName})
	Expect(err).NotTo(HaveOccurred())
	return jobs
}

func cleanupTask(name string) {
	task := &kubetaskv1.Task{ObjectMeta: metav1.ObjectMeta{Name: name}}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, task)
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(task, taskFinalizer) {
		controllerutil.RemoveFinalizer(task, taskFinalizer)
		_ = k8sClient.Update(ctx, task)
	}
	_ = k8sClient.Delete(ctx, task)
	// Delete orphan jobs
	jobs := &batchv1.JobList{}
	_ = k8sClient.List(ctx, jobs, client.MatchingLabels{"kubetask.io/task": name})
	for i := range jobs.Items {
		_ = k8sClient.Delete(ctx, &jobs.Items[i])
	}
}

func int32Ptr(v int32) *int32 { return &v }
func int64Ptr(v int64) *int64 { return &v }
func boolPtr(v bool) *bool    { return &v }

func markJobComplete(job *batchv1.Job) {
	now := metav1.Now()
	job.Status.StartTime = &now
	job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
		Type:               batchv1.JobSuccessCriteriaMet,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      now,
		LastTransitionTime: now,
	})
	job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{
		Type:               batchv1.JobComplete,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      now,
		LastTransitionTime: now,
	})
	job.Status.Succeeded = 1
	job.Status.CompletionTime = &now
}
