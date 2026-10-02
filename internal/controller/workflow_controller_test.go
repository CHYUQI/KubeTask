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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newWorkflowReconciler() *WorkflowReconciler {
	return &WorkflowReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
}

// reconcileWorkflow 连续调用 Reconcile 若干次，模拟 controller-runtime 的重复收敛。
func reconcileWorkflow(name string, times int) {
	r := newWorkflowReconciler()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: name}}
	for range times {
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
	}
}

// createWorkflow 建 Workflow 并完成第一次 reconcile（只登记 finalizer）。
func createWorkflow(wf *kubetaskv1.Workflow) {
	Expect(k8sClient.Create(ctx, wf)).To(Succeed())
	reconcileWorkflow(wf.Name, 1)
}

func getWorkflow(name string) *kubetaskv1.Workflow {
	wf := &kubetaskv1.Workflow{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, wf)).To(Succeed())
	return wf
}

// workflowNode 取某个节点的状态，节点不存在时直接判定用例失败。
func workflowNode(wf *kubetaskv1.Workflow, name string) kubetaskv1.WorkflowNodeStatus {
	for _, node := range wf.Status.Nodes {
		if node.Name == name {
			return node
		}
	}
	Fail(fmt.Sprintf("workflow %s has no node %q", wf.Name, name))
	return kubetaskv1.WorkflowNodeStatus{}
}

func getChildTask(name string) *kubetaskv1.Task {
	task := &kubetaskv1.Task{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, task)
	if apierrors.IsNotFound(err) {
		return nil
	}
	Expect(err).NotTo(HaveOccurred())
	return task
}

// listWorkflowChildren 按 label 列出某个 Workflow 创建的全部子 Task。
func listWorkflowChildren(name string) []kubetaskv1.Task {
	tasks := &kubetaskv1.TaskList{}
	Expect(k8sClient.List(ctx, tasks, client.MatchingLabels{workflowLabel: name})).To(Succeed())
	return tasks.Items
}

// setChildTaskPhase 直接把子 Task 推进到终态，绕过真实 Job/Pod。
func setChildTaskPhase(name string, phase kubetaskv1.TaskPhase) {
	task := getChildTask(name)
	Expect(task).NotTo(BeNil(), "child task %s should exist", name)

	task.Status.Phase = phase
	if phase == kubetaskv1.TaskSucceeded || phase == kubetaskv1.TaskFailed {
		now := metav1.Now()
		task.Status.LastCompletionTime = &now
	}
	Expect(k8sClient.Status().Update(ctx, task)).To(Succeed())
}

