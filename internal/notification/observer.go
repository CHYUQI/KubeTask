package notification

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

type observer struct {
	cache      cache.Cache
	dispatcher *dispatcher
}

// SetupWithManager registers the outbound notification pipeline. Call only when
// enabled. Both observation and sending start after this manager becomes leader.
func SetupWithManager(mgr ctrl.Manager, opts Options) error {
	log := ctrl.Log.WithName("notification")
	d := newDispatcher(mgr.GetCache(), opts, log)
	return mgr.Add(&observer{cache: mgr.GetCache(), dispatcher: d})
}

func (o *observer) NeedLeaderElection() bool { return true }

func (o *observer) Start(ctx context.Context) error {
	informer, err := o.cache.GetInformer(ctx, &batchv1.Job{})
	if err != nil {
		return fmt.Errorf("get notification Job informer: %w", err)
	}
	registration, err := informer.AddEventHandler(o.handler(ctx))
	if err != nil {
		return fmt.Errorf("register notification Job handler: %w", err)
	}
	defer func() {
		if err := informer.RemoveEventHandler(registration); err != nil {
			o.dispatcher.log.Error(err, "Failed to remove notification handler")
		}
	}()
	o.dispatcher.log.Info("Webhook dispatcher started", "timeout", o.dispatcher.webhook.client.Timeout,
		"maxRetries", o.dispatcher.maxRetries, "queueCapacity", queueCapacity, "workers", workerCount)
	o.dispatcher.run(ctx)
	return nil
}

func (o *observer) handler(ctx context.Context) toolscache.ResourceEventHandler {
	return toolscache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj any, initial bool) {
			if !initial {
				o.observe(ctx, obj)
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			oldJob, oldOK := oldObj.(*batchv1.Job)
			newJob, newOK := newObj.(*batchv1.Job)
			if !oldOK || !newOK || (oldJob.UID == newJob.UID && failureCondition(oldJob) != nil) {
				return
			}
			o.observe(ctx, newJob)
		},
	}
}

func (o *observer) observe(ctx context.Context, obj any) {
	job, ok := obj.(*batchv1.Job)
	if !ok || ctx.Err() != nil {
		return
	}
	if e, eligible := snapshot(job, time.Now()); eligible {
		o.dispatcher.enqueue(e)
	}
}
