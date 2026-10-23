package handler

import (
	stdErrors "errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/workflow"
)

const (
	workflowNotFoundMessage = "workflow not found"
)

type WorkflowHandler struct {
	client client.Client
}

func NewWorkflowHandler(k8sClient client.Client) *WorkflowHandler {
	return &WorkflowHandler{client: k8sClient}
}

// ==========================================================================
// POST /api/v1/workflows
// ==========================================================================

func (h *WorkflowHandler) Create(c *gin.Context) {
	var wf kubetaskv1.Workflow
	if err := c.ShouldBindJSON(&wf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: err.Error()})
		return
	}

	if wf.APIVersion == "" {
		wf.APIVersion = kubetaskv1.SchemeGroupVersion.String()
	}
	if wf.Kind == "" {
		wf.Kind = "Workflow"
	}
	if wf.Name == "" {
		wf.GenerateName = "workflow-"
	}

	// CEL 管不到引用、依赖环和子 Task 名长度，这些在入口先校验一次，
	// 让用户拿到 400 + Reason，而不是创建出一个马上 Failed 的 Workflow。
	if err := validateWorkflowSpec(wf.Name, wf.Spec); err != nil {
		writeWorkflowValidationError(c, err)
		return
	}

	if err := h.client.Create(c.Request.Context(), &wf); err != nil {
		switch {
		case errors.IsAlreadyExists(err):
			c.JSON(http.StatusConflict, gin.H{jsonKeyError: "workflow already exists"})
		case errors.IsInvalid(err), errors.IsBadRequest(err):
			// CRD 结构校验 / CEL 规则拒绝：直接把 API Server 的说明透出去。
			c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, wf)
}

// ==========================================================================
// GET /api/v1/workflows
// ==========================================================================

func (h *WorkflowHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	filterPhase := c.Query("phase")

	workflows := &kubetaskv1.WorkflowList{}
	if err := h.client.List(c.Request.Context(), workflows); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: err.Error()})
		return
	}

	filtered := make([]kubetaskv1.Workflow, 0, len(workflows.Items))
	for _, wf := range workflows.Items {
		if filterPhase != "" && string(wf.Status.Phase) != filterPhase {
			continue
		}
		filtered = append(filtered, wf)
	}

	total := len(filtered)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	c.JSON(http.StatusOK, gin.H{
		"items":      filtered[start:end],
		jsonKeyTotal: total,
		"page":       page,
		"pageSize":   pageSize,
	})
}

// ==========================================================================
// GET /api/v1/workflows/:name
// ==========================================================================

// 详情返回原始对象 + 前端 DAG 视图需要的图数据（节点、边、拓扑序、并行层）。
func (h *WorkflowHandler) Get(c *gin.Context) {
	name := c.Param("name")

	wf := &kubetaskv1.Workflow{}
	if err := h.client.Get(c.Request.Context(), types.NamespacedName{Name: name}, wf); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{jsonKeyError: workflowNotFoundMessage})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workflow": wf,
		"graph":    buildWorkflowGraph(wf),
	})
}

// ==========================================================================
// DELETE /api/v1/workflows/:name
// ==========================================================================

// 删除是异步的：Workflow 带 finalizer，Controller 会先清理子 Task 再真正删除。
func (h *WorkflowHandler) Delete(c *gin.Context) {
	name := c.Param("name")

	wf := &kubetaskv1.Workflow{}
	if err := h.client.Get(c.Request.Context(), types.NamespacedName{Name: name}, wf); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{jsonKeyError: workflowNotFoundMessage})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: err.Error()})
		return
	}

	if err := h.client.Delete(c.Request.Context(), wf); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{jsonKeyError: workflowNotFoundMessage})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{jsonKeyMessage: "workflow deleting", "workflow": wf.Name})
}

// ==========================================================================
// 校验辅助
// ==========================================================================

// validateWorkflowSpec 覆盖 CEL 管不到的运行时规则：模板/依赖引用、依赖环、子 Task 名长度。
// 名字为空（走 generateName）时跳过与名字相关的检查。
func validateWorkflowSpec(name string, spec kubetaskv1.WorkflowSpec) error {
	graph, err := workflow.Build(spec)
	if err != nil {
		return err
	}
	if name == "" {
		return nil
	}
	return workflow.ValidateChildTaskNames(name, graph)
}

func writeWorkflowValidationError(c *gin.Context, err error) {
	if validationErr, ok := stdErrors.AsType[*workflow.ValidationError](err); ok {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: validationErr.Message, "reason": validationErr.Reason})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: err.Error()})
}
