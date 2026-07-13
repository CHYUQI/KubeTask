package handler

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
)

type LogHandler struct {
	client    client.Client
	clientset kubernetes.Interface
}

func NewLogHandler(k8sClient client.Client, clientset kubernetes.Interface) *LogHandler {
	return &LogHandler{client: k8sClient, clientset: clientset}
}

func (h *LogHandler) Stream(c *gin.Context) {
	taskName := c.Param("name")
	tailStr := c.DefaultQuery("tail", "100")
	follow := c.DefaultQuery("follow", "false") == "true"
	sinceSeconds := c.DefaultQuery("sinceSeconds", "")

	task := &kubetaskv1.Task{}
	if err := h.client.Get(c.Request.Context(), types.NamespacedName{Name: taskName}, task); err != nil {
		if k8serrors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	jobs := &batchv1.JobList{}
	if err := h.client.List(c.Request.Context(), jobs, client.MatchingLabels{"kubetask.io/task": taskName}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(jobs.Items) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no jobs found for task"})
		return
	}

	job := jobs.Items[len(jobs.Items)-1]

	pods := &corev1.PodList{}
	if err := h.client.List(c.Request.Context(), pods,
		client.InNamespace(job.Namespace),
		client.MatchingLabels{"job-name": job.Name},
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(pods.Items) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no pods found for job"})
		return
	}

	pod := &pods.Items[len(pods.Items)-1]

	tailLines, _ := strconv.ParseInt(tailStr, 10, 64)

	logOpts := &corev1.PodLogOptions{
		Container: "task",
		TailLines: &tailLines,
		Follow:    follow,
	}

	if sinceSeconds != "" {
		if s, err := strconv.ParseInt(sinceSeconds, 10, 64); err == nil {
			logOpts.SinceSeconds = &s
		}
	}

	req := h.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
	stream, err := req.Stream(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to open log stream: %v", err)})
		return
	}
	defer stream.Close()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	c.Stream(func(w io.Writer) bool {
		scanner := bufio.NewScanner(stream)
		for scanner.Scan() {
			line := scanner.Text()
			_, err := fmt.Fprintf(w, "data: %s\n\n", line)
			if err != nil {
				return false
			}
		}
		if !follow {
			return false
		}
		return scanner.Err() == nil
	})
}
