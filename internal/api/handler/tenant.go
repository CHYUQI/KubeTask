package handler

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/tenant"
)

const (
	taskResource     = "tasks"
	workflowResource = "workflows"
)

func prepareTenantCreate(ctx context.Context, obj client.Object) {
	namespace := tenant.Namespace(ctx)
	tenant.SetNamespace(obj, namespace)
	if namespace != "" {
		obj.SetOwnerReferences(nil)
	}
}

func tenantListOptions(ctx context.Context) []client.ListOption {
	if namespace := tenant.Namespace(ctx); namespace != "" {
		return []client.ListOption{client.MatchingLabels{tenant.Label: namespace}}
	}
	return nil
}

func getTenantObject(ctx context.Context, reader client.Reader, resource, name string, obj client.Object) error {
	if err := reader.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
		return err
	}
	if namespace := tenant.Namespace(ctx); namespace != "" && obj.GetLabels()[tenant.Label] != namespace {
		return errors.NewNotFound(kubetaskv1.GroupVersion.WithResource(resource).GroupResource(), name)
	}
	return nil
}

func tenantDeleteOptions(ctx context.Context, obj client.Object) []client.DeleteOption {
	if tenant.Namespace(ctx) != "" {
		return []client.DeleteOption{client.Preconditions{UID: new(obj.GetUID())}}
	}
	return nil
}
