package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMountWebUI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "index.html"), "<!doctype html><title>KubeTask</title>")
	mustWrite(t, filepath.Join(root, "assets", "app.js"), "console.log('app')")

	engine := gin.New()
	engine.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	v1 := engine.Group("/api/v1")
	v1.GET("/tasks", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	if err := MountWebUI(engine, root); err != nil {
		t.Fatalf("MountWebUI failed: %v", err)
	}

	req := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}

	if w := req(http.MethodGet, "/"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "KubeTask") {
		t.Fatalf("GET / = %d, body %q", w.Code, w.Body.String())
	}
	if w := req(http.MethodGet, "/assets/app.js"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "console.log") {
		t.Fatalf("GET /assets/app.js = %d, body %q", w.Code, w.Body.String())
	}
	if w := req(http.MethodGet, "/api/v1/tasks"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("GET /api/v1/tasks = %d, body %q", w.Code, w.Body.String())
	}
	if w := req(http.MethodGet, "/api/v1/unknown"); w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/unknown = %d, want 404", w.Code)
	}
	if w := req(http.MethodPost, "/"); w.Code != http.StatusNotFound {
		t.Fatalf("POST / = %d, want 404", w.Code)
	}
	if w := req(http.MethodGet, "/missing.js"); w.Code != http.StatusNotFound {
		t.Fatalf("GET /missing.js = %d, want 404", w.Code)
	}
}

func TestMountWebUIRequiresIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	empty := t.TempDir()
	if err := MountWebUI(engine, empty); err == nil {
		t.Fatal("MountWebUI should fail without index.html")
	}
	if err := MountWebUI(engine, ""); err != nil {
		t.Fatalf("empty webRoot should be a no-op, got %v", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
