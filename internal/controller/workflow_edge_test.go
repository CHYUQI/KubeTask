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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

// createExistingChildTask 手工造一个"看起来已经存在"的子 Task，
// 用来模拟 Controller 在创建子 Task 与写 status 之间重启的场景。
func createExistingChildTask(wf *kubetaskv1.Workflow, node string, phase kubetaskv1.TaskPhase) {
	task := &kubetaskv1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name: childTaskName(wf.Name, node),
			Labels: map[string]string{
				workflowLabel:     wf.Name,
				workflowNodeLabel: node,
			},
		},
		Spec: kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: "busybox"},
	}
	Expect(controllerutil.SetControllerReference(wf, task, k8sClient.Scheme())).To(Succeed())
	Expect(k8sClient.Create(ctx, task)).To(Succeed())

	now := metav1.Now()
	task.Status.Phase = phase
	task.Status.LastStartTime = &now
	task.Status.LastCompletionTime = &now
	Expect(k8sClient.Status().Update(ctx, task)).To(Succeed())
}

func deleteChildTask(name string) {
	task := getChildTask(name)
	Expect(task).NotTo(BeNil(), "child task %s should exist", name)
	Expect(k8sClient.Delete(ctx, task)).To(Succeed())

	Eventually(func() bool {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &kubetaskv1.Task{})
		return apierrors.IsNotFound(err)
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
}

var _ = Describe("Workflow Controller edge cases", func() {

	// =========================================================================
	// Controller 重启后的收敛（活性回归）
	// =========================================================================
	Context("when a child Task already exists before the node was scheduled", func() {
		wfName := "wf-adopt"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("adopts the finished child Task without stalling the workflow", func() {
			createWorkflow(newWorkflow(wfName))

			// 模拟"子 Task 早已创建并跑完，但 Workflow status 还没写成功"。
			createExistingChildTask(getWorkflow(wfName), "build", kubetaskv1.TaskSucceeded)

			reconcileWorkflow(wfName, 1)

			// 关键点：终态的子 Task 不会再产生事件，必须在这一轮就落终态并推进下游。
			stored := getWorkflow(wfName)
			build := workflowNode(stored, "build")
			Expect(build.Phase).To(Equal(kubetaskv1.NodeSucceeded))
			Expect(build.TaskName).To(Equal(wfName + "-build"))
			Expect(build.CompletionTime).NotTo(BeNil())
			Expect(workflowNode(stored, "test").Phase).To(Equal(kubetaskv1.NodeRunning))
			Expect(getChildTask(wfName + "-test")).NotTo(BeNil())
		})
	})

	// =========================================================================
	// maxParallel 名额释放
	// =========================================================================
	Context("when a running node finishes", func() {
		wfName := "wf-parallel-release"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("frees a maxParallel slot for the next pending node", func() {
			wf := newWorkflow(wfName)
			wf.Spec.MaxParallel = 1
			wf.Spec.Tasks = []kubetaskv1.WorkflowTask{
				{Name: "first", Template: "go-build"},
				{Name: "second", Template: "go-build"},
			}

			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)
			Expect(workflowNode(getWorkflow(wfName), "second").Phase).To(Equal(kubetaskv1.NodePending))

			setChildTaskPhase(wfName+"-first", kubetaskv1.TaskSucceeded)
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(workflowNode(stored, "first").Phase).To(Equal(kubetaskv1.NodeSucceeded))
			Expect(workflowNode(stored, "second").Phase).To(Equal(kubetaskv1.NodeRunning))
			Expect(getChildTask(wfName + "-second")).NotTo(BeNil())
		})
	})

	// =========================================================================
	// 子 Task 被外部删除
	// =========================================================================
	Context("when a running child Task is deleted", func() {
		wfName := "wf-recreate"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("recreates the child Task with the same deterministic name", func() {
			createWorkflow(newWorkflow(wfName))
			reconcileWorkflow(wfName, 1)
			Expect(getChildTask(wfName + "-build")).NotTo(BeNil())

			uidBefore := getChildTask(wfName + "-build").UID
			deleteChildTask(wfName + "-build")

			reconcileWorkflow(wfName, 1)

			recreated := getChildTask(wfName + "-build")
			Expect(recreated).NotTo(BeNil())
			Expect(recreated.UID).NotTo(Equal(uidBefore), "应该是新建的对象，而不是原来的 Task")
			Expect(workflowNode(getWorkflow(wfName), "build").Phase).To(Equal(kubetaskv1.NodeRunning))
		})
	})

	// =========================================================================
	// 校验失败后修正 spec
	// =========================================================================
	Context("when an invalid spec is fixed", func() {
		wfName := "wf-retry"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("reschedules the workflow from the corrected spec", func() {
			wf := newWorkflow(wfName)
			wf.Spec.Tasks = []kubetaskv1.WorkflowTask{{Name: "build", Template: "missing"}}
			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)

			failed := getWorkflow(wfName)
			Expect(failed.Status.Phase).To(Equal(kubetaskv1.WorkflowFailed))
			Expect(failed.Status.CompletionTime).NotTo(BeNil())

			// 修正 spec：换成存在的模板
			failed.Spec.Tasks = []kubetaskv1.WorkflowTask{{Name: "build", Template: "go-build"}}
			Expect(k8sClient.Update(ctx, failed)).To(Succeed())

			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowRunning))
			Expect(stored.Status.Message).To(BeEmpty())
			Expect(stored.Status.CompletionTime).To(BeNil())
			Expect(stored.Status.ObservedGeneration).To(Equal(stored.Generation))
			Expect(workflowNode(stored, "build").Phase).To(Equal(kubetaskv1.NodeRunning))
			Expect(getChildTask(wfName + "-build")).NotTo(BeNil())
		})
	})

	// =========================================================================
	// 终态之后修改 spec（一次性语义）
	// =========================================================================
	Context("when the spec changes after the workflow finished", func() {
		wfName := "wf-oneshot"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("keeps the terminal result instead of running again", func() {
			createWorkflow(newWorkflow(wfName))
			reconcileWorkflow(wfName, 1)

			setChildTaskPhase(wfName+"-build", kubetaskv1.TaskSucceeded)
			reconcileWorkflow(wfName, 1)
			setChildTaskPhase(wfName+"-test", kubetaskv1.TaskSucceeded)
			reconcileWorkflow(wfName, 1)
			Expect(getWorkflow(wfName).Status.Phase).To(Equal(kubetaskv1.WorkflowSucceeded))

			finished := getWorkflow(wfName)
			finished.Spec.Tasks = append(finished.Spec.Tasks,
				kubetaskv1.WorkflowTask{Name: "third", DependsOn: []string{"test"}, Template: "go-build"})
			Expect(k8sClient.Update(ctx, finished)).To(Succeed())

			reconcileWorkflow(wfName, 2)

			stored := getWorkflow(wfName)
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowSucceeded))
			Expect(stored.Status.ObservedGeneration).To(Equal(stored.Generation))
			Expect(stored.Status.Nodes).To(HaveLen(2), "不应该为新节点补状态")
			Expect(getChildTask(wfName+"-third")).To(BeNil(), "不应该为终态后的 spec 变更创建子 Task")
		})
	})

	// =========================================================================
	// 模板里的调度字段被兜底清掉
	// =========================================================================
	Context("when a task template carries scheduling fields", func() {
		wfName := "wf-template-sanitize"

		AfterEach(func() {
			cleanupWorkflow(wfName)
		})

		It("forces the child Task to run once", func() {
			suspend := true
			delay := metav1.Duration{Duration: time.Minute}
			wf := &kubetaskv1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: wfName},
				Spec: kubetaskv1.WorkflowSpec{
					Tasks: []kubetaskv1.WorkflowTask{{Name: "build", Template: "cron-template"}},
					TaskTemplates: map[string]kubetaskv1.TaskSpec{
						"cron-template": {
							Type:     kubetaskv1.TaskTypeCron,
							Schedule: "*/5 * * * *",
							Delay:    &delay,
							Suspend:  &suspend,
							Image:    "busybox",
						},
					},
				},
			}

			createWorkflow(wf)
			reconcileWorkflow(wfName, 1)

			child := getChildTask(wfName + "-build")
			Expect(child).NotTo(BeNil())
			Expect(child.Spec.Type).To(Equal(kubetaskv1.TaskTypeOneTime))
			Expect(child.Spec.Schedule).To(BeEmpty())
			Expect(child.Spec.Delay).To(BeNil())
			Expect(child.Spec.Suspend).To(BeNil())
		})
	})

	// =========================================================================
	// 子 Task 名字被本 Workflow 之外的 Task 占用
	// =========================================================================
	Context("when the deterministic child Task name is already taken", func() {
		wfName := "wf-name-conflict"

		AfterEach(func() {
			// 外来 Task 没有 workflow label，cleanupWorkflow 找不到它，需要单独清理。
			if foreign := getChildTask(wfName + "-build"); foreign != nil {
				_ = k8sClient.Delete(ctx, foreign)
			}
			cleanupWorkflow(wfName)
		})

		It("fails the workflow instead of hijacking the foreign Task", func() {
			foreign := &kubetaskv1.Task{
				ObjectMeta: metav1.ObjectMeta{Name: wfName + "-build"},
				Spec:       kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: "busybox"},
			}
			Expect(k8sClient.Create(ctx, foreign)).To(Succeed())

			createWorkflow(newWorkflow(wfName))
			reconcileWorkflow(wfName, 1)

			stored := getWorkflow(wfName)
			Expect(stored.Status.Phase).To(Equal(kubetaskv1.WorkflowFailed))
			Expect(stored.Status.Message).To(ContainSubstring("child task name conflict"))
			Expect(stored.Status.CompletionTime).NotTo(BeNil())

			// 外来的 Task 必须保持原样，不能被 Workflow 接管。
			kept := getChildTask(wfName + "-build")
			Expect(kept).NotTo(BeNil())
			Expect(kept.UID).To(Equal(foreign.UID))
			Expect(kept.OwnerReferences).To(BeEmpty())
		})
	})

	// =========================================================================
	// 删除时子 Task 还在清理
	// =========================================================================
	Context("when a child Task is still terminating", func() {
		wfName := "wf-deletion-wait"

		AfterEach(func() {
			if child := getChildTask(wfName + "-build"); child != nil {
				child.Finalizers = nil
				_ = k8sClient.Update(ctx, child)
			}
			cleanupWorkflow(wfName)
		})

		It("keeps the finalizer until every child Task is gone", func() {
			createWorkflow(newWorkflow(wfName))
			reconcileWorkflow(wfName, 1)

			// 模拟 TaskReconciler 给子 Task 挂上 finalizer（正在清理 Job）。
			child := getChildTask(wfName + "-build")
			Expect(child).NotTo(BeNil())
			controllerutil.AddFinalizer(child, "kubetask.kubetask.io/finalizer")
			Expect(k8sClient.Update(ctx, child)).To(Succeed())

			Expect(k8sClient.Delete(ctx, getWorkflow(wfName))).To(Succeed())
			reconcileWorkflow(wfName, 2)

			// 子 Task 还在 Terminating，Workflow 不能被摘掉 finalizer（否则子 Task 会变成孤儿）。
			stored := getWorkflow(wfName)
			Expect(stored.DeletionTimestamp).NotTo(BeNil())
			Expect(controllerutil.ContainsFinalizer(stored, workflowFinalizer)).To(BeTrue())

			// 清理完成：子 Task 消失后 Workflow 才真正删除。
			child = getChildTask(wfName + "-build")
			child.Finalizers = nil
			Expect(k8sClient.Update(ctx, child)).To(Succeed())

			reconcileWorkflow(wfName, 2)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: wfName}, &kubetaskv1.Workflow{})
				return apierrors.IsNotFound(err)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})
})
