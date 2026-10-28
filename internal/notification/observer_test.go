package notification

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

func TestObserverOnlyAcceptsNewFailures(t *testing.T) {
	d := newDispatcher(nil, Options{Timeout: time.Second}, logr.Discard())
	o := &observer{dispatcher: d}
	if !o.NeedLeaderElection() {
		t.Fatal("notification pipeline must be leader scoped")
	}
	h := o.handler(context.Background())
	failed := failedJob()
	running := failed.DeepCopy()
	running.Status.Conditions = nil
	h.OnAdd(failed, true)
	h.OnUpdate(failed, failed.DeepCopy())
	h.OnAdd(running, true)
	if len(d.queue) != 0 {
		t.Fatal("initial or unchanged failure was queued")
	}
	h.OnUpdate(running, failed)
	h.OnUpdate(running, failed)
	if len(d.queue) != 1 {
		t.Fatal("transition must enqueue exactly once")
	}
	fresh := failed.DeepCopy()
	fresh.UID = types.UID("fresh-job")
	h.OnAdd(fresh, false)
	if len(d.queue) != 2 {
		t.Fatal("newly discovered terminal Job was missed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o.handler(ctx).OnAdd(failed, false)
	if len(d.queue) != 2 {
		t.Fatal("accepted event after shutdown")
	}
	fresh.Status.Conditions[0].Message = "changed in informer"
	<-d.queue
	e := <-d.queue
	if e.Execution.Message == "changed in informer" {
		t.Fatal("queue retained mutable informer object")
	}
}
