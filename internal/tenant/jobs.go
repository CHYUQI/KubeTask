package tenant

import (
	"context"
	"slices"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

// TaskLabel identifies Jobs associated with a Task.
const TaskLabel = "kubetask.io/task"

// JobNamespace keeps unassigned legacy Tasks in default and places tenant Jobs in their Namespace.
func JobNamespace(task *kubetaskv1.Task) string {
	if namespace := task.Labels[Label]; namespace != "" {
		return namespace
	}
	return metav1.NamespaceDefault
}

// ListTaskJobs restricts Job lookups to the Task's execution Namespace.
func ListTaskJobs(ctx context.Context, reader client.Reader, task *kubetaskv1.Task) ([]batchv1.Job, error) {
	jobs := &batchv1.JobList{}
	if err := reader.List(ctx, jobs, client.InNamespace(JobNamespace(task)), client.MatchingLabels{TaskLabel: task.Name}); err != nil {
		return nil, err
	}
	if task.Labels[Label] != "" {
		jobs.Items = slices.DeleteFunc(jobs.Items, func(job batchv1.Job) bool {
			return !metav1.IsControlledBy(&job, task)
		})
	}
	return jobs.Items, nil
}