// cleanupWorkflow 强制清掉 Workflow 及其子 Task，避免用例间状态污染。
func cleanupWorkflow(name string) {
	tasks := &kubetaskv1.TaskList{}
	if err := k8sClient.List(ctx, tasks, client.MatchingLabels{workflowLabel: name}); err == nil {
		for i := range tasks.Items {
			_ = k8sClient.Delete(ctx, &tasks.Items[i], client.PropagationPolicy(metav1.DeletePropagationBackground))
		}
	}

	wf := &kubetaskv1.Workflow{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, wf); err == nil {
		if len(wf.Finalizers) > 0 {
			wf.Finalizers = nil
			_ = k8sClient.Update(ctx, wf)
		}
		_ = k8sClient.Delete(ctx, wf, client.PropagationPolicy(metav1.DeletePropagationBackground))
	}

	Eventually(func() bool {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &kubetaskv1.Workflow{})
		return apierrors.IsNotFound(err)
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

	Eventually(func() int {
		remaining := &kubetaskv1.TaskList{}
		if err := k8sClient.List(ctx, remaining, client.MatchingLabels{workflowLabel: name}); err != nil {
			return -1
		}
		return len(remaining.Items)
	}, 10*time.Second, 100*time.Millisecond).Should(Equal(0))
}

// newWorkflow 构造 "build -> test" 的串行流程，两个节点都引用命名模板。
func newWorkflow(name string) *kubetaskv1.Workflow {
	return &kubetaskv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kubetaskv1.WorkflowSpec{
			Tasks: []kubetaskv1.WorkflowTask{
				{Name: "build", Template: "go-build"},
				{Name: "test", DependsOn: []string{"build"}, Template: "go-test"},
			},
			TaskTemplates: map[string]kubetaskv1.TaskSpec{
				"go-build": {
					Type:    kubetaskv1.TaskTypeOneTime,
					Image:   "golang:1.25",
					Command: []string{"go", "build", "./..."},
				},
				"go-test": {
					Type:    kubetaskv1.TaskTypeOneTime,
					Image:   "golang:1.25",
					Command: []string{"go", "test", "./..."},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Suite
// ---------------------------------------------------------------------------

var _ = Describe("Workflow Controller", func() {

	// =========================================================================
	// 正向调度
	// =========================================================================
	Context("when reconciling a valid DAG", func() {
		wfName := "wf-basic"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("adds the finalizer on the first reconcile only", func() {
			wf := newWorkflow(wfName)
			Expect(k8sClient.Create(ctx, wf)).To(Succeed())

			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(controllerutil.ContainsFinalizer(stored, workflowFinalizer)).To(BeTrue())
			Expect(stored.Status.Phase).To(BeEmpty())
			Expect(listWorkflowChildren(wfName)).To(BeEmpty())
		})

		It("initializes status and only schedules the root node", func() {
			createWorkflow(newWorkflow(wfName))
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowRunning))
			Expect(stored.Status.ObservedGeneration).To(Equal(stored.Generation))
			Expect(stored.Status.StartTime).NotTo(BeNil())
			Expect(stored.Status.Nodes).To(HaveLen(2))

			build := workflowNode(stored, "build")
			Expect(build.Phase).To(Equal(kubetaskv1.NodeRunning))
			Expect(build.TaskName).To(Equal(wfName + "-build"))
			Expect(build.StartTime).NotTo(BeNil())

			test := workflowNode(stored, "test")
			Expect(test.Phase).To(Equal(kubetaskv1.NodePending))
			Expect(getChildTask(wfName + "-test")).To(BeNil())
		})

		It("creates the root child Task from a template snapshot", func() {
			createWorkflow(newWorkflow(wfName))
			reconcileWorkflow(wfName, 1)

			child := getChildTask(wfName + "-build")
			Expect(child).NotTo(BeNil())
			Expect(child.Labels).To(HaveKeyWithValue(workflowLabel, wfName))
			Expect(child.Labels).To(HaveKeyWithValue(workflowNodeLabel, "build"))

			Expect(child.Spec.Type).To(Equal(kubetaskv1.TaskTypeOneTime))
			Expect(child.Spec.Image).To(Equal("golang:1.25"))
			Expect(child.Spec.Command).To(Equal([]string{"go", "build", "./..."}))
			Expect(child.Spec.Schedule).To(BeEmpty())
			Expect(child.Spec.Delay).To(BeNil())
			Expect(child.Spec.Suspend).To(BeNil())

			Expect(child.OwnerReferences).To(HaveLen(1))
			Expect(child.OwnerReferences[0].Name).To(Equal(wfName))
			Expect(child.OwnerReferences[0].Controller).NotTo(BeNil())
			Expect(*child.OwnerReferences[0].Controller).To(BeTrue())
		})

		It("creates the child Task from an inline taskSpec", func() {
			wf := newWorkflow(wfName)
			wf.Spec.Tasks = []kubetaskv1.WorkflowTask{
				{
					Name: "inline",
					TaskSpec: &kubetaskv1.TaskSpec{
						Type:    kubetaskv1.TaskTypeOneTime,
						Image:   "busybox",
						Command: []string{"echo", "hello"},
					},
				},
			}
			wf.Spec.TaskTemplates = nil

			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)

			child := getChildTask(wfName + "-inline")
			Expect(child).NotTo(BeNil())
			Expect(child.Spec.Type).To(Equal(kubetaskv1.TaskTypeOneTime))
			Expect(child.Spec.Image).To(Equal("busybox"))
			Expect(child.Spec.Command).To(Equal([]string{"echo", "hello"}))
		})

		It("schedules downstream nodes after dependencies succeed", func() {
			createWorkflow(newWorkflow(wfName))
			reconcileWorkflow(wfName, 1)

			setChildTaskPhase(wfName+"-build", kubetaskv1.TaskSucceeded)
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(workflowNode(stored, "build").Phase).To(Equal(kubetaskv1.NodeSucceeded))
			Expect(workflowNode(stored, "test").Phase).To(Equal(kubetaskv1.NodeRunning))
			Expect(getChildTask(wfName + "-test")).NotTo(BeNil())

			setChildTaskPhase(wfName+"-test", kubetaskv1.TaskSucceeded)
			reconcileWorkflow(wfName, 1)

			stored = getWorkflow(wfName)
			Expect(workflowNode(stored, "test").Phase).To(Equal(kubetaskv1.NodeSucceeded))
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowSucceeded))
			Expect(stored.Status.CompletionTime).NotTo(BeNil())
		})
	})

	// =========================================================================
	// 失败传播与 runOnFailure
	// =========================================================================
	Context("when a node fails", func() {
		wfName := "wf-failure"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("fails the workflow and skips downstream nodes", func() {
			createWorkflow(newWorkflow(wfName))
			reconcileWorkflow(wfName, 1)

			setChildTaskPhase(wfName+"-build", kubetaskv1.TaskFailed)
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(workflowNode(stored, "build").Phase).To(Equal(kubetaskv1.NodeFailed))
			Expect(workflowNode(stored, "build").CompletionTime).NotTo(BeNil())

			skipped := workflowNode(stored, "test")
			Expect(skipped.Phase).To(Equal(kubetaskv1.NodeSkipped))
			Expect(skipped.Message).To(ContainSubstring("build"))
			Expect(getChildTask(wfName + "-test")).To(BeNil())

			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowFailed))
			Expect(stored.Status.Message).To(ContainSubstring("build"))
			Expect(stored.Status.CompletionTime).NotTo(BeNil())
		})

		It("runs a runOnFailure node when a dependency failed", func() {
			wf := newWorkflow(wfName)
			wf.Spec.Tasks = []kubetaskv1.WorkflowTask{
				{Name: "build", Template: "go-build"},
				{Name: "notify", DependsOn: []string{"build"}, RunOnFailure: true, Template: "go-test"},
			}

			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)

			setChildTaskPhase(wfName+"-build", kubetaskv1.TaskFailed)
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(workflowNode(stored, "notify").Phase).To(Equal(kubetaskv1.NodeRunning))
			Expect(getChildTask(wfName + "-notify")).NotTo(BeNil())
		})

		It("skips a runOnFailure node when all dependencies succeeded", func() {
			wf := newWorkflow(wfName)
			wf.Spec.Tasks = []kubetaskv1.WorkflowTask{
				{Name: "build", Template: "go-build"},
				{Name: "notify", DependsOn: []string{"build"}, RunOnFailure: true, Template: "go-test"},
			}

			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)

			setChildTaskPhase(wfName+"-build", kubetaskv1.TaskSucceeded)
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			notify := workflowNode(stored, "notify")
			Expect(notify.Phase).To(Equal(kubetaskv1.NodeSkipped))
			Expect(notify.Message).To(Equal("all dependencies succeeded"))
			Expect(getChildTask(wfName + "-notify")).To(BeNil())
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowSucceeded))
		})
	})

	// =========================================================================
	// maxParallel 限流
	// =========================================================================
	Context("when maxParallel limits concurrency", func() {
		wfName := "wf-parallel"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("only schedules up to maxParallel nodes", func() {
			wf := newWorkflow(wfName)
			wf.Spec.MaxParallel = 1
			wf.Spec.Tasks = []kubetaskv1.WorkflowTask{
				{Name: "first", Template: "go-build"},
				{Name: "second", Template: "go-build"},
			}

			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(workflowNode(stored, "first").Phase).To(Equal(kubetaskv1.NodeRunning))
			Expect(workflowNode(stored, "second").Phase).To(Equal(kubetaskv1.NodePending))
			Expect(listWorkflowChildren(wfName)).To(HaveLen(1))
			Expect(getChildTask(wfName + "-first")).NotTo(BeNil())
			Expect(getChildTask(wfName + "-second")).To(BeNil())
		})
	})

	// =========================================================================
	// 非法 spec
	// =========================================================================
	Context("when the spec is invalid", func() {
		wfName := "wf-invalid"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("fails the workflow when a template is missing", func() {
			wf := newWorkflow(wfName)
			wf.Spec.Tasks = []kubetaskv1.WorkflowTask{{Name: "build", Template: "does-not-exist"}}

			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowFailed))
			Expect(stored.Status.Message).To(ContainSubstring("MissingTemplate"))
			Expect(stored.Status.CompletionTime).NotTo(BeNil())
			Expect(listWorkflowChildren(wfName)).To(BeEmpty())
		})

		It("fails the workflow on a dependency cycle", func() {
			wf := newWorkflow(wfName)
			wf.Spec.Tasks = []kubetaskv1.WorkflowTask{
				{Name: "first", DependsOn: []string{"second"}, Template: "go-build"},
				{Name: "second", DependsOn: []string{"first"}, Template: "go-build"},
			}

			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowFailed))
			Expect(stored.Status.Message).To(ContainSubstring("CyclicDependency"))
			Expect(listWorkflowChildren(wfName)).To(BeEmpty())
		})
	})

	// =========================================================================
	// 子 Task 名字长度
	// =========================================================================
	Context("when child task names would be too long", func() {
		// 46 字符上限：workflow 名 + "-" + 节点名 必须 <= 46
		wfName := "wf-child-task-name-limit-that-is-way-too-long"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("fails the workflow instead of creating an invalid Job name", func() {
			wf := newWorkflow(wfName)
			wf.Spec.Tasks = []kubetaskv1.WorkflowTask{{Name: "build", Template: "go-build"}}

			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowFailed))
			Expect(stored.Status.Message).To(ContainSubstring("child task names"))
			Expect(listWorkflowChildren(wfName)).To(BeEmpty())
		})
	})

	// =========================================================================
	// 删除清理
	// =========================================================================
	Context("when the workflow is deleted", func() {
		wfName := "wf-deletion"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("deletes the child Tasks and removes the finalizer", func() {
			createWorkflow(newWorkflow(wfName))
			reconcileWorkflow(wfName, 1)
			Expect(getChildTask(wfName + "-build")).NotTo(BeNil())

			Expect(k8sClient.Delete(ctx, getWorkflow(wfName))).To(Succeed())
			reconcileWorkflow(wfName, 3)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: wfName}, &kubetaskv1.Workflow{})
				return apierrors.IsNotFound(err)
			}, 10*time.Second, 200*time.Millisecond).Should(BeTrue())
			Expect(listWorkflowChildren(wfName)).To(BeEmpty())
		})
	})
})
