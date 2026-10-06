package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

// ==========================================================================
// Helper
// ==========================================================================

// workflowJSON 构造一个 "build -> test" 的合法 Workflow 请求体。
func workflowJSON(name string) string {
	return `{
		"apiVersion": "kubetask.kubetask.io/v1",
		"kind": "Workflow",
		"metadata": { "name": "` + name + `" },
		"spec": {
			"tasks": [
				{ "name": "build", "template": "go-build" },
				{ "name": "test", "dependsOn": ["build"], "template": "go-test" }
			],
			"taskTemplates": {
				"go-build": { "type": "OneTime", "image": "golang:1.25", "command": ["go", "build", "./..."] },
				"go-test": { "type": "OneTime", "image": "golang:1.25", "command": ["go", "test", "./..."] }
			}
		}
	}`
}

func deleteWorkflow(name string) {
	wf := &kubetaskv1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_ = k8sClient.Delete(context.Background(), wf)
}

// createWorkflowDirect 绕过 HTTP 校验直接落库，用来构造"入口放行不了"的现场。
func createWorkflowDirect(name string, spec kubetaskv1.WorkflowSpec) {
	wf := &kubetaskv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       spec,
	}
	Expect(k8sClient.Create(context.Background(), wf)).To(Succeed())
}

// ==========================================================================
// POST /api/v1/workflows
// ==========================================================================

var _ = Describe("POST /api/v1/workflows", func() {
	AfterEach(func() {
		deleteWorkflow("api-wf-create")
		deleteWorkflow("api-wf-invalid")
	})

	It("should create a workflow and return 201", func() {
		w := do("POST", "/api/v1/workflows", workflowJSON("api-wf-create"))
		Expect(w.Code).To(Equal(http.StatusCreated))

		var wf kubetaskv1.Workflow
		Expect(json.Unmarshal(w.Body.Bytes(), &wf)).To(Succeed())
		Expect(wf.Name).To(Equal("api-wf-create"))
		Expect(wf.Spec.Tasks).To(HaveLen(2))
		Expect(wf.Spec.Tasks[1].DependsOn).To(Equal([]string{"build"}))
	})

	It("should reject a workflow referencing an unknown template", func() {
		body := `{
			"metadata": { "name": "api-wf-invalid" },
			"spec": { "tasks": [ { "name": "build", "template": "nope" } ] }
		}`
		w := do("POST", "/api/v1/workflows", body)
		Expect(w.Code).To(Equal(http.StatusBadRequest))

		var resp map[string]string
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["reason"]).To(Equal("MissingTemplate"))
	})

	It("should reject a dependency cycle", func() {
		body := `{
			"metadata": { "name": "api-wf-invalid" },
			"spec": { "tasks": [
				{ "name": "a", "dependsOn": ["b"], "taskSpec": { "type": "OneTime", "image": "busybox" } },
				{ "name": "b", "dependsOn": ["a"], "taskSpec": { "type": "OneTime", "image": "busybox" } }
			] }
		}`
		w := do("POST", "/api/v1/workflows", body)
		Expect(w.Code).To(Equal(http.StatusBadRequest))

		var resp map[string]string
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["reason"]).To(Equal("CyclicDependency"))
	})

	It("should reject an empty task list", func() {
		body := `{ "metadata": { "name": "api-wf-invalid" }, "spec": { "tasks": [] } }`
		w := do("POST", "/api/v1/workflows", body)
		Expect(w.Code).To(Equal(http.StatusBadRequest))

		var resp map[string]string
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["reason"]).To(Equal("NoTasks"))
	})

	It("should reject child Task names that would be too long", func() {
		longName := "api-wf-name-that-blows-the-child-task-limit"
		defer deleteWorkflow(longName)

		body := `{
			"metadata": { "name": "` + longName + `" },
			"spec": { "tasks": [ { "name": "build", "taskSpec": { "type": "OneTime", "image": "busybox" } } ] }
		}`
		w := do("POST", "/api/v1/workflows", body)
		Expect(w.Code).To(Equal(http.StatusBadRequest))

		var resp map[string]string
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["reason"]).To(Equal("InvalidTask"))
	})

	It("should reject an inline taskSpec whose type is not OneTime (CEL)", func() {
		body := `{
			"metadata": { "name": "api-wf-invalid" },
			"spec": { "tasks": [ { "name": "build", "taskSpec": { "type": "Cron", "image": "busybox" } } ] }
		}`
		w := do("POST", "/api/v1/workflows", body)
		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})

	It("should return 409 when the workflow already exists", func() {
		Expect(do("POST", "/api/v1/workflows", workflowJSON("api-wf-create")).Code).
			To(Equal(http.StatusCreated))

		w := do("POST", "/api/v1/workflows", workflowJSON("api-wf-create"))
		Expect(w.Code).To(Equal(http.StatusConflict))
	})
})

