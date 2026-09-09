package api

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/testutil"
)

// TestSamePortWebUIAndAPI starts a real kube-apiserver via envtest, serves a
// generated SPA fixture and the REST API through one Router, and checks that
// both are reachable on the same TCP address.
func TestSamePortWebUIAndAPI(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set")
	}

	testutil.KillOrphanedEnvTestProcesses(filepath.Join("..", "..", "bin", "k8s"))

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() {
		if err := testutil.StopEnvTest(testEnv); err != nil {
			t.Errorf("stop envtest: %v", err)
		}
	}()

	scheme := k8sruntime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kubetaskv1.AddToScheme(scheme))

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	webRoot := t.TempDir()
	mustWrite(t, filepath.Join(webRoot, "index.html"), `<!doctype html><html><head><title>web</title></head><body><script type="module" src="/assets/index-smoke.js"></script></body></html>`)
	mustWrite(t, filepath.Join(webRoot, "assets", "index-smoke.js"), "console.log('smoke')")

	addr := freeTCPAddr(t)
	router := NewRouter(k8sClient, clientset, addr)
	if err := router.ServeWeb(webRoot); err != nil {
		t.Fatalf("ServeWeb(%q): %v", webRoot, err)
	}
	go func() {
		_ = router.Run()
	}()
	defer func() {
		_ = router.Shutdown(3 * time.Second)
	}()

	base := "http://" + addr
	waitForHealthz(t, base+"/healthz")

	if body := getBody(t, base+"/"); !strings.Contains(body, "<title>web</title>") {
		t.Fatalf("GET / does not return SPA index.html: %q", body)
	}

	asset := firstAsset(t, getBody(t, base+"/"))
	if asset == "" {
		t.Fatal("no script asset found in index.html")
	}
	if body := getBody(t, base+asset); body == "" {
		t.Fatalf("GET %s returned empty body", asset)
	}

	statsBody := getBody(t, base+"/api/v1/stats")
	var stats map[string]any
	if err := json.Unmarshal([]byte(statsBody), &stats); err != nil {
		t.Fatalf("GET /api/v1/stats is not JSON: %q", statsBody)
	}
	if _, ok := stats["total"]; !ok {
		t.Fatalf("stats missing total: %q", statsBody)
	}

	resp, err := http.Get(base + "/api/v1/unknown-route")
	if err != nil {
		t.Fatalf("GET unknown API route: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET unknown API route = %d, want 404", resp.StatusCode)
	}
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}
	return addr
}

func waitForHealthz(t *testing.T, url string) {
	t.Helper()
	hc := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := hc.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready", url)
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, body %q", url, resp.StatusCode, body)
	}
	return string(body)
}

func firstAsset(t *testing.T, html string) string {
	t.Helper()
	re := regexp.MustCompile(`src="(/assets/[^"]+)"`)
	m := re.FindStringSubmatch(html)
	if m == nil {
		return ""
	}
	return m[1]
}
