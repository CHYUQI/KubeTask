package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// MountWebUI serves a built SPA from webRoot on the given engine.
// Registered routes such as /healthz and /api/v1/* keep priority; only
// unmatched requests fall through to the static file server. It returns an
// error when webRoot is non-empty but has no index.html, so callers can
// decide whether to keep running without the UI.
func MountWebUI(engine *gin.Engine, webRoot string) error {
	if webRoot == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(webRoot, "index.html")); err != nil {
		return fmt.Errorf("web root %q has no index.html: %w", webRoot, err)
	}
	fileServer := http.FileServer(http.Dir(webRoot))
	engine.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.String(http.StatusNotFound, "404 page not found")
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.String(http.StatusNotFound, "404 page not found")
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	return nil
}
