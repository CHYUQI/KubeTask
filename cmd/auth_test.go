package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authenticationclient "k8s.io/client-go/kubernetes/typed/authentication/v1"

	"kubetask.io/kubetask/internal/api/middleware"
	"kubetask.io/kubetask/internal/config"
)

type startupTokenReviewFunc func(
	context.Context, *authenticationv1.TokenReview, metav1.CreateOptions,
) (*authenticationv1.TokenReview, error)

func (f startupTokenReviewFunc) Create(
	ctx context.Context, review *authenticationv1.TokenReview, opts metav1.CreateOptions,
) (*authenticationv1.TokenReview, error) {
	return f(ctx, review, opts)
}

func TestAuthenticationMiddlewareSetup(t *testing.T) {
	reviews := startupTokenReviewFunc(func(
		context.Context, *authenticationv1.TokenReview, metav1.CreateOptions,
	) (*authenticationv1.TokenReview, error) {
		t.Fatal("middleware setup must not call Kubernetes")
		return nil, nil
	})
	for _, tt := range []struct {
		name     string
		cfg      config.Config
		reviews  authenticationclient.TokenReviewInterface
		wantSize int
		wantErr  bool
	}{
		{name: "disabled without client"},
		{name: "disabled without audience", reviews: reviews},
		{
			name: "enabled", cfg: config.Config{AuthEnabled: true, AuthAudience: "kubetask"},
			reviews: reviews, wantSize: 1,
		},
		{
			name: "enabled without client", cfg: config.Config{AuthEnabled: true, AuthAudience: "kubetask"},
			wantErr: true,
		},
		{name: "enabled without audience", cfg: config.Config{AuthEnabled: true}, reviews: reviews, wantErr: true},
		{
			name: "enabled with blank audience", cfg: config.Config{AuthEnabled: true, AuthAudience: " \t"},
			reviews: reviews, wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handlers, err := authenticationMiddleware(&tt.cfg, tt.reviews)
			if (err != nil) != tt.wantErr || len(handlers) != tt.wantSize {
				t.Errorf("authentication setup = (%d handlers, %v), want (%d handlers, error %v)",
					len(handlers), err, tt.wantSize, tt.wantErr)
			}
		})
	}
}

func TestAuthenticationMiddlewareDisabledAllowsAnonymous(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers, err := authenticationMiddleware(&config.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(handlers...)
	engine.GET("/api/v1/tasks", func(c *gin.Context) {
		if user, ok := middleware.AuthenticatedUser(c); ok || user != nil {
			t.Error("anonymous request has an authenticated identity")
		}
		c.Status(http.StatusNoContent)
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("disabled authentication returned %d, want anonymous handler response", w.Code)
	}
}

func TestAuthenticationMiddlewareTokenReview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wantUser := authenticationv1.UserInfo{
		Username: "system:serviceaccount:team-a:ci",
		UID:      "account-uid",
		Groups:   []string{"system:serviceaccounts", "system:authenticated"},
		Extra:    map[string]authenticationv1.ExtraValue{"credential-id": {"trusted-credential"}},
	}
	for _, tt := range []struct {
		name    string
		header  string
		review  authenticationv1.TokenReviewStatus
		err     error
		status  int
		calls   int
		handled bool
	}{
		{name: "missing credentials", status: http.StatusUnauthorized},
		{
			name: "valid token", header: "Bearer opaque-token",
			review: authenticationv1.TokenReviewStatus{
				Authenticated: true, Audiences: []string{"configured-audience"}, User: wantUser,
			},
			status: http.StatusNoContent, calls: 1, handled: true,
		},
		{name: "invalid token", header: "Bearer opaque-token", status: http.StatusUnauthorized, calls: 1},
		{
			name: "wrong audience", header: "Bearer opaque-token",
			review: authenticationv1.TokenReviewStatus{
				Authenticated: true, Audiences: []string{"other-audience"}, User: wantUser,
			},
			status: http.StatusUnauthorized, calls: 1,
		},
		{
			name: "review failure", header: "Bearer opaque-token",
			err: errors.New("private API failure for opaque-token"), status: http.StatusServiceUnavailable, calls: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls, handled := 0, false
			reviews := startupTokenReviewFunc(func(
				_ context.Context, review *authenticationv1.TokenReview, _ metav1.CreateOptions,
			) (*authenticationv1.TokenReview, error) {
				calls++
				if review.Spec.Token != "opaque-token" ||
					!reflect.DeepEqual(review.Spec.Audiences, []string{"configured-audience"}) {
					t.Error("startup wiring lost the original token or configured audience")
				}
				return &authenticationv1.TokenReview{Status: tt.review}, tt.err
			})
			cfg := &config.Config{AuthEnabled: true, AuthAudience: "configured-audience"}
			handlers, err := authenticationMiddleware(cfg, reviews)
			if err != nil {
				t.Fatal(err)
			}
			engine := gin.New()
			engine.Use(handlers...)
			engine.GET("/api/v1/tasks", func(c *gin.Context) {
				handled = true
				user, ok := middleware.AuthenticatedUser(c)
				if !ok || !reflect.DeepEqual(user, &wantUser) {
					t.Error("startup authentication chain did not deliver the full trusted identity")
				}
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != tt.status || calls != tt.calls || handled != tt.handled {
				t.Errorf("authentication chain: status %d, calls %d, handled %v; want %d, %d, %v",
					w.Code, calls, handled, tt.status, tt.calls, tt.handled)
			}
			if !tt.handled {
				wantBody := `{"error":"Unauthorized"}`
				if tt.status == http.StatusServiceUnavailable {
					wantBody = `{"error":"Service Unavailable"}`
				}
				if w.Body.String() != wantBody {
					t.Error("authentication rejection exposed non-generic details")
				}
			}
		})
	}
}
