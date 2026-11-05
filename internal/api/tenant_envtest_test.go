package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/api/middleware"
	"kubetask.io/kubetask/internal/auth"
	"kubetask.io/kubetask/internal/tenant"
)

func testTenantBoundary(
	t *testing.T, ctx context.Context, k8sClient client.Client, admin kubernetes.Interface,
	limitedConfig *rest.Config, namespace, account, audience, token string,
) {
	t.Helper()
	const roleName, foreignNamespace, testImage = "tenant-editor", "another-tenant", "busybox"
	role := &rbacv1.Role{
		Name:      roleName,
		Namespace: namespace,
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{kubetaskv1.GroupVersion.Group},
			Resources: []string{testTaskResource, "workflows"},
			Verbs:     []string{"get", testListVerb, "create", "update", "delete"},
		}},
	}
	if _, err := admin.RbacV1().Roles(namespace).Create(ctx, role, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	binding := &rbacv1.RoleBinding{
		Name:      roleName,
		Namespace: namespace,
		RoleRef:   rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: roleName},
		Subjects:  []rbacv1.Subject{{Kind: "ServiceAccount", Namespace: namespace, Name: account}},
	}
	if _, err := admin.RbacV1().RoleBindings(namespace).Create(ctx, binding, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	foreignTask := &kubetaskv1.Task{
		Name: "tenant-foreign-task",
		Spec: kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: testImage},
	}
	tenant.SetNamespace(foreignTask, foreignNamespace)
	foreignWorkflow := &kubetaskv1.Workflow{
		Name: "tenant-foreign-workflow",
		Spec: kubetaskv1.WorkflowSpec{Tasks: []kubetaskv1.WorkflowTask{{
			Name: "run", TaskSpec: &kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: testImage},
		}}},
	}
	tenant.SetNamespace(foreignWorkflow, foreignNamespace)
	for _, object := range []client.Object{foreignTask, foreignWorkflow} {
		if err := k8sClient.Create(ctx, object); err != nil {
			t.Fatal(err)
		}
	}
	authenticator, err := auth.NewServiceAccountAuthenticator(admin.AuthenticationV1().TokenReviews(), audience)
	if err != nil {
		t.Fatal(err)
	}
	authorizer, err := auth.NewResourceAuthorizer(admin.AuthorizationV1().SubjectAccessReviews())
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(k8sClient, admin, "", middleware.Authentication(authenticator), middleware.TenantIsolation(), middleware.Authorization(authorizer))
	request := func(ctx context.Context, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.engine.ServeHTTP(w, req)
		return w
	}
	const tasksPath = "/api/v1/tasks"
	if err := wait.PollUntilContextTimeout(ctx, 50*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		return request(ctx, http.MethodGet, tasksPath, "").Code == http.StatusOK, nil
	}); err != nil {
		t.Fatal("namespace RoleBinding did not authorize the KubeTask list")
	}
	limited, err := client.New(limitedConfig, client.Options{Scheme: k8sClient.Scheme()})
	if err != nil {
		t.Fatal(err)
	}
	if err := limited.List(ctx, &kubetaskv1.TaskList{}); !k8serrors.IsForbidden(err) {
		t.Fatal("namespace RoleBinding must not grant direct access to cluster-scoped Tasks")
	}
	const ownName = "tenant-own-task"
	body := fmt.Sprintf(`{"metadata":{"name":%q,"labels":{%q:%q}},"spec":{"type":"OneTime","image":"busybox"}}`, ownName, tenant.Label, foreignNamespace)
	ownerLabel := fmt.Sprintf("%q:%q", tenant.Label, namespace)
	for _, tt := range []struct {
		method, path, body, contains string
		status                       int
	}{
		{http.MethodPost, tasksPath, body, ownerLabel, http.StatusCreated},
		{http.MethodPut, tasksPath + "/" + ownName, body, ownerLabel, http.StatusOK},
		{http.MethodGet, tasksPath, "", `"total":1`, http.StatusOK},
		{http.MethodGet, "/api/v1/stats", "", `"total":1`, http.StatusOK},
		{http.MethodDelete, tasksPath + "/" + foreignTask.Name, "", "", http.StatusNotFound},
		{http.MethodGet, tasksPath + "/" + foreignTask.Name + "/logs", "", "", http.StatusNotFound},
		{http.MethodGet, "/api/v1/workflows/" + foreignWorkflow.Name, "", "", http.StatusNotFound},
	} {
		w := request(ctx, tt.method, tt.path, tt.body)
		if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.contains) {
			t.Errorf("%s %s: status %d, body %s; want %d and %q", tt.method, tt.path, w.Code, w.Body.String(), tt.status, tt.contains)
		}
	}
}