// ==========================================================================
// GET /api/v1/workflows
// ==========================================================================

var _ = Describe("GET /api/v1/workflows", func() {
	BeforeEach(func() {
		Expect(do("POST", "/api/v1/workflows", workflowJSON("api-wf-list")).Code).
			To(Equal(http.StatusCreated))
	})

	AfterEach(func() {
		deleteWorkflow("api-wf-list")
	})

	It("should list workflows with pagination metadata", func() {
		w := do("GET", "/api/v1/workflows", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		var resp struct {
			Items    []kubetaskv1.Workflow `json:"items"`
			Total    int                   `json:"total"`
			Page     int                   `json:"page"`
			PageSize int                   `json:"pageSize"`
		}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp.Total).To(BeNumerically(">=", 1))
		Expect(resp.Page).To(Equal(1))
		Expect(resp.PageSize).To(Equal(20))
	})

	It("should filter by phase", func() {
		wf := &kubetaskv1.Workflow{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: "api-wf-list"}, wf)).To(Succeed())
		wf.Status.Phase = kubetaskv1.WorkflowFailed
		Expect(k8sClient.Status().Update(context.Background(), wf)).To(Succeed())

		w := do("GET", "/api/v1/workflows?phase=Failed", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		var resp struct {
			Items []kubetaskv1.Workflow `json:"items"`
		}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())

		names := make([]string, 0, len(resp.Items))
		for _, item := range resp.Items {
			names = append(names, item.Name)
		}
		Expect(names).To(ContainElement("api-wf-list"))
	})
})

// ==========================================================================
// GET /api/v1/workflows/:name
// ==========================================================================

