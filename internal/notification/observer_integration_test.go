package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/testutil"
)

// Track the real listener's initial replay, so tests need no startup sleep and
// exercise the same warm-cache registration path as the Manager runnable.
type registrationCache struct {
	cache.Cache
	registered chan toolscache.ResourceEventHandlerRegistration
}

type registrationInformer struct {
	cache.Informer
	registered chan toolscache.ResourceEventHandlerRegistration
}

func (c *registrationCache) GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error) {
	i, err := c.Cache.GetInformer(ctx, obj, opts...)
	if err != nil {
		return nil, err
	}
	return &registrationInformer{Informer: i, registered: c.registered}, nil
}

func (i *registrationInformer) AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	r, err := i.Informer.AddEventHandler(handler)
	if err == nil {
		i.registered <- r
	}
	return r, err
}

func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if check() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for informer")
		case <-tick.C:
		}
	}
}

func startNotificationManager(t *testing.T, cfg *rest.Config, scheme *runtime.Scheme, url string) (*dispatcher, func()) {
	t.Helper()
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme, Logger: logr.Discard(), LeaderElection: false,
		Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := &registrationCache{Cache: mgr.GetCache(), registered: make(chan toolscache.ResourceEventHandlerRegistration, 1)}
	d := newDispatcher(mgr.GetCache(), Options{URL: url, Timeout: time.Second}, logr.Discard())
	if err := mgr.Add(&observer{cache: c, dispatcher: d}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(10 * time.Second):
				t.Error("manager failed to stop")
			}
		})
	}
	t.Cleanup(stop)
	select {
	case r := <-c.registered:
		eventually(t, r.HasSynced)
	case <-time.After(10 * time.Second):
		t.Fatal("notification handler was not registered")
	}
	return d, stop
}

func TestObserverWithRealInformer(t *testing.T) {
	testutil.KillOrphanedEnvTestProcesses(filepath.Join("..", "..", "bin", "k8s"))
	controlPlane := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")}, ErrorIfCRDPathMissing: true}
	cfg, err := controlPlane.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testutil.StopEnvTest(controlPlane); err != nil {
			t.Error(err)
		}
	})
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := kubetaskv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	k8s, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ns := &corev1.Namespace{Name: "webhook-test"}
	if err := k8s.Create(ctx, ns); err != nil {
		t.Fatal(err)
	}
	task := &kubetaskv1.Task{
		Name: testTaskName, Labels: map[string]string{"kubetask.io/workflow-node": testNode},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: kubetaskv1.GroupVersion.String(), Kind: testWorkflowKind,
			Name: testWorkflowName, UID: testWorkflowUID, Controller: new(true),
		}},
		Spec: kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: "busybox:1.37", Suspend: new(true)},
	}
	if err := k8s.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	createJob := func(name string, terminal bool) *batchv1.Job {
		t.Helper()
		job := failedJob()
		job.Name, job.Namespace, job.UID = name, ns.Name, ""
		job.OwnerReferences[0].UID = task.UID
		status := job.Status
		job.Status = batchv1.JobStatus{}
		if err := k8s.Create(ctx, job); err != nil {
			t.Fatal(err)
		}
		if terminal {
			job.Status = status
			if err := k8s.Status().Update(ctx, job); err != nil {
				t.Fatal(err)
			}
		}
		return job
	}
	historical := createJob("historical-failed", true)
	running := createJob("running-then-failed", false)
	received := make(chan Event, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e Event
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		received <- e
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	d, stop := startNotificationManager(t, cfg, scheme, server.URL)
	d.mu.Lock()
	initialCount := len(d.records)
	d.mu.Unlock()
	if initialCount != 0 {
		t.Fatal("initial failed Job was replayed")
	}
	historical.Labels = map[string]string{"updated": "true"}
	if err := k8s.Update(ctx, historical); err != nil {
		t.Fatal(err)
	}
	running.Status = failedJob().Status
	if err := k8s.Status().Update(ctx, running); err != nil {
		t.Fatal(err)
	}
	fresh := createJob("fresh-failed", true)
	want := map[string]bool{string(running.UID): true, string(fresh.UID): true}
	for range 2 {
		select {
		case e := <-received:
			if !want[string(e.Execution.JobUID)] {
				t.Fatalf("unexpected event: %+v", e)
			}
			delete(want, string(e.Execution.JobUID))
			if e.Workflow == nil || e.Workflow.Node != testNode || e.Task.Type != kubetaskv1.TaskTypeOneTime {
				t.Fatalf("missing suspended child Task context: %+v", e)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("missing failure event")
		}
	}
	// Restart over the same already-failed Jobs: the new listener must not backfill.
	stop()
	d, stop = startNotificationManager(t, cfg, scheme, server.URL)
	d.mu.Lock()
	restartedCount := len(d.records)
	d.mu.Unlock()
	if restartedCount != 0 {
		t.Fatal("restart replayed historical failures")
	}
	select {
	case e := <-received:
		t.Fatalf("unexpected historical notification: %+v", e)
	default:
	}
	stop()
}
