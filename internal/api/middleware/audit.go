package middleware

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	eventsclient "k8s.io/client-go/kubernetes/typed/events/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/auth"
	"kubetask.io/kubetask/internal/tenant"
)

const (
	auditTargetKey = "kubetask.audit.target"
	auditTask      = "Task"
	auditWorkflow  = "Workflow"
)

type auditOperation struct{ kind, action string }

var auditOperations = map[string]auditOperation{
	"POST /api/v1/tasks":               {auditTask, "Create"},
	"PUT /api/v1/tasks/:name":          {auditTask, "Update"},
	"DELETE /api/v1/tasks/:name":       {auditTask, "Delete"},
	"POST /api/v1/tasks/:name/trigger": {auditTask, "Trigger"},
	"POST /api/v1/tasks/:name/suspend": {auditTask, "Suspend"},
	"POST /api/v1/tasks/:name/resume":  {auditTask, "Resume"},
	"POST /api/v1/workflows":           {auditWorkflow, "Create"},
	"DELETE /api/v1/workflows/:name":   {auditWorkflow, "Delete"},
}

// SetAuditTarget records the name and UID assigned by Kubernetes on creation.
func SetAuditTarget(c *gin.Context, object metav1.Object) {
	c.Set(auditTargetKey, corev1.ObjectReference{Name: object.GetName(), UID: object.GetUID()})
}

// Audit emits one Event per authenticated write request, including authorization denials.
func Audit(events eventsclient.EventsGetter) gin.HandlerFunc {
	instance, err := os.Hostname()
	if err != nil {
		instance = "kubetask"
	}
	logger := log.Log.WithName("api-audit")
	return func(c *gin.Context) {
		operation, tracked := auditOperations[c.Request.Method+" "+c.FullPath()]
		if !tracked {
			c.Next()
			return
		}
		c.Next()

		user, ok := AuthenticatedUser(c)
		if !ok {
			return
		}
		namespace := tenant.ServiceAccountNamespace(user.Username)
		account := strings.TrimPrefix(user.Username, "system:serviceaccount:"+namespace+":")
		status := c.Writer.Status()
		eventType, reason := corev1.EventTypeNormal, "APIRequestSucceeded"
		if status >= http.StatusBadRequest {
			eventType, reason = corev1.EventTypeWarning, "APIRequestFailed"
		}
		if status == http.StatusForbidden {
			reason = "APIRequestDenied"
		}
		event := &eventsv1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace, GenerateName: "kubetask-audit-",
			},
			EventTime:           metav1.MicroTime{Time: time.Now()},
			ReportingController: "kubetask.io/api-audit",
			ReportingInstance:   instance,
			Action:              operation.action,
			Reason:              reason,
			Regarding: corev1.ObjectReference{
				APIVersion: "v1", Kind: "ServiceAccount", Namespace: namespace, Name: account, UID: types.UID(user.UID),
			},
			Type: eventType,
			Note: fmt.Sprintf("%s request returned HTTP %d", operation.kind, status),
		}
		if ids := user.Extra[auth.APIKeyIDExtra]; len(ids) != 0 {
			event.Annotations = map[string]string{auth.APIKeyIDExtra: ids[0]}
		}
		value, _ := c.Get(auditTargetKey)
		target, ok := value.(corev1.ObjectReference)
		if !ok {
			target.Name = c.Param("name")
		}
		if target.Name != "" {
			target.APIVersion = kubetaskv1.SchemeGroupVersion.String()
			target.Kind = operation.kind
			event.Related = &target
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), time.Second)
		defer cancel()
		if _, err := events.Events(namespace).Create(ctx, event, metav1.CreateOptions{}); err != nil {
			logger.Info("Failed to create audit event", "reason", k8serrors.ReasonForError(err))
		}
	}
}
