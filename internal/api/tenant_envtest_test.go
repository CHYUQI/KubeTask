package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
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
			Verbs:     []string{testGetVerb, testListVerb, "create", "update", "delete"},
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
		{http.MethodGet, tasksPath, "", testSingleTotal, http.StatusOK},
		{http.MethodGet, statsPath, "", testSingleTotal, http.StatusOK},
		{http.MethodDelete, tasksPath + "/" + foreignTask.Name, "", "", http.StatusNotFound},
		{http.MethodGet, tasksPath + "/" + foreignTask.Name + "/logs", "", "", http.StatusNotFound},
		{http.MethodGet, "/api/v1/workflows/" + foreignWorkflow.Name, "", "", http.StatusNotFound},
	} {
		w := request(ctx, tt.method, tt.path, tt.body)
		if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.contains) {
			t.Errorf("%s %s: status %d, body %s; want %d and %q", tt.method, tt.path, w.Code, w.Body.String(), tt.status, tt.contains)
		}
	}
	accountObject, err := admin.CoreV1().ServiceAccounts(namespace).Get(ctx, account, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	key := "ktk_" + strings.Repeat("a", 64)
	keysPath := filepath.Join(t.TempDir(), "keys.json")
	bindings := fmt.Appendf(nil, `[{"id":"ci","sha256":"%x","namespace":%q,"serviceAccount":%q,"uid":%q}]`,
		sha256.Sum256([]byte(key)), namespace, account, accountObject.UID)
	if err := os.WriteFile(keysPath, bindings, 0o600); err != nil {
		t.Fatal(err)
	}
	keys := auth.NewAPIKeyAuthenticator(authenticator, admin.CoreV1(), keysPath)
	router = NewRouter(k8sClient, admin, "", middleware.Authentication(keys), middleware.TenantIsolation(),
		middleware.Audit(admin.EventsV1()), middleware.Authorization(authorizer))
	token = key
	if w := request(ctx, http.MethodGet, tasksPath, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), ownerLabel) {
		t.Errorf("API Key did not receive its ServiceAccount tenant access: %d %s", w.Code, w.Body.String())
	}
	if w := request(ctx, http.MethodGet, tasksPath+"/"+foreignTask.Name, ""); w.Code != http.StatusNotFound {
		t.Errorf("API Key must not see another tenant's Task, got %d", w.Code)
	}
	createdResponse := request(ctx, http.MethodPost, tasksPath, `{"spec":{"type":"OneTime","image":"busybox"}}`)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("API Key create: status %d, want 201", createdResponse.Code)
	}
	var created kubetaskv1.Task
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if w := request(ctx, http.MethodPost, tasksPath+"/"+created.Name+"/trigger", ""); w.Code != http.StatusForbidden {
		t.Fatalf("trigger without status update permission: status %d, want 403", w.Code)
	}
	events, err := admin.EventsV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Items) != 2 {
		t.Fatalf("want an Event for each allowed or denied write, got %d", len(events.Items))
	}
	want := map[string]string{"Create": "APIRequestSucceeded", "Trigger": "APIRequestDenied"}
	for _, event := range events.Items {
		assertTenantAuditEvent(t, event, &created, accountObject, key, want[event.Action])
		delete(want, event.Action)
	}
}

func assertTenantAuditEvent(t *testing.T, event eventsv1.Event, task *kubetaskv1.Task, account *corev1.ServiceAccount, key, reason string) {
	t.Helper()
	if event.Reason != reason || event.Related == nil || event.Related.Name != task.Name ||
		event.Regarding.Name != account.Name || event.Regarding.Namespace != account.Namespace || event.Regarding.UID != account.UID ||
		event.Annotations[auth.APIKeyIDExtra] != "ci" || strings.Contains(event.Note, key) {
		t.Errorf("audit Event for %s did not identify the expected actor, target or outcome", event.Action)
	}
	if event.Action == "Create" && event.Related != nil && event.Related.UID != task.UID {
		t.Error("create audit Event must contain the API-server-assigned target UID")
	}
}
