package controller

import (
	"context"
	"fmt"
	"time"

	kubetaskv1 "kubetask.io/kubetask/api/v1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/robfig/cron/v3"
)

const taskFinalizer = "kubetask.kubetask.io/finalizer"

type TaskReconciler struct {
	//embed the client.Client interface to provide methods for interacting with the Kubernetes API server. 
	client.Client
	
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kubetask.kubetask.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubetask.kubetask.io,resources=tasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubetask.kubetask.io,resources=tasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *TaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// initialize logger
	log := log.FromContext(ctx)

	// Fetch the Task instance
	task := &kubetaskv1.Task{}
	if err := r.Get(ctx, req.NamespacedName, task); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Task")
		return ctrl.Result{}, err
	}

	// Log the current phase of the Task
	log.Info("Reconciling Task", "name", task.Name, "phase", task.Status.Phase)

	// handle deletion if the Task is marked for deletion
	if !task.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, task)
	}

	// handle finalizer: add if not present,for deletion handling
	if !controllerutil.ContainsFinalizer(task, taskFinalizer) {
		controllerutil.AddFinalizer(task, taskFinalizer)
		if err := r.Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Handle suspension 
	if task.Spec.Suspend != nil && *task.Spec.Suspend {
		return r.handleSuspend(ctx, task)
	}

	// handle 3 kinds of Task type
	switch task.Spec.Type {
	case kubetaskv1.TaskTypeOneTime:
		return r.handleOneTime(ctx, task)
	case kubetaskv1.TaskTypeCron:
		return r.handleCron(ctx, task)
	case kubetaskv1.TaskTypeDelay:
		return r.handleDelay(ctx, task)
	default:
		return ctrl.Result{}, nil
	}
}


func (r *TaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// return a new controller managed by the manager, watching for Task resources and owning Job resources
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubetaskv1.Task{}).
		Owns(&batchv1.Job{}).
		Named("task").
		Complete(r)
}

// =============================================================================
// Deletion & Suspend
// =============================================================================

func (r *TaskReconciler) handleDeletion(ctx context.Context, task *kubetaskv1.Task) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Handling Task deletion", "task", task.Name)

	jobs := &batchv1.JobList{}
	if err := r.List(ctx, jobs, client.MatchingLabels{"kubetask.io/task": task.Name}); err != nil {
		return ctrl.Result{}, err
	}

	for i := range jobs.Items {
		job := &jobs.Items[i]
		if job.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to delete Job", "job", job.Name)
					return ctrl.Result{}, err
				}
			}
			log.Info("Deleted Job for Task", "task", task.Name, "job", job.Name)
		}
	}

	jobs = &batchv1.JobList{}
	if err := r.List(ctx, jobs, client.MatchingLabels{"kubetask.io/task": task.Name}); err != nil {
		return ctrl.Result{}, err
	}

	if len(jobs.Items) > 0 {
		log.Info("Waiting for Jobs to be fully deleted", "task", task.Name, "remaining", len(jobs.Items))
		return ctrl.Result{RequeueAfter: time.Second * 2}, nil
	}

	controllerutil.RemoveFinalizer(task, taskFinalizer)
	if err := r.Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Task deletion completed", "task", task.Name)
	return ctrl.Result{}, nil
}

func (r *TaskReconciler) handleSuspend(ctx context.Context, task *kubetaskv1.Task) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if task.Status.Phase != kubetaskv1.TaskSuspended {
		task.Status.Phase = kubetaskv1.TaskSuspended
		task.Status.Message = "Task is suspended"
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info("Task is suspended, skipping", "task", task.Name)
	return ctrl.Result{}, nil
}

// =============================================================================
// Task type handlers
// =============================================================================

func (r *TaskReconciler) handleOneTime(ctx context.Context, task *kubetaskv1.Task) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if r.jobExists(ctx, task.Name) {
		log.Info("Job already exists for OneTime task", "task", task.Name)
		return r.syncJobStatus(ctx, task)
	}

	return r.createJobAndUpdateStatus(ctx, task)
}

