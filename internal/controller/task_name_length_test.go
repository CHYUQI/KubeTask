package controller

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/naming"
)

// Task 名会被拼成 Job 名（再被 Job 控制器拼成 Pod 名），超长时 Controller 直接判失败，
// 不去创建注定失败的 Job。边界值来自 naming.MaxTaskNameLength。
var _ = Describe("Task name length guard", func() {
	atLimit := strings.Repeat("a", naming.MaxTaskNameLength)
	overLimit := strings.Repeat("b", naming.MaxTaskNameLength+1)

	AfterEach(func() {
		cleanupTask(atLimit)
		cleanupTask(overLimit)
	})

	It("still runs a task whose name is exactly at the limit", func() {
		createAndInitTask(newOneTimeTask(atLimit))

		stored := getTask(atLimit)
		Expect(stored.Status.Phase).To(Equal(kubetaskv1.TaskRunning))
		Expect(listJobs(atLimit).Items).To(HaveLen(1))
	})

	It("fails an over-long task without creating a Job", func() {
		createAndInitTask(newOneTimeTask(overLimit))

		stored := getTask(overLimit)
		Expect(stored.Status.Phase).To(Equal(kubetaskv1.TaskFailed))
		Expect(stored.Status.Message).To(ContainSubstring("too long"))
		Expect(stored.Status.Message).To(ContainSubstring("46"))
		Expect(listJobs(overLimit).Items).To(BeEmpty())
	})

	It("keeps the failure result stable across repeated reconciles", func() {
		createAndInitTask(newOneTimeTask(overLimit))
		reconcileUntilStable(newTestReconciler(), overLimit)

		stored := getTask(overLimit)
		Expect(stored.Status.Phase).To(Equal(kubetaskv1.TaskFailed))
		Expect(stored.Status.Message).To(ContainSubstring("too long"))
		Expect(stored.Status.Succeeded).To(BeZero())
		Expect(stored.Status.Failed).To(BeZero())
		Expect(listJobs(overLimit).Items).To(BeEmpty())
	})
})
