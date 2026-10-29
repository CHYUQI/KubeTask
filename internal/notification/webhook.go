package notification

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

// Options contains already validated global webhook configuration.
type Options struct {
	URL        string
	Timeout    time.Duration
	MaxRetries int
}

type webhook struct {
	url    string
	client *http.Client
}

type attemptResult struct {
	status int
	retry  bool
	reason string
}

func newWebhook(opts Options) *webhook {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &webhook{
		url: opts.URL,
		client: &http.Client{
			Timeout: opts.Timeout, Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (w *webhook) send(ctx context.Context, id string, body []byte) attemptResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return attemptResult{reason: "request_invalid"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-KubeTask-Event-ID", id)
	resp, err := w.client.Do(req)
	if err != nil {
		// Never return a raw transport error: url.Error embeds the webhook URL.
		if ctx.Err() != nil {
			return attemptResult{reason: "canceled"}
		}
		reason := "network_error"
		if timeout, ok := errors.AsType[net.Error](err); ok && timeout.Timeout() {
			reason = "timeout"
		}
		return attemptResult{retry: true, reason: reason}
	}
	// Drain only a small body for connection reuse. Delivery depends on the
	// status code, never on response content or a body read error.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	_ = resp.Body.Close()
	return attemptResult{
		status: resp.StatusCode,
		retry:  resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode <= 599),
		reason: "http_status",
	}
}

func retryDelay(retry int) time.Duration {
	delay := time.Second << retry
	return min(delay, 30*time.Second)
}

func waitRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}
