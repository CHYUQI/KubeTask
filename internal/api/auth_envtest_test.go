package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/api/middleware"
	"kubetask.io/kubetask/internal/auth"
	"kubetask.io/kubetask/internal/testutil"
)

// TestServiceAccountTokenAuthentication uses locally issued, signed ServiceAccount
// tokens and real TokenReview requests. It never uses an existing cluster.
func TestServiceAccountTokenAuthentication(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set")
	}
	gin.SetMode(gin.TestMode)
	useExistingCluster := false
	testutil.KillOrphanedEnvTestProcesses(filepath.Join("..", "..", "bin", "k8s"))
	testEnv := &envtest.Environment{
		UseExistingCluster:    &useExistingCluster,
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start local envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testutil.StopEnvTest(testEnv); err != nil {
			t.Errorf("stop local envtest: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	const namespace, account, audience = "auth-integration", "ci", "kubetask"
	if _, err := admin.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		Name: namespace,
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	sa, err := admin.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{
		Name: account,
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	issueToken := func(audiences []string) string {
		return requestServiceAccountToken(t, ctx, admin, namespace, account, audiences).Status.Token
	}
	validToken := issueToken([]string{audience})
	wrongAudienceToken := issueToken([]string{"other-service"})
	// The reviewer needs a token for the Kubernetes API itself. It is distinct
	// from the incoming token intended for KubeTask.
	limitedConfig := rest.AnonymousClientConfig(cfg)
	limitedConfig.BearerToken = issueToken(nil)
	limited, err := kubernetes.NewForConfig(limitedConfig)
	if err != nil {
		t.Fatal(err)
	}
	_, err = limited.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{Token: validToken, Audiences: []string{audience}},
	}, metav1.CreateOptions{})
	if !k8serrors.IsForbidden(err) {
		t.Fatal("test reviewer must be authenticated but forbidden to create TokenReview")
	}
	scheme := runtime.NewScheme()
	if err := kubetaskv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		token    string
		reviewer kubernetes.Interface
		status   int
	}{
		{name: "valid signed token", token: validToken, reviewer: admin, status: http.StatusOK},
		{name: "signed token with wrong audience", token: wrongAudienceToken, reviewer: admin, status: http.StatusUnauthorized},
		{name: "invalid token", token: "invalid-token", reviewer: admin, status: http.StatusUnauthorized},
		{name: "missing credentials", reviewer: admin, status: http.StatusUnauthorized},
		{name: "reviewer lacks permission", token: validToken, reviewer: limited, status: http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			authenticator, err := auth.NewServiceAccountAuthenticator(tt.reviewer.AuthenticationV1().TokenReviews(), audience)
			if err != nil {
				t.Fatal(err)
			}
			handled := false
			router := NewRouter(k8sClient, admin, "", middleware.Authentication(authenticator), func(c *gin.Context) {
				handled = true
				user, ok := middleware.AuthenticatedUser(c)
				if !ok || user.Username != "system:serviceaccount:"+namespace+":"+account || user.UID != string(sa.UID) || !slices.Contains(user.Groups, "system:serviceaccounts:"+namespace) {
					t.Error("real TokenReview did not deliver the expected ServiceAccount identity")
				}
			})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil).WithContext(ctx)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			w := httptest.NewRecorder()
			router.engine.ServeHTTP(w, req)
			if w.Code != tt.status || handled != (tt.status == http.StatusOK) {
				t.Errorf("real-token request: status %d, handler reached %v; want status %d", w.Code, handled, tt.status)
			}
			switch tt.status {
			case http.StatusOK:
				if !strings.Contains(w.Body.String(), `"total":0`) {
					t.Error("authenticated request did not reach the real Task list handler")
				}
			case http.StatusUnauthorized:
				if w.Body.String() != `{"error":"Unauthorized"}` || w.Header().Get("WWW-Authenticate") != "Bearer" {
					t.Error("invalid credentials must receive a generic Bearer challenge")
				}
			case http.StatusServiceUnavailable:
				if w.Body.String() != `{"error":"Service Unavailable"}` {
					t.Error("TokenReview permission failure must receive a generic 503")
				}
			}
			if tt.token != "" && strings.Contains(w.Body.String(), tt.token) {
				t.Error("HTTP response leaked the token")
			}
		})
	}
	t.Run("business RBAC is not yet enforced by HTTP API", func(t *testing.T) {
		testBusinessRBACBoundary(t, ctx, k8sClient, admin, limitedConfig, audience, validToken)
	})
	t.Run("natural token expiration slow test", func(t *testing.T) {
		testNaturalTokenExpiration(t, admin, k8sClient, namespace, account, audience)
	})
}