func (r *TaskReconciler) handleCron(ctx context.Context, task *kubetaskv1.Task) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	schedule, err := cron.ParseStandard(task.Spec.Schedule)
	if err != nil {
		log.Error(err, "Invalid cron expression", "schedule", task.Spec.Schedule)
		task.Status.Phase = kubetaskv1.TaskFailed
		task.Status.Message = fmt.Sprintf("Invalid cron expression: %s", task.Spec.Schedule)
		r.Status().Update(ctx, task)
		return ctrl.Result{}, nil
	}

	now := time.Now()
	var nextTime time.Time
	if task.Status.LastScheduleTime == nil {
		nextTime = schedule.Next(now)
	} else {
		nextTime = schedule.Next(task.Status.LastScheduleTime.Time)
	}

	if now.Before(nextTime) {
		return ctrl.Result{RequeueAfter: nextTime.Sub(now)}, nil
	}

	if !r.concurrencyAllowed(ctx, task, schedule.Next(now)) {
		return ctrl.Result{RequeueAfter: schedule.Next(now).Sub(now)}, nil
	}

	return r.createJobAndUpdateStatus(ctx, task)
}

func (r *TaskReconciler) handleDelay(ctx context.Context, task *kubetaskv1.Task) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if task.Spec.Delay == nil {
		task.Status.Phase = kubetaskv1.TaskFailed
		task.Status.Message = "Delay task requires spec.delay"
		r.Status().Update(ctx, task)
		return ctrl.Result{}, nil
	}

	targetTime := task.CreationTimestamp.Add(task.Spec.Delay.Duration)
	now := time.Now()

	if now.Before(targetTime) {
		waitDuration := targetTime.Sub(now)
		log.Info("Delay task not yet ready", "task", task.Name, "wait", waitDuration.Round(time.Second))
		return ctrl.Result{RequeueAfter: waitDuration}, nil
	}

	if r.jobExists(ctx, task.Name) {
		log.Info("Job already exists for Delay task", "task", task.Name)
		return r.syncJobStatus(ctx, task)
	}

	return r.createJobAndUpdateStatus(ctx, task)
}

// =============================================================================
// Shared helpers — Job creation
// =============================================================================

func (r *TaskReconciler) createJobAndUpdateStatus(ctx context.Context, task *kubetaskv1.Task) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	job, err := r.constructJob(task)
	if err != nil {
		log.Error(err, "Failed to construct Job")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, job); err != nil {
		log.Error(err, "Failed to create Job")
		return ctrl.Result{}, err
	}

	log.Info("Created Job", "task", task.Name, "job", job.Name)

	task.Status.Phase = kubetaskv1.TaskRunning
	now := metav1.Now()
	task.Status.LastStartTime = &now
	task.Status.ActiveJobs = append(task.Status.ActiveJobs, job.Name)

	if task.Spec.Type == kubetaskv1.TaskTypeCron {
		task.Status.LastScheduleTime = &now
	}

	if err := r.Status().Update(ctx, task); err != nil {
		log.Error(err, "Failed to update Task status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *TaskReconciler) jobExists(ctx context.Context, taskName string) bool {
	jobs := &batchv1.JobList{}
	if err := r.List(ctx, jobs, client.MatchingLabels{"kubetask.io/task": taskName}); err != nil {
		return false
	}
	return len(jobs.Items) > 0
}

func (r *TaskReconciler) constructJob(task *kubetaskv1.Task) (*batchv1.Job, error) {
	jobName := fmt.Sprintf("%s-%d", task.Name, time.Now().Unix())

	backoffLimit := int32(3)
	if task.Spec.BackoffLimit != nil {
		backoffLimit = *task.Spec.BackoffLimit
	}

	container := corev1.Container{
		Name:      "task",
		Image:     task.Spec.Image,
		Command:   task.Spec.Command,
		Args:      task.Spec.Args,
		Env:       task.Spec.Env,
		Resources: task.Spec.Resources,
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: "default",
			Labels:    map[string]string{"kubetask.io/task": task.Name},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{container},
				},
			},
		},
	}

	if task.Spec.ActiveDeadlineSeconds != nil {
		job.Spec.ActiveDeadlineSeconds = task.Spec.ActiveDeadlineSeconds
	}
	if task.Spec.TTLSecondsAfterFinished != nil {
		job.Spec.TTLSecondsAfterFinished = task.Spec.TTLSecondsAfterFinished
	}

	if err := controllerutil.SetControllerReference(task, job, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	return job, nil
}

