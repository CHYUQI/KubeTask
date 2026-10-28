package notification

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	queueCapacity     = 128
	workerCount       = 2
	completedCapacity = 4096
	completedTTL      = 24 * time.Hour
	statePending      = "pending"
	stateSending      = "sending"
	stateDelivered    = "delivered"
	stateRejected     = "rejected"
	stateExhausted    = "exhausted"
)

var (
	errQueueFull = errors.New("webhook queue is full")
	errDelivery  = errors.New("webhook delivery failed")
)

type completedEvent struct {
	id string
	at time.Time
}

type deliveryRecord struct {
	state string
}

type dispatcher struct {
	queue      chan Event
	reader     client.Reader
	webhook    *webhook
	maxRetries int
	log        logr.Logger
	now        func() time.Time
	wait       func(context.Context, time.Duration) bool

	mu        sync.Mutex
	stopped   bool
	records   map[string]deliveryRecord
	completed *list.List
}

func newDispatcher(reader client.Reader, opts Options, log logr.Logger) *dispatcher {
	return &dispatcher{
		queue: make(chan Event, queueCapacity), reader: reader,
		webhook: newWebhook(opts), maxRetries: opts.MaxRetries, log: log,
		now: time.Now, wait: waitRetry,
		records: make(map[string]deliveryRecord), completed: list.New(),
	}
}

// enqueue atomically reserves the ID and publishes a snapshot. It never waits
// for a worker or an HTTP request, and never closes the queue under a producer.
func (d *dispatcher) enqueue(e Event) bool {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return false
	}
	d.prune(d.now())
	if _, found := d.records[e.ID]; found {
		d.mu.Unlock()
		return false
	}
	select {
	case d.queue <- e:
		d.records[e.ID] = deliveryRecord{state: statePending}
		d.mu.Unlock()
		return true
	default:
		d.mu.Unlock()
		d.log.Error(errQueueFull, "Webhook event dropped", "eventID", e.ID, "task", e.Task.Name,
			"job", e.Execution.JobName, "reason", "queue_full")
		return false
	}
}

// prune is called with mu held; only completed records are eligible for eviction.
func (d *dispatcher) prune(now time.Time) {
	for front := d.completed.Front(); front != nil; front = d.completed.Front() {
		entry := front.Value.(completedEvent)
		if now.Sub(entry.at) < completedTTL {
			break
		}
		delete(d.records, entry.id)
		d.completed.Remove(front)
	}
}

func (d *dispatcher) finish(id, state string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for d.completed.Len() >= completedCapacity {
		front := d.completed.Front()
		delete(d.records, front.Value.(completedEvent).id)
		d.completed.Remove(front)
	}
	d.records[id] = deliveryRecord{state: state}
	d.completed.PushBack(completedEvent{id: id, at: d.now()})
}

func (d *dispatcher) run(ctx context.Context) {
	defer d.webhook.client.CloseIdleConnections()
	var workers sync.WaitGroup
	for range workerCount {
		workers.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case e := <-d.queue:
					if ctx.Err() != nil {
						return
					}
					d.mu.Lock()
					d.records[e.ID] = deliveryRecord{state: stateSending}
					d.mu.Unlock()
					state := d.deliver(ctx, e)
					if state != "" {
						d.finish(e.ID, state)
					}
				}
			}
		})
	}
	<-ctx.Done()
	d.mu.Lock()
	d.stopped = true
	d.mu.Unlock()
	workers.Wait()
	d.mu.Lock()
	unfinished := 0
	for _, record := range d.records {
		if record.state == statePending || record.state == stateSending {
			unfinished++
		}
	}
	d.mu.Unlock()
	d.log.Info("Webhook dispatcher stopped", "unfinished", unfinished)
}

func (d *dispatcher) deliver(ctx context.Context, e Event) string {
	if d.reader != nil {
		enrich(ctx, d.reader, &e)
	}
	body, err := json.Marshal(e)
	if err != nil {
		d.log.Error(errDelivery, "Webhook event encoding failed", "eventID", e.ID, "reason", "encode_error")
		return stateRejected
	}
	log := d.log.WithValues("eventID", e.ID, "task", e.Task.Name, "job", e.Execution.JobName)
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ""
		}
		result := d.webhook.send(ctx, e.ID, body)
		if ctx.Err() != nil {
			return ""
		}
		if result.status >= 200 && result.status <= 299 {
			log.Info("Webhook delivered", "attempts", attempt+1, "statusCode", result.status)
			return stateDelivered
		}
		if !result.retry || attempt == d.maxRetries {
			log.Error(errDelivery, "Webhook delivery stopped", "attempts", attempt+1,
				"statusCode", result.status, "reason", result.reason)
			if result.retry {
				return stateExhausted
			}
			return stateRejected
		}
		delay := retryDelay(attempt)
		log.Info("Retrying webhook", "attempt", attempt+1, "statusCode", result.status,
			"reason", result.reason, "retryAfter", delay)
		if !d.wait(ctx, delay) {
			return ""
		}
	}
	return stateExhausted
}
