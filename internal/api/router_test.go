package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/api/middleware"
	"kubetask.io/kubetask/internal/auth"
)

const (
	testAuthTaskName      = "auth-task"
	testAuthTaskJSON      = `"name":"` + testAuthTaskName + `"`
	testBearerSecretToken = "Bearer secret-token"
)

type routerAuthenticatorFunc func(context.Context, string) (*authenticationv1.UserInfo, error)

func (f routerAuthenticatorFunc) Authenticate(ctx context.Context, token string) (*authenticationv1.UserInfo, error) {
	return f(ctx, token)
}

type routerAuthorizerFunc func(context.Context, *authenticationv1.UserInfo, authorizationv1.ResourceAttributes) error

func (f routerAuthorizerFunc) Authorize(ctx context.Context, user *authenticationv1.UserInfo, attributes authorizationv1.ResourceAttributes) error {
	return f(ctx, user, attributes)
}

func TestRouterAuthorizationProtectsAllAPIRoutes(t *testing.T) {
	for _, route := range NewRouter(nil, nil, "").engine.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1/") {
			continue
		}
		t.Run(route.Method+" "+route.Path, func(t *testing.T) {
			a := routerAuthenticatorFunc(func(context.Context, string) (*authenticationv1.UserInfo, error) {
				return &authenticationv1.UserInfo{Username: "system:serviceaccount:team-a:readonly"}, nil
			})
			calls := 0
			z := routerAuthorizerFunc(func(_ context.Context, user *authenticationv1.UserInfo, attributes authorizationv1.ResourceAttributes) error {
				calls++
				if user.Username == "" || attributes.Group != kubetaskv1.SchemeGroupVersion.Group || attributes.Namespace != "" {
					t.Error("authorization lost the authenticated caller or cluster resource scope")
				}
				return fmt.Errorf("private review detail: %w", auth.ErrForbidden)
			})
			router := NewRouter(nil, nil, "", middleware.Authentication(a), middleware.Authorization(z), func(*gin.Context) {
				t.Error("forbidden request reached the business handler")
			})
			req := httptest.NewRequest(route.Method, strings.ReplaceAll(route.Path, ":name", "example"), nil)
			req.Header.Set("Authorization", testBearerSecretToken)
			w := httptest.NewRecorder()
			router.engine.ServeHTTP(w, req)
			if calls != 1 || w.Code != http.StatusForbidden || w.Body.String() != `{"error":"Forbidden"}` {
				t.Errorf("authorization: %d calls, status %d, body %s; want one call and generic 403", calls, w.Code, w.Body.String())
			}
		})
	}
}

func TestTriggerChecksStatusPermissionBeforeMutation(t *testing.T) {
	k8sClient := routerFixtureClient(t)
	a := routerAuthenticatorFunc(func(context.Context, string) (*authenticationv1.UserInfo, error) {
		return &authenticationv1.UserInfo{Username: "system:serviceaccount:team-a:editor"}, nil
	})
	checks := 0
	z := routerAuthorizerFunc(func(_ context.Context, _ *authenticationv1.UserInfo, attributes authorizationv1.ResourceAttributes) error {
		checks++
		if attributes.Name != testAuthTaskName {
			t.Error("authorization lost the target resource name")
		}
		if attributes.Subresource == "status" {
			return auth.ErrForbidden
		}
		return nil
	})
	router := NewRouter(k8sClient, nil, "", middleware.Authentication(a), middleware.Authorization(z))
	req := httptest.NewRequest(http.MethodPost, tasksPath+"/"+testAuthTaskName+"/trigger", nil)
	req.Header.Set("Authorization", testBearerSecretToken)
	w := httptest.NewRecorder()
	router.engine.ServeHTTP(w, req)
	task := &kubetaskv1.Task{}
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: testAuthTaskName}, task); err != nil {
		t.Fatal(err)
	}
	if checks != 3 || w.Code != http.StatusForbidden || task.Annotations["kubetask.io/last-trigger"] != "" {
		t.Fatal("trigger must check all permissions before changing Task metadata or status")
	}
}

func TestRouterAuthenticationProtectsAllAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	routes := NewRouter(nil, nil, "").engine.Routes()
	checked := 0
	for _, route := range routes {
		if !strings.HasPrefix(route.Path, "/api/v1/") {
			continue
		}
		checked++
		for _, tt := range []struct {
			name                 string
			header               string
			err                  error
			missingAuthenticator bool
			status               int
			calls                int
		}{
			{name: "missing credentials", status: http.StatusUnauthorized},
			{name: "invalid credentials", header: testBearerSecretToken, err: fmt.Errorf("private secret-token: %w", auth.ErrUnauthenticated), status: http.StatusUnauthorized, calls: 1},
			{name: "authentication unavailable", header: testBearerSecretToken, err: fmt.Errorf("private secret-token: %w", auth.ErrAuthenticationUnavailable), status: http.StatusServiceUnavailable, calls: 1},
			{name: "missing authenticator", header: testBearerSecretToken, missingAuthenticator: true, status: http.StatusServiceUnavailable},
		} {
			t.Run(route.Method+" "+route.Path+"/"+tt.name, func(t *testing.T) {
				calls := 0
				var authenticator middleware.Authenticator = routerAuthenticatorFunc(func(_ context.Context, token string) (*authenticationv1.UserInfo, error) {
					calls++
					if token != "secret-token" {
						t.Error("router did not pass the original Bearer token")
					}
					return nil, tt.err
				})
				if tt.missingAuthenticator {
					authenticator = nil
				}
				// 放在认证之后的探针覆盖真实路由链，拒绝后不能到达它或业务 handler。
				router := NewRouter(nil, nil, "", middleware.Authentication(authenticator), func(*gin.Context) {
					t.Error("rejected request reached the business handler chain")
				})
				path := strings.ReplaceAll(route.Path, ":name", "example")
				req := httptest.NewRequest(route.Method, path, nil)
				if tt.header != "" {
					req.Header.Set("Authorization", tt.header)
				}
				w := httptest.NewRecorder()
				router.engine.ServeHTTP(w, req)
				if w.Code != tt.status || w.Body.String() != fmt.Sprintf(`{"error":%q}`, http.StatusText(tt.status)) {
					t.Errorf("authentication rejection: status %d, want generic %d response", w.Code, tt.status)
				}
				if calls != tt.calls {
					t.Errorf("authentication calls = %d, want %d", calls, tt.calls)
				}
			})
		}
	}
	if checked == 0 {
		t.Fatal("no API routes were checked")
	}
}

func TestRouterAuthenticationAllowsRealHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	k8sClient := routerFixtureClient(t)
	wantUser := &authenticationv1.UserInfo{
		Username: "system:serviceaccount:team-a:ci",
		UID:      "account-uid",
		Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:team-a", "system:authenticated"},
		Extra:    map[string]authenticationv1.ExtraValue{"credential-id": {"trusted-credential"}},
	}
	for _, tt := range []struct {
		path   string
		status int
		body   string
	}{
		{path: tasksPath, status: http.StatusOK, body: testAuthTaskJSON},
		{path: tasksPath + "/" + testAuthTaskName, status: http.StatusOK, body: testAuthTaskJSON},
		{path: "/api/v1/workflows", status: http.StatusOK, body: `"name":"auth-workflow"`},
		{path: "/api/v1/workflows/auth-workflow", status: http.StatusOK, body: `"workflow":`},
		{path: statsPath, status: http.StatusOK, body: testSingleTotal},
		{path: "/api/v1/stats/trend", status: http.StatusOK, body: `[]`},
		// 无关联 Job 时由真实日志 handler 返回此错误，确认该路由也能通过认证。
		{path: tasksPath + "/" + testAuthTaskName + "/logs?follow=true", status: http.StatusNotFound, body: `"error":"no jobs found for task"`},
	} {
		t.Run(tt.path, func(t *testing.T) {
			type requestKey struct{}
			requestCtx := context.WithValue(context.Background(), requestKey{}, "request-value")
			calls, verified := 0, false
			authenticator := routerAuthenticatorFunc(func(ctx context.Context, token string) (*authenticationv1.UserInfo, error) {
				calls++
				if token != "secret-token" || ctx.Value(requestKey{}) != "request-value" {
					t.Error("authentication lost the token or request context")
				}
				return wantUser, nil
			})
			router := NewRouter(k8sClient, nil, "", middleware.Authentication(authenticator), func(c *gin.Context) {
				verified = true
				user, ok := middleware.AuthenticatedUser(c)
				if !ok || !reflect.DeepEqual(user, wantUser) {
					t.Error("trusted identity was not available to later middleware")
				}
				if c.Request.Context() != requestCtx || requestCtx.Err() != nil {
					t.Error("authentication changed or canceled the business request context")
				}
			})
			req := httptest.NewRequest(http.MethodGet, tt.path, nil).WithContext(requestCtx)
			req.Header.Set("Authorization", testBearerSecretToken)
			w := httptest.NewRecorder()
			router.engine.ServeHTTP(w, req)
			if calls != 1 || !verified || w.Code != tt.status || !strings.Contains(w.Body.String(), tt.body) {
				t.Errorf("authenticated request: calls %d, identity verified %v, status %d, want %d and handler response", calls, verified, w.Code, tt.status)
			}
		})
	}
}

func TestRouterAuthenticationLeavesHealthAndWebPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := routerAuthenticatorFunc(func(context.Context, string) (*authenticationv1.UserInfo, error) {
		t.Error("public or unmatched route invoked authentication")
		return nil, auth.ErrAuthenticationUnavailable
	})
	router := NewRouter(nil, nil, "", middleware.Authentication(authenticator))
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "index.html"), "<!doctype html><title>Public KubeTask</title>")
	mustWrite(t, filepath.Join(root, "assets", "app.js"), "console.log('public')")
	if err := router.ServeWeb(root); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		path   string
		status int
		body   string
	}{
		{path: "/healthz", status: http.StatusOK, body: `{"status":"ok"}`},
		{path: "/", status: http.StatusOK, body: "Public KubeTask"},
		{path: "/assets/app.js", status: http.StatusOK, body: "console.log('public')"},
		{path: "/api/v1/unknown", status: http.StatusNotFound, body: "404 page not found"},
		{path: "/api/unknown", status: http.StatusNotFound, body: "404 page not found"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			// 即使附带凭据，组外请求也不应调用认证服务。
			req.Header.Set("Authorization", testBearerSecretToken)
			w := httptest.NewRecorder()
			router.engine.ServeHTTP(w, req)
			if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.body) {
				t.Errorf("public route: status %d, want %d and original response", w.Code, tt.status)
			}
		})
	}
}

func TestRouterWithoutMiddlewareRetainsAnonymousAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(routerFixtureClient(t), nil, "")
	w := httptest.NewRecorder()
	router.engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tasksPath, nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), testAuthTaskJSON) {
		t.Errorf("router without middleware: status %d, want anonymous 200 response", w.Code)
	}
}

func routerFixtureClient(t *testing.T) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := kubetaskv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&kubetaskv1.Task{
			Name: testAuthTaskName,
			Spec: kubetaskv1.TaskSpec{Type: kubetaskv1.TaskTypeOneTime, Image: "busybox"},
		},
		&kubetaskv1.Workflow{Name: "auth-workflow"},
	).Build()
}
