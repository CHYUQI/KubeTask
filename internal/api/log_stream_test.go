package api

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	authenticationv1 "k8s.io/api/authentication/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/api/middleware"
)

func TestAuthenticatedLogStreamContinuesAfterAuthenticationTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	second := make(chan struct{})
	upstreamDone := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(upstreamDone)
		if r.URL.Path != "/api/v1/namespaces/default/pods/stream-pod/log" || r.URL.Query().Get("follow") != "true" {
			t.Error("unexpected upstream log request")
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprintln(w, "first")
		w.(http.Flusher).Flush()
		select {
		case <-second:
			_, _ = fmt.Fprintln(w, "second")
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
		<-r.Context().Done()
	}))
	defer upstream.Close()
	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{kubetaskv1.AddToScheme, batchv1.AddToScheme, corev1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&kubetaskv1.Task{Name: "stream-task"},
		&batchv1.Job{Name: "stream-job", Namespace: "default", Labels: map[string]string{"kubetask.io/task": "stream-task"}},
		&corev1.Pod{Name: "stream-pod", Namespace: "default", Labels: map[string]string{"job-name": "stream-job"}},
	).Build()
	var calls atomic.Int32
	a := routerAuthenticatorFunc(func(context.Context, string) (*authenticationv1.UserInfo, error) {
		calls.Add(1)
		return &authenticationv1.UserInfo{Username: "system:serviceaccount:team-a:ci"}, nil
	})
	router := NewRouter(k8sClient, clientset, "", middleware.Authentication(a))
	server := httptest.NewServer(router.engine)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/tasks/stream-task/logs?follow=true", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer opaque-token")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open live SSE stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatal("expected an authenticated SSE response")
	}
	reader := bufio.NewReader(resp.Body)
	readEvent := func(want string) {
		t.Helper()
		line, err := reader.ReadString('\n')
		if err != nil || line != "data: "+want+"\n" {
			t.Fatalf("live SSE event was not delivered: %v", err)
		}
		if separator, err := reader.ReadString('\n'); err != nil || separator != "\n" {
			t.Fatal("invalid SSE event separator")
		}
	}
	readEvent("first")
	timer := time.NewTimer(3200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("request ended before the authentication timeout boundary")
	}
	close(second)
	readEvent("second")
	if calls.Load() != 1 {
		t.Errorf("authentication calls = %d, want one per SSE request", calls.Load())
	}
	cancel()
	select {
	case <-upstreamDone:
	case <-time.After(2 * time.Second):
		t.Error("client cancellation did not close the upstream log request")
	}
}
