package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/api/handler"
	"kubetask.io/kubetask/internal/testutil"
)

var (
	testEnv   *envtest.Environment
	k8sClient client.Client
	router    *gin.Engine
)

func TestAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testutil.KillOrphanedEnvTestProcesses(filepath.Join("..", "..", "..", "bin", "k8s"))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	scheme := k8sruntime.NewScheme()
	Expect(kubetaskv1.AddToScheme(scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	h := handler.NewTaskHandler(k8sClient)

	engine := gin.New()
	v1 := engine.Group("/api/v1")
	{
		v1.POST("/tasks", h.Create)
		v1.GET("/tasks", h.List)
		v1.GET("/tasks/:name", h.Get)
		v1.PUT("/tasks/:name", h.Update)
		v1.DELETE("/tasks/:name", h.Delete)
		v1.POST("/tasks/:name/trigger", h.Trigger)
		v1.POST("/tasks/:name/suspend", h.Suspend)
		v1.POST("/tasks/:name/resume", h.Resume)
		v1.GET("/stats", h.Stats)
		v1.GET("/stats/trend", h.Trend)
	}
	router = engine
})

var _ = AfterSuite(func() {
	Expect(testutil.StopEnvTest(testEnv)).To(Succeed())
})

// ==========================================================================
// Helper
// ==========================================================================
func do(method, path string, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func taskJSON(name, image string, command ...string) string {
	cmd, _ := json.Marshal(command)
	return `{
		"apiVersion": "kubetask.kubetask.io/v1",
		"kind": "Task",
		"metadata": { "name": "` + name + `" },
		"spec": {
			"type": "OneTime",
			"image": "` + image + `",
			"command": ` + string(cmd) + `
		}
	}`
}

// ==========================================================================
// Tests
// ==========================================================================

var _ = Describe("POST /api/v1/tasks", func() {
	AfterEach(func() {
		deleteTask("api-create")
	})

	It("should create a task and return 201", func() {
		w := do("POST", "/api/v1/tasks", taskJSON("api-create", "busybox", "echo", "hi"))
		Expect(w.Code).To(Equal(http.StatusCreated))

		var task kubetaskv1.Task
		Expect(json.Unmarshal(w.Body.Bytes(), &task)).To(Succeed())
		Expect(task.Name).To(Equal("api-create"))
		Expect(task.Spec.Type).To(Equal(kubetaskv1.TaskTypeOneTime))
	})

	It("should reject missing type", func() {
		w := do("POST", "/api/v1/tasks", `{"metadata":{"name":"x"},"spec":{"image":"b"}}`)
		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})

	It("should reject invalid cron schedule", func() {
		w := do("POST", "/api/v1/tasks", `{"metadata":{"name":"api-cron-invalid"},"spec":{"type":"Cron","image":"busybox","schedule":"bad"}}`)
		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})
})

var _ = Describe("GET /api/v1/tasks", func() {
	BeforeEach(func() {
		w := do("POST", "/api/v1/tasks", taskJSON("api-list-1", "busybox"))
		Expect(w.Code).To(Equal(http.StatusCreated))
	})

	AfterEach(func() {
		deleteTask("api-list-1")
	})

	It("should list tasks", func() {
		w := do("GET", "/api/v1/tasks", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		var resp struct {
			Items []kubetaskv1.Task `json:"items"`
			Total int               `json:"total"`
		}
		Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp.Total).To(BeNumerically(">=", 1))
	})
})

var _ = Describe("GET /api/v1/tasks/:name", func() {
	BeforeEach(func() {
		w := do("POST", "/api/v1/tasks", taskJSON("api-get", "alpine"))
		Expect(w.Code).To(Equal(http.StatusCreated))
	})

	AfterEach(func() {
		deleteTask("api-get")
	})

	It("should return the task", func() {
		w := do("GET", "/api/v1/tasks/api-get", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		var task kubetaskv1.Task
		Expect(json.Unmarshal(w.Body.Bytes(), &task)).To(Succeed())
		Expect(task.Name).To(Equal("api-get"))
		Expect(task.Spec.Image).To(Equal("alpine"))
	})

	It("should return 404 for unknown task", func() {
		w := do("GET", "/api/v1/tasks/no-such", "")
		Expect(w.Code).To(Equal(http.StatusNotFound))
	})
})

var _ = Describe("PUT /api/v1/tasks/:name", func() {
	BeforeEach(func() {
		do("POST", "/api/v1/tasks", taskJSON("api-update", "alpine"))
	})

	AfterEach(func() {
		deleteTask("api-update")
	})

	It("should update task spec", func() {
		body := `{"spec":{"type":"Delay","image":"ubuntu","command":["sleep","10"]}}`
		w := do("PUT", "/api/v1/tasks/api-update", body)
		Expect(w.Code).To(Equal(http.StatusOK))

		w = do("GET", "/api/v1/tasks/api-update", "")
		var task kubetaskv1.Task
		Expect(json.Unmarshal(w.Body.Bytes(), &task)).To(Succeed())
		Expect(task.Spec.Image).To(Equal("ubuntu"))
		Expect(task.Spec.Type).To(Equal(kubetaskv1.TaskTypeDelay))
	})
})

var _ = Describe("DELETE /api/v1/tasks/:name", func() {
	BeforeEach(func() {
		do("POST", "/api/v1/tasks", taskJSON("api-delete", "busybox"))
	})

	It("should delete the task", func() {
		w := do("DELETE", "/api/v1/tasks/api-delete", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		w = do("GET", "/api/v1/tasks/api-delete", "")
		Expect(w.Code).To(Equal(http.StatusNotFound))
	})
})

var _ = Describe("POST /api/v1/tasks/:name/suspend + resume", func() {
	BeforeEach(func() {
		do("POST", "/api/v1/tasks", taskJSON("api-pause", "busybox"))
	})

	AfterEach(func() {
		deleteTask("api-pause")
	})

	It("should suspend a task", func() {
		w := do("POST", "/api/v1/tasks/api-pause/suspend", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		w = do("GET", "/api/v1/tasks/api-pause", "")
		var task kubetaskv1.Task
		Expect(json.Unmarshal(w.Body.Bytes(), &task)).To(Succeed())
		Expect(*task.Spec.Suspend).To(BeTrue())
	})

	It("should resume a suspended task", func() {
		do("POST", "/api/v1/tasks/api-pause/suspend", "")
		w := do("POST", "/api/v1/tasks/api-pause/resume", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		w = do("GET", "/api/v1/tasks/api-pause", "")
		var task kubetaskv1.Task
		Expect(json.Unmarshal(w.Body.Bytes(), &task)).To(Succeed())
		Expect(*task.Spec.Suspend).To(BeFalse())
	})
})

var _ = Describe("POST /api/v1/tasks/:name/trigger", func() {
	BeforeEach(func() {
		do("POST", "/api/v1/tasks", `{"apiVersion":"kubetask.kubetask.io/v1","kind":"Task","metadata":{"name":"api-trig"},"spec":{"type":"Cron","image":"busybox","command":["echo"],"schedule":"0 0 * * *"}}`)
	})

	AfterEach(func() {
		deleteTask("api-trig")
	})

	It("should trigger a task", func() {
		w := do("POST", "/api/v1/tasks/api-trig/trigger", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		w = do("GET", "/api/v1/tasks/api-trig", "")
		var task kubetaskv1.Task
		Expect(json.Unmarshal(w.Body.Bytes(), &task)).To(Succeed())
		Expect(task.Annotations["kubetask.io/last-trigger"]).NotTo(BeEmpty())
	})
})

var _ = Describe("GET /api/v1/stats", func() {
	It("should return stats object", func() {
		w := do("GET", "/api/v1/stats", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		var stats map[string]int
		Expect(json.Unmarshal(w.Body.Bytes(), &stats)).To(Succeed())
		Expect(stats).To(HaveKey("total"))
		Expect(stats).To(HaveKey("running"))
		Expect(stats).To(HaveKey("cron"))
	})
})

var _ = Describe("GET /api/v1/stats/trend", func() {
	It("should return trend array", func() {
		w := do("GET", "/api/v1/stats/trend", "")
		Expect(w.Code).To(Equal(http.StatusOK))

		var trend []map[string]any
		Expect(json.Unmarshal(w.Body.Bytes(), &trend)).To(Succeed())
	})
})

// ==========================================================================
// Cleanup helper
// ==========================================================================
func deleteTask(name string) {
	task := &kubetaskv1.Task{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_ = k8sClient.Delete(context.Background(), task)
}
