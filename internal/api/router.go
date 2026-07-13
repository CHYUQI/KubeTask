package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kubetask.io/kubetask/internal/api/handler"
)

type Router struct {
	engine *gin.Engine
	addr   string
}

func NewRouter(k8sClient client.Client, clientset kubernetes.Interface, addr string) *Router {
	engine := gin.New()
	engine.Use(gin.Recovery())

	taskH := handler.NewTaskHandler(k8sClient)
	logH := handler.NewLogHandler(k8sClient, clientset)

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := engine.Group("/api/v1")
	{
		v1.POST("/tasks", taskH.Create)
		v1.GET("/tasks", taskH.List)
		v1.GET("/tasks/:name", taskH.Get)
		v1.PUT("/tasks/:name", taskH.Update)
		v1.DELETE("/tasks/:name", taskH.Delete)
		v1.POST("/tasks/:name/trigger", taskH.Trigger)
		v1.POST("/tasks/:name/suspend", taskH.Suspend)
		v1.POST("/tasks/:name/resume", taskH.Resume)
		v1.GET("/tasks/:name/logs", logH.Stream)
		v1.GET("/stats", taskH.Stats)
		v1.GET("/stats/trend", taskH.Trend)
	}

	return &Router{
		engine: engine,
		addr:   addr,
	}
}

func (r *Router) Run() error {
	return r.engine.Run(r.addr)
}
