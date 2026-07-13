package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

type TaskHandler struct {
	client client.Client
}

func NewTaskHandler(client client.Client) *TaskHandler {
	return &TaskHandler{client: client}
}

// ==========================================================================
// POST /api/v1/tasks
// ==========================================================================
func (h *TaskHandler) Create(c *gin.Context) {
	var task kubetaskv1.Task
	if err := c.ShouldBindJSON(&task); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if task.APIVersion == "" {
		task.APIVersion = kubetaskv1.SchemeGroupVersion.String()
	}
	if task.Kind == "" {
		task.Kind = "Task"
	}
	if task.Name == "" {
		task.GenerateName = "kubetask-"
	}
	if task.Spec.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "spec.type is required"})
		return
	}
	if task.Spec.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "spec.image is required"})
		return
	}

	if err := h.client.Create(c.Request.Context(), &task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, task)
}

// ==========================================================================
// GET /api/v1/tasks
// ==========================================================================
func (h *TaskHandler) List(c *gin.Context) {
	// 分页
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}

	// 筛选
	filterType := c.Query("type")
	filterPhase := c.Query("phase")

	tasks := &kubetaskv1.TaskList{}
	if err := h.client.List(c.Request.Context(), tasks); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 内存筛选 + 分页
	var filtered []kubetaskv1.Task
	for _, t := range tasks.Items {
		if filterType != "" && string(t.Spec.Type) != filterType {
			continue
		}
		if filterPhase != "" && string(t.Status.Phase) != filterPhase {
			continue
		}
		filtered = append(filtered, t)
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

	result := filtered[start:end]
	if result == nil {
		result = []kubetaskv1.Task{}
	}

	c.JSON(http.StatusOK, gin.H{
		"items":    result,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// ==========================================================================
// GET /api/v1/tasks/:name
// ==========================================================================
func (h *TaskHandler) Get(c *gin.Context) {
	name := c.Param("name")

	task := &kubetaskv1.Task{}
	if err := h.client.Get(c.Request.Context(), types.NamespacedName{Name: name}, task); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// ==========================================================================
// PUT /api/v1/tasks/:name
// ==========================================================================
func (h *TaskHandler) Update(c *gin.Context) {
	name := c.Param("name")

	existing := &kubetaskv1.Task{}
	if err := h.client.Get(c.Request.Context(), types.NamespacedName{Name: name}, existing); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var updated kubetaskv1.Task
	if err := c.ShouldBindJSON(&updated); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing.Spec = updated.Spec
	existing.Labels = updated.Labels
	existing.Annotations = updated.Annotations

	if err := h.client.Update(c.Request.Context(), existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// ==========================================================================
// DELETE /api/v1/tasks/:name
// ==========================================================================
func (h *TaskHandler) Delete(c *gin.Context) {
	name := c.Param("name")

	task := &kubetaskv1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}

	if err := h.client.Delete(c.Request.Context(), task); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "task deleted"})
}

// ==========================================================================
// POST /api/v1/tasks/:name/trigger
// ==========================================================================
func (h *TaskHandler) Trigger(c *gin.Context) {
	name := c.Param("name")

	task := &kubetaskv1.Task{}
	if err := h.client.Get(c.Request.Context(), types.NamespacedName{Name: name}, task); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if task.Annotations == nil {
		task.Annotations = make(map[string]string)
	}
	task.Annotations["kubetask.io/last-trigger"] = time.Now().UTC().Format(time.RFC3339)

	if err := h.client.Update(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	task.Status.LastScheduleTime = nil
	task.Status.ActiveJobs = nil

	if err := h.client.Status().Update(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "task triggered", "task": task.Name})
}

// ==========================================================================
// GET /api/v1/stats
// ==========================================================================
func (h *TaskHandler) Stats(c *gin.Context) {
	tasks := &kubetaskv1.TaskList{}
	if err := h.client.List(c.Request.Context(), tasks); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	stats := map[string]int{
		"total":     0,
		"pending":   0,
		"running":   0,
		"succeeded": 0,
		"failed":    0,
		"suspended": 0,
		"onetime":   0,
		"cron":      0,
		"delay":     0,
	}

	for _, t := range tasks.Items {
		stats["total"]++
		switch t.Status.Phase {
		case kubetaskv1.TaskPending:
			stats["pending"]++
		case kubetaskv1.TaskRunning:
			stats["running"]++
		case kubetaskv1.TaskSucceeded:
			stats["succeeded"]++
		case kubetaskv1.TaskFailed:
			stats["failed"]++
		case kubetaskv1.TaskSuspended:
			stats["suspended"]++
		}
		switch t.Spec.Type {
		case kubetaskv1.TaskTypeCron:
			stats["cron"]++
		case kubetaskv1.TaskTypeOneTime:
			stats["onetime"]++
		case kubetaskv1.TaskTypeDelay:
			stats["delay"]++
		}
	}

	c.JSON(http.StatusOK, stats)
}

// ==========================================================================
// POST /api/v1/tasks/:name/suspend
// ==========================================================================
func (h *TaskHandler) Suspend(c *gin.Context) {
	name := c.Param("name")

	task := &kubetaskv1.Task{}
	if err := h.client.Get(c.Request.Context(), types.NamespacedName{Name: name}, task); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	suspend := true
	task.Spec.Suspend = &suspend

	if err := h.client.Update(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "task suspended", "task": task.Name})
}

// ==========================================================================
// POST /api/v1/tasks/:name/resume
// ==========================================================================
func (h *TaskHandler) Resume(c *gin.Context) {
	name := c.Param("name")

	task := &kubetaskv1.Task{}
	if err := h.client.Get(c.Request.Context(), types.NamespacedName{Name: name}, task); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	suspend := false
	task.Spec.Suspend = &suspend

	if err := h.client.Update(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "task resumed", "task": task.Name})
}

// ==========================================================================
// GET /api/v1/stats/trend
// ==========================================================================
func (h *TaskHandler) Trend(c *gin.Context) {
	tasks := &kubetaskv1.TaskList{}
	if err := h.client.List(c.Request.Context(), tasks); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type trendPoint struct {
		Date      string `json:"date"`
		Total     int    `json:"total"`
		Succeeded int    `json:"succeeded"`
		Failed    int    `json:"failed"`
	}

	trend := make(map[string]*trendPoint)

	for _, t := range tasks.Items {
		for _, rec := range t.Status.ExecutionHistory {
			date := rec.StartTime.Format("2006-01-02")
			if trend[date] == nil {
				trend[date] = &trendPoint{Date: date}
			}
			trend[date].Total++
			switch rec.Phase {
			case kubetaskv1.TaskSucceeded:
				trend[date].Succeeded++
			case kubetaskv1.TaskFailed:
				trend[date].Failed++
			}
		}
	}

	var result []trendPoint
	for _, p := range trend {
		result = append(result, *p)
	}

	if result == nil {
		result = []trendPoint{}
	}

	c.JSON(http.StatusOK, result)
}