// =============================================================================
// Concurrency control
// =============================================================================

func (r *TaskReconciler) concurrencyAllowed(ctx context.Context, task *kubetaskv1.Task, _ time.Time) bool {
	log := log.FromContext(ctx)

	jobs := &batchv1.JobList{}
	if err := r.List(ctx, jobs, client.MatchingLabels{"kubetask.io/task": task.Name}); err != nil {
		return true
	}

	active := 0
	for _, job := range jobs.Items {
		if job.Status.Active > 0 {
			active++
		}
	}
	if active == 0 {
		return true
	}

	switch task.Spec.ConcurrencyPolicy {
	case kubetaskv1.ConcurrencyAllow:
		return true
	case kubetaskv1.ConcurrencyForbid:
		log.Info("Skipping execution, Forbid policy", "task", task.Name, "activeJobs", active)
		return false
	case kubetaskv1.ConcurrencyReplace:
		log.Info("Replacing active Jobs", "task", task.Name)
		for i := range jobs.Items {
			job := &jobs.Items[i]
			if job.Status.Active > 0 {
				if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
					log.Error(err, "Failed to delete old Job", "job", job.Name)
				}
			}
		}
		return true
	default:
		return true
	}
}

// =============================================================================
// Status synchronization
// =============================================================================

func (r *TaskReconciler) syncJobStatus(ctx context.Context, task *kubetaskv1.Task) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	jobs := &batchv1.JobList{}
	if err := r.List(ctx, jobs, client.MatchingLabels{"kubetask.io/task": task.Name}); err != nil {
		return ctrl.Result{}, err
	}

	var activeJobs []string
	newPhase := task.Status.Phase
	phaseChanged := false

	for i := range jobs.Items {
		job := &jobs.Items[i]
		if job.Status.Active > 0 {
			activeJobs = append(activeJobs, job.Name)
		}

		if r.jobTransitioned(job) {
			task.Status.Failed += job.Status.Failed
			task.Status.Succeeded += job.Status.Succeeded

			task.Status.ExecutionHistory = append(task.Status.ExecutionHistory, kubetaskv1.ExecutionRecord{
				JobName:   job.Name,
				StartTime: job.CreationTimestamp,
				StopTime:  metav1.Now(),
				Phase:     r.jobPhase(job),
			})

			if len(task.Status.ExecutionHistory) > 10 {
				task.Status.ExecutionHistory = task.Status.ExecutionHistory[len(task.Status.ExecutionHistory)-10:]
			}

			if job.Status.CompletionTime != nil {
				task.Status.LastCompletionTime = &metav1.Time{Time: job.Status.CompletionTime.Time}
			}

			phaseChanged = true
		}
	}

	task.Status.ActiveJobs = activeJobs

	if phaseChanged || activeJobsChanged(task, activeJobs) {
		switch {
		case len(activeJobs) > 0:
			newPhase = kubetaskv1.TaskRunning
		case task.Status.Failed == 0 || task.Status.Succeeded > 0:
			newPhase = kubetaskv1.TaskSucceeded
		default:
			newPhase = kubetaskv1.TaskFailed
		}

		task.Status.Phase = newPhase
		if err := r.Status().Update(ctx, task); err != nil {
			log.Error(err, "Failed to update Task status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *TaskReconciler) jobTransitioned(job *batchv1.Job) bool {
	return job.Status.Succeeded > 0 || job.Status.Failed > 0 ||
		job.Status.Active == 1 && job.Status.StartTime != nil
}

func (r *TaskReconciler) jobPhase(job *batchv1.Job) kubetaskv1.TaskPhase {
	if job.Status.Succeeded > 0 {
		return kubetaskv1.TaskSucceeded
	}
	if job.Status.Failed > 0 {
		return kubetaskv1.TaskFailed
	}
	return kubetaskv1.TaskRunning
}

func activeJobsChanged(task *kubetaskv1.Task, current []string) bool {
	if len(task.Status.ActiveJobs) != len(current) {
		return true
	}
	for i := range task.Status.ActiveJobs {
		if task.Status.ActiveJobs[i] != current[i] {
			return true
		}
	}
	return false
}