func requestServiceAccountToken(
	t *testing.T, ctx context.Context, admin kubernetes.Interface, namespace, account string, audiences []string,
) *authenticationv1.TokenRequest {
	t.Helper()
	expiration := int64(600)
	request, err := admin.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, account, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{Audiences: audiences, ExpirationSeconds: &expiration},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("issue local ServiceAccount token: %v", err)
	}
	if request.Status.Token == "" {
		t.Fatal("TokenRequest returned an empty token")
	}
	return request
}

func testBusinessRBACBoundary(
	t *testing.T, ctx context.Context, k8sClient client.Client, admin kubernetes.Interface,
	limitedConfig *rest.Config, audience, token string,
) {
	a, err := auth.NewServiceAccountAuthenticator(admin.AuthenticationV1().TokenReviews(), audience)
	if err != nil {
		t.Fatal(err)
	}
	user, err := a.Authenticate(ctx, token)
	if err != nil {
		t.Fatal("test caller must authenticate before checking authorization")
	}
	review, err := admin.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   user.Username,
			UID:    user.UID,
			Groups: user.Groups,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Group: kubetaskv1.SchemeGroupVersion.Group, Resource: "tasks", Verb: "list",
			},
		},
	}, metav1.CreateOptions{})
	if err != nil || review.Status.Allowed {
		t.Fatal("test caller must not have Kubernetes permission to list Tasks")
	}
	limitedClient, err := client.New(limitedConfig, client.Options{Scheme: k8sClient.Scheme()})
	if err != nil {
		t.Fatal(err)
	}
	if err := limitedClient.List(ctx, &kubetaskv1.TaskList{}); !k8serrors.IsForbidden(err) {
		t.Fatal("direct Kubernetes Task list must be forbidden")
	}
	router := NewRouter(k8sClient, admin, "", middleware.Authentication(a))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("current authentication-only API returned %d, want documented 200 boundary", w.Code)
	}
}

func testNaturalTokenExpiration(
	t *testing.T, admin kubernetes.Interface, k8sClient client.Client, namespace, account, audience string,
) {
	if os.Getenv("KUBETASK_TEST_TOKEN_EXPIRY") != "true" {
		t.Skip("set KUBETASK_TEST_TOKEN_EXPIRY=true and go test -timeout=15m to wait for real expiration")
	}
	slowCtx, stop := context.WithTimeout(context.Background(), 13*time.Minute)
	defer stop()
	issued := requestServiceAccountToken(t, slowCtx, admin, namespace, account, []string{audience})
	a, err := auth.NewServiceAccountAuthenticator(admin.AuthenticationV1().TokenReviews(), audience)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(k8sClient, admin, "", middleware.Authentication(a))
	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil).WithContext(slowCtx)
		req.Header.Set("Authorization", "Bearer "+issued.Status.Token)
		w := httptest.NewRecorder()
		router.engine.ServeHTTP(w, req)
		return w.Code
	}
	if request() != http.StatusOK {
		t.Fatal("new token must authenticate before its expiration")
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := issued.Status.ExpirationTimestamp.Add(2 * time.Minute)
	for {
		select {
		case <-slowCtx.Done():
			t.Fatal("timed out waiting for natural token expiration")
		case <-ticker.C:
			if time.Now().Before(issued.Status.ExpirationTimestamp.Time) {
				continue
			}
			switch status := request(); status {
			case http.StatusUnauthorized:
				return
			case http.StatusOK:
				if time.Now().After(deadline) {
					t.Fatal("token remained accepted beyond expiration tolerance")
				}
			default:
				t.Fatalf("unexpected HTTP status %d while checking expiration", status)
			}
		}
	}
}