var _ = Describe("GET /api/v1/workflows/:name", func() {
	AfterEach(func() {
		deleteWorkflow("api-wf-get")
		deleteWorkflow("api-wf-broken")
	})

	It("should return the workflow together with DAG graph data", func() {
		Expect(do("POST", "/api/v1/workflows", workflowJSON("api-wf-get")).Code).
			To(Equal(http.StatusCreated))

		w := do("GET", "/api/v1/workflows/api-wf-get", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		var resp struct {
			Workflow kubetaskv1.Workflow `json:"workflow"`
			Graph    struct {
				Nodes []struct {
					Name      string   `json:"name"`
					DependsOn []string `json:"dependsOn"`
					Phase     string   `json:"phase"`
				} `json:"nodes"`
				Edges []struct {
					From string `json:"from"`
					To   string `json:"to"`
				} `json:"edges"`
				Order  []string   `json:"order"`
				Levels [][]string `json:"levels"`
			} `json:"graph"`
		}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())

		Expect(resp.Workflow.Name).To(Equal("api-wf-get"))
		Expect(resp.Graph.Nodes).To(HaveLen(2))
		Expect(resp.Graph.Nodes[0].Name).To(Equal("build"))
		Expect(resp.Graph.Nodes[0].DependsOn).To(BeEmpty())
		Expect(resp.Graph.Nodes[0].Phase).To(Equal(string(kubetaskv1.NodePending)),
			"没有 Controller 跑过时，节点应默认 Pending")
		Expect(resp.Graph.Nodes[1].Name).To(Equal("test"))
		Expect(resp.Graph.Nodes[1].DependsOn).To(Equal([]string{"build"}))

		Expect(resp.Graph.Edges).To(HaveLen(1))
		Expect(resp.Graph.Edges[0].From).To(Equal("build"))
		Expect(resp.Graph.Edges[0].To).To(Equal("test"))

		Expect(resp.Graph.Order).To(Equal([]string{"build", "test"}))
		Expect(resp.Graph.Levels).To(Equal([][]string{{"build"}, {"test"}}))
	})

	It("should merge node status into the graph", func() {
		Expect(do("POST", "/api/v1/workflows", workflowJSON("api-wf-get")).Code).
			To(Equal(http.StatusCreated))

		wf := &kubetaskv1.Workflow{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: "api-wf-get"}, wf)).To(Succeed())
		now := metav1.Now()
		wf.Status.Phase = kubetaskv1.WorkflowRunning
		wf.Status.Nodes = []kubetaskv1.WorkflowNodeStatus{{
			Name:      "build",
			Phase:     kubetaskv1.NodeRunning,
			TaskName:  "api-wf-get-build",
			StartTime: &now,
		}}
		Expect(k8sClient.Status().Update(context.Background(), wf)).To(Succeed())

		w := do("GET", "/api/v1/workflows/api-wf-get", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		var resp struct {
			Graph struct {
				Nodes []struct {
					Name      string `json:"name"`
					Phase     string `json:"phase"`
					TaskName  string `json:"taskName"`
					StartTime string `json:"startTime"`
				} `json:"nodes"`
			} `json:"graph"`
		}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp.Graph.Nodes[0].Phase).To(Equal(string(kubetaskv1.NodeRunning)))
		Expect(resp.Graph.Nodes[0].TaskName).To(Equal("api-wf-get-build"))
		Expect(resp.Graph.Nodes[0].StartTime).NotTo(BeEmpty())
		Expect(resp.Graph.Nodes[1].Phase).To(Equal(string(kubetaskv1.NodePending)))
	})

	It("should still render the graph when the spec was edited outside the API", func() {
		createWorkflowDirect("api-wf-broken", kubetaskv1.WorkflowSpec{
			Tasks: []kubetaskv1.WorkflowTask{{Name: "build", Template: "missing"}},
		})

		w := do("GET", "/api/v1/workflows/api-wf-broken", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		var resp struct {
			Graph struct {
				Nodes           []map[string]any `json:"nodes"`
				Order           []string         `json:"order"`
				ValidationError string           `json:"validationError"`
			} `json:"graph"`
		}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp.Graph.ValidationError).To(ContainSubstring("MissingTemplate"))
		Expect(resp.Graph.Nodes).To(HaveLen(1), "非法 spec 也要能画出节点")
		Expect(resp.Graph.Order).To(BeEmpty())
	})

	It("should return 404 for an unknown workflow", func() {
		w := do("GET", "/api/v1/workflows/api-wf-nope", "")
		Expect(w.Code).To(Equal(http.StatusNotFound))
	})
})

// ==========================================================================
// DELETE /api/v1/workflows/:name
// ==========================================================================

var _ = Describe("DELETE /api/v1/workflows/:name", func() {
	AfterEach(func() {
		deleteWorkflow("api-wf-delete")
	})

	It("should delete the workflow", func() {
		Expect(do("POST", "/api/v1/workflows", workflowJSON("api-wf-delete")).Code).
			To(Equal(http.StatusCreated))

		w := do("DELETE", "/api/v1/workflows/api-wf-delete", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		Eventually(func() int {
			return do("GET", "/api/v1/workflows/api-wf-delete", "").Code
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(http.StatusNotFound))
	})

	It("should return 404 for an unknown workflow", func() {
		w := do("DELETE", "/api/v1/workflows/api-wf-nope", "")
		Expect(w.Code).To(Equal(http.StatusNotFound))
	})
})
