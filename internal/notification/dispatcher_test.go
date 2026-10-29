package notification

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestDeliveryRetriesStablePayload(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	var ids []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies = append(bodies, body)
		ids = append(ids, r.Header.Get("X-KubeTask-Event-ID"))
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Error("unexpected HTTP contract")
		}
		statuses := []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusNoContent}
		w.WriteHeader(statuses[len(bodies)-1])
	}))
	defer server.Close()
	d := newDispatcher(nil, Options{URL: server.URL, Timeout: time.Second, MaxRetries: 3}, logr.Discard())
	defer d.webhook.client.CloseIdleConnections()
	var delays []time.Duration
	d.wait = func(_ context.Context, delay time.Duration) bool { delays = append(delays, delay); return true }
	if state := d.deliver(context.Background(), testEvent(t)); state != stateDelivered {
		t.Fatalf("delivery state=%q", state)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 3 || len(delays) != 2 || delays[0] != time.Second || delays[1] != 2*time.Second {
		t.Fatalf("unexpected retry schedule: %d attempts, %v", len(bodies), delays)
	}
	for i := range bodies {
		if !bytes.Equal(bodies[0], bodies[i]) || ids[i] != testEventID {
			t.Fatal("payload or ID changed on retry")
		}
	}
}

func TestDeliveryStopsOnRejectionAndExhaustion(t *testing.T) {
	for _, tt := range []struct {
		name                      string
		status, retries, attempts int
		state                     string
	}{
		{name: "unauthorized", status: 401, retries: 3, attempts: 1, state: stateRejected},
		{name: "redirect", status: 307, retries: 3, attempts: 1, state: stateRejected},
		{name: "exhausted", status: 503, retries: 3, attempts: 4, state: stateExhausted},
		{name: "retries disabled", status: 500, retries: 0, attempts: 1, state: stateExhausted},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.Header().Set("Location", "/must-not-follow")
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			d := newDispatcher(nil, Options{URL: server.URL, Timeout: time.Second, MaxRetries: tt.retries}, logr.Discard())
			defer d.webhook.client.CloseIdleConnections()
			d.wait = func(context.Context, time.Duration) bool { return true }
			if state := d.deliver(context.Background(), testEvent(t)); state != tt.state || int(attempts.Load()) != tt.attempts {
				t.Fatalf("state=%q attempts=%d, want %q/%d", state, attempts.Load(), tt.state, tt.attempts)
			}
		})
	}
}

func TestWebhookTimeoutAndCanceledRetry(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	w := newWebhook(Options{URL: server.URL, Timeout: 30 * time.Millisecond})
	defer w.client.CloseIdleConnections()
	result := w.send(context.Background(), testEventID, []byte("{}"))
	if !result.retry || result.reason != "timeout" {
		t.Fatalf("timeout result: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitRetry(ctx, time.Hour) {
		t.Fatal("canceled backoff continued")
	}
	result = w.send(ctx, testEventID, nil)
	if result.retry || result.reason != "canceled" {
		t.Fatalf("canceled result: %+v", result)
	}
	if retryDelay(5) != 30*time.Second {
		t.Fatal("backoff cap is incorrect")
	}
}

func TestQueueBoundAndConcurrentDeduplication(t *testing.T) {
	d := newDispatcher(nil, Options{Timeout: time.Second}, logr.Discard())
	e := testEvent(t)
	var accepted atomic.Int32
	var producers sync.WaitGroup
	for range 32 {
		producers.Go(func() {
			if d.enqueue(e) {
				accepted.Add(1)
			}
		})
	}
	producers.Wait()
	if accepted.Load() != 1 || len(d.queue) != 1 {
		t.Fatal("concurrent enqueue bypassed deduplication")
	}
	for i := 1; i < queueCapacity; i++ {
		e.ID = fmt.Sprintf("event-%d", i)
		if !d.enqueue(e) {
			t.Fatal("queue filled too early")
		}
	}
	e.ID = "overflow"
	if d.enqueue(e) || len(d.records) != queueCapacity {
		t.Fatal("queue overflow grew state or accepted event")
	}
	<-d.queue
	if !d.enqueue(e) {
		t.Fatal("dropped event was incorrectly recorded as pending")
	}
}

func TestCompletedEvictionPreservesPending(t *testing.T) {
	d := newDispatcher(nil, Options{Timeout: time.Second}, logr.Discard())
	now := time.Now()
	d.now = func() time.Time { return now }
	pending := testEvent(t)
	if !d.enqueue(pending) {
		t.Fatal("enqueue failed")
	}
	for i := range completedCapacity + 1 {
		d.finish(fmt.Sprintf("done-%d", i), stateDelivered)
	}
	if d.completed.Len() != completedCapacity || len(d.records) != completedCapacity+1 {
		t.Fatal("completed records are not bounded independently of pending records")
	}
	if _, exists := d.records["done-0"]; exists {
		t.Fatal("oldest completed record was not evicted")
	}
	if d.enqueue(pending) {
		t.Fatal("pending record was evicted")
	}
	now = now.Add(completedTTL)
	e := pending
	e.ID = "after-expiry"
	if !d.enqueue(e) || d.completed.Len() != 0 || len(d.records) != 2 {
		t.Fatal("completed expiry is incorrect")
	}
}

func TestDispatcherCancelsInflightAndStopsAccepting(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	d := newDispatcher(nil, Options{URL: server.URL, Timeout: time.Minute, MaxRetries: 3}, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { d.run(ctx); close(done) }()
	if !d.enqueue(testEvent(t)) {
		t.Fatal("enqueue failed")
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("dispatcher did not cancel request")
	}
	e := testEvent(t)
	e.ID = "after-stop"
	if d.enqueue(e) {
		t.Fatal("dispatcher accepted event after stop")
	}
}

func TestFailureLogsDoNotExposeWebhookURL(t *testing.T) {
	var logs bytes.Buffer
	log := zapr.NewLogger(zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&logs), zap.InfoLevel)))
	d := newDispatcher(nil, Options{URL: "https://invalid.example/hook?secret=private-token", Timeout: time.Second}, log)
	// An invalid header causes the real transport to reject the request before networking;
	// url.Error still includes the complete secret-bearing target URL.
	e := testEvent(t)
	e.ID = "invalid\nheader"
	if state := d.deliver(context.Background(), e); state != stateExhausted {
		t.Fatal("expected network failure")
	}
	for _, secret := range []string{"private-token", "invalid.example", "/hook?secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("log contains webhook address or credentials")
		}
	}
}
