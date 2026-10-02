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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

// CRD 层的声明式校验（结构校验 + CEL）：这些规则在 API Server 入口就该拦住，
// 不需要 Controller 参与；Controller 的 internal/workflow 只负责运行时规则（引用 / 环等）。
var _ = Describe("Workflow CRD validation", func() {

	oneTime := func() *kubetaskv1.TaskSpec {
		return &kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: "busybox"}
	}

	invalidSpecs := []struct {
		slug    string
		desc    string
		mutate  func(spec *kubetaskv1.WorkflowSpec)
		message string
	}{
		{
			slug: "taskspec-cron",
			desc: "rejects an inline taskSpec whose type is not OneTime",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				spec.Tasks = []kubetaskv1.WorkflowTask{{
					Name:     "build",
					TaskSpec: &kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeCron, Image: "busybox"},
				}}
			},
			message: "OneTime",
		},
		{
			slug: "taskspec-schedule",
			desc: "rejects an inline taskSpec with schedule",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				taskSpec := oneTime()
				taskSpec.Schedule = "*/5 * * * *"
				spec.Tasks = []kubetaskv1.WorkflowTask{{Name: "build", TaskSpec: taskSpec}}
			},
			message: "schedule",
		},
		{
			slug: "taskspec-delay",
			desc: "rejects an inline taskSpec with delay",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				taskSpec := oneTime()
				taskSpec.Delay = &metav1.Duration{Duration: 30_000_000_000} // 30s
				spec.Tasks = []kubetaskv1.WorkflowTask{{Name: "build", TaskSpec: taskSpec}}
			},
			message: "delay",
		},
		{
			slug: "taskspec-suspend",
			desc: "rejects an inline taskSpec with suspend=true",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				taskSpec := oneTime()
				suspended := true
				taskSpec.Suspend = &suspended
				spec.Tasks = []kubetaskv1.WorkflowTask{{Name: "build", TaskSpec: taskSpec}}
			},
			message: "suspend",
		},
		{
			slug: "both-sources",
			desc: "rejects a node that sets both template and taskSpec",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				spec.Tasks = []kubetaskv1.WorkflowTask{{
					Name:     "build",
					Template: "go-build",
					TaskSpec: oneTime(),
				}}
			},
			message: "必须且只能设置一个",
		},
		{
			slug: "no-source",
			desc: "rejects a node that sets neither template nor taskSpec",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				spec.Tasks = []kubetaskv1.WorkflowTask{{Name: "build"}}
			},
			message: "必须且只能设置一个",
		},
		{
			slug: "empty-tasks",
			desc: "rejects an empty task list",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				spec.Tasks = nil
			},
			message: "tasks",
		},
		{
			slug: "bad-name",
			desc: "rejects a node name that breaks the DNS-1123 pattern",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				spec.Tasks = []kubetaskv1.WorkflowTask{{Name: "Build_Step", Template: "go-build"}}
			},
			message: "name",
		},
		{
			slug: "duplicate-name",
			desc: "rejects duplicated node names",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				spec.Tasks = []kubetaskv1.WorkflowTask{
					{Name: "build", Template: "go-build"},
					{Name: "build", Template: "go-build"},
				}
			},
			message: "build",
		},
		{
			slug: "negative-parallel",
			desc: "rejects a negative maxParallel",
			mutate: func(spec *kubetaskv1.WorkflowSpec) {
				spec.MaxParallel = -1
			},
			message: "maxParallel",
		},
	}

	for _, tc := range invalidSpecs {
		tc := tc
		It(tc.desc, func() {
			wf := newWorkflow("wf-invalid-" + tc.slug)
			tc.mutate(&wf.Spec)

			err := k8sClient.Create(ctx, wf)
			if err == nil {
				cleanupWorkflow(wf.Name)
				Fail(fmt.Sprintf("API Server 应该拒绝这个 spec，但它被创建成功了: %s", tc.desc))
			}

			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "unexpected error type: %v", err)
			Expect(err.Error()).To(ContainSubstring(tc.message))
		})
	}

	// 依赖引用（自依赖 / 未知依赖 / 环）是运行时规则，CEL 管不到：
	// 这里确认 API Server 会放行，从而说明 internal/workflow 的校验是不可省的。
	It("accepts a self-dependency at admission and leaves it to the controller", func() {
		wf := newWorkflow("wf-self-dependency")
		wf.Spec.Tasks = []kubetaskv1.WorkflowTask{{
			Name:      "build",
			DependsOn: []string{"build"},
			Template:  "go-build",
		}}

		Expect(k8sClient.Create(ctx, wf)).To(Succeed())
		cleanupWorkflow(wf.Name)
	})
})
