package tenant

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Label is the service-managed tenant Namespace of a Task or Workflow.
const Label = "kubetask.io/tenant"

type namespaceKey struct{}

// WithNamespace scopes a request to its authenticated tenant.
func WithNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, namespaceKey{}, namespace)
}

// Namespace returns the request's tenant, or empty when isolation is disabled.
func Namespace(ctx context.Context) string {
	namespace, _ := ctx.Value(namespaceKey{}).(string)
	return namespace
}

// ServiceAccountNamespace extracts the Namespace from a trusted Kubernetes identity.
func ServiceAccountNamespace(username string) string {
	account, ok := strings.CutPrefix(username, "system:serviceaccount:")
	if !ok {
		return ""
	}
	namespace, name, ok := strings.Cut(account, ":")
	if !ok || name == "" || strings.Contains(name, ":") {
		return ""
	}
	return namespace
}

// SetNamespace sets the reserved label without replacing unrelated labels.
func SetNamespace(obj metav1.Object, namespace string) {
	labels := obj.GetLabels()
	if namespace == "" {
		delete(labels, Label)
	} else {
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[Label] = namespace
	}
	obj.SetLabels(labels)
}
