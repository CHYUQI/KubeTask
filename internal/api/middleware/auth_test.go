package middleware_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	authenticationv1 "k8s.io/api/authentication/v1"

	"kubetask.io/kubetask/internal/api/middleware"
	"kubetask.io/kubetask/internal/auth"
)

type authenticateFunc func(context.Context, string) (*authenticationv1.UserInfo, error)

func (f authenticateFunc) Authenticate(ctx context.Context, token string) (*authenticationv1.UserInfo, error) {
	return f(ctx, token)
}

// 编译时确认现有校验器满足中间件的最小接口。
var _ middleware.Authenticator = (*auth.ServiceAccountAuthenticator)(nil)

func TestAuthenticationHeaders(t *testing.T) {
	for _, tt := range []struct {
		name    string
		headers []string
		target  string
		token   string
	}{
		{name: "missing"},
		{name: "empty", headers: []string{""}},
		{name: "blank", headers: []string{" "}},
		{name: "wrong scheme", headers: []string{"Basic opaque-token"}},
		{name: "no separator", headers: []string{"Beareropaque-token"}},
		{name: "no token", headers: []string{"Bearer"}},
		{name: "empty token", headers: []string{"Bearer   "}},
		{name: "leading whitespace", headers: []string{" Bearer opaque-token"}},
		{name: "trailing whitespace", headers: []string{"Bearer opaque-token "}},
		{name: "tab separator", headers: []string{"Bearer\topaque-token"}},
		{name: "tab in token", headers: []string{"Bearer opaque\ttoken"}},
		{name: "newline in token", headers: []string{"Bearer opaque\ntoken"}},
		{name: "multiple tokens", headers: []string{"Bearer opaque-token extra"}},
		{name: "combined headers", headers: []string{"Bearer opaque-token, Bearer another-token"}},
		{name: "duplicate headers", headers: []string{"Bearer opaque-token", "Bearer another-token"}},
		{name: "duplicate empty header", headers: []string{"Bearer opaque-token", ""}},
		{name: "unicode whitespace", headers: []string{"Bearer opaque\u00a0token"}},
		{name: "invalid character", headers: []string{"Bearer token:secret"}},
		{name: "misplaced padding", headers: []string{"Bearer token=secret"}},
		{name: "padding without token", headers: []string{"Bearer ==="}},
		{name: "query tokens ignored", target: "/protected?token=query-secret&access_token=query-secret"},
		{name: "valid", headers: []string{"Bearer opaque-token"}, token: "opaque-token"},
		{name: "case insensitive scheme", headers: []string{"bEaReR opaque-token"}, token: "opaque-token"},
		{name: "multiple separator spaces", headers: []string{"Bearer   opaque-token"}, token: "opaque-token"},
		{name: "all allowed characters", headers: []string{"Bearer AZaz09-._~+/=="}, token: "AZaz09-._~+/=="},
		{name: "header wins over query", headers: []string{"Bearer opaque-token"}, target: "/protected?access_token=query-secret", token: "opaque-token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls, handled := 0, false
			a := authenticateFunc(func(_ context.Context, token string) (*authenticationv1.UserInfo, error) {
				calls++
				if token != tt.token {
					t.Error("authenticator did not receive the original header token")
				}
				return &authenticationv1.UserInfo{Username: "system:serviceaccount:team-a:ci"}, nil
			})
			router := authRouter(a, func(c *gin.Context) {
				handled = true
				c.Status(http.StatusNoContent)
			})
			target := tt.target
			if target == "" {
				target = "/protected"
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			for _, header := range tt.headers {
				req.Header.Add("Authorization", header)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if tt.token == "" {
				assertAuthRejection(t, w, http.StatusUnauthorized)
				if calls != 0 || handled {
					t.Error("invalid credentials reached authenticator or business handler")
				}
			} else if w.Code != http.StatusNoContent || calls != 1 || !handled {
				t.Errorf("valid credentials: status %d, calls %d, handled %v", w.Code, calls, handled)
			}
		})
	}
}

func TestAuthenticationErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "unauthenticated", err: auth.ErrUnauthenticated, status: http.StatusUnauthorized},
		{name: "wrapped unauthenticated", err: fmt.Errorf("expired secret-token: %w", auth.ErrUnauthenticated), status: http.StatusUnauthorized},
		{name: "unavailable", err: auth.ErrAuthenticationUnavailable, status: http.StatusServiceUnavailable},
		{name: "wrapped unavailable", err: fmt.Errorf("cannot review secret-token: %w", auth.ErrAuthenticationUnavailable), status: http.StatusServiceUnavailable},
		{name: "unexpected", err: errors.New("internal details: secret-token"), status: http.StatusServiceUnavailable},
		{name: "context error", err: context.DeadlineExceeded, status: http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handled := false
			var authCtx context.Context
			a := authenticateFunc(func(ctx context.Context, _ string) (*authenticationv1.UserInfo, error) {
				authCtx = ctx
				// 即使错误同时带有身份，也不能继续执行业务。
				return &authenticationv1.UserInfo{Username: "system:serviceaccount:team-a:ci"}, tt.err
			})
			router := authRouter(a, func(*gin.Context) { handled = true })
			req := bearerRequest(context.Background())
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assertAuthRejection(t, w, tt.status)
			if handled {
				t.Error("rejected authentication executed business handler")
			}
			if authCtx == nil || !errors.Is(authCtx.Err(), context.Canceled) {
				t.Error("authentication context was not released on error")
			}
		})
	}
}

func TestAuthenticationMissingDependencyOrIdentity(t *testing.T) {
	for _, tt := range []struct {
		name string
		a    middleware.Authenticator
	}{
		{name: "missing authenticator"},
		{name: "nil identity", a: authenticateFunc(func(context.Context, string) (*authenticationv1.UserInfo, error) {
			return nil, nil
		})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := authRouter(tt.a, func(*gin.Context) { t.Error("missing authentication result executed handler") })
			w := httptest.NewRecorder()
			router.ServeHTTP(w, bearerRequest(context.Background()))
			assertAuthRejection(t, w, http.StatusServiceUnavailable)
		})
	}
}

func TestAuthenticationIdentityAndRequestContext(t *testing.T) {
	want := authenticationv1.UserInfo{
		Username: "system:serviceaccount:team-a:ci",
		UID:      "account-uid",
		Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:team-a", "system:authenticated"},
		Extra:    map[string]authenticationv1.ExtraValue{"authentication.kubernetes.io/credential-id": {"credential-id"}},
	}
	source := want.DeepCopy()
	type contextKey struct{}
	requestCtx := context.WithValue(context.Background(), contextKey{}, "request-value")
	var authCtx context.Context
	a := authenticateFunc(func(ctx context.Context, _ string) (*authenticationv1.UserInfo, error) {
		authCtx = ctx
		if ctx.Value(contextKey{}) != "request-value" {
			t.Error("authentication context lost request values")
		}
		return source, nil
	})
	handled := false
	router := authRouter(a, func(c *gin.Context) {
		handled = true
		if c.Request.Context() != requestCtx || c.Request.Context().Err() != nil {
			t.Error("authentication replaced or canceled business request context")
		}
		if !errors.Is(authCtx.Err(), context.Canceled) {
			t.Error("authentication context must be released before business handler")
		}
		user, ok := middleware.AuthenticatedUser(c)
		if !ok || !reflect.DeepEqual(user, &want) {
			t.Fatal("handler did not receive full trusted UserInfo")
		}
		// 修改校验器原始结果与读取结果，均不应改变 context 中的身份快照。
		source.Username = "changed source"
		source.Groups[0] = "changed source group"
		source.Extra["authentication.kubernetes.io/credential-id"][0] = "changed source extra"
		user.UID = "changed uid"
		user.Groups[1] = "changed reader group"
		user.Extra["authentication.kubernetes.io/credential-id"][0] = "changed reader extra"
		again, ok := middleware.AuthenticatedUser(c)
		if !ok || !reflect.DeepEqual(again, &want) {
			t.Error("trusted identity was changed through a shared pointer, slice or map")
		}
		c.Status(http.StatusNoContent)
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, bearerRequest(requestCtx))
	if w.Code != http.StatusNoContent || !handled {
		t.Errorf("successful authentication: status %d, handled %v", w.Code, handled)
	}
}

func TestAuthenticatedUserMissing(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if user, ok := middleware.AuthenticatedUser(c); user != nil || ok {
		t.Error("context without authentication returned an identity")
	}
}

func TestAuthenticationTimeout(t *testing.T) {
	a := authenticateFunc(func(ctx context.Context, _ string) (*authenticationv1.UserInfo, error) {
		deadline, ok := ctx.Deadline()
		if remaining := time.Until(deadline); !ok || remaining <= 0 || remaining > 3*time.Second {
			t.Error("authentication must have a short deadline of at most three seconds")
			return nil, errors.New("missing short deadline")
		}
		<-ctx.Done()
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Error("authentication was not canceled by its deadline")
		}
		return nil, fmt.Errorf("review secret-token: %w", ctx.Err())
	})
	requestCtx := context.Background()
	router := authRouter(a, func(*gin.Context) { t.Error("timed out authentication executed handler") })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, bearerRequest(requestCtx))
	assertAuthRejection(t, w, http.StatusServiceUnavailable)
	if requestCtx.Err() != nil {
		t.Error("authentication timeout canceled parent request")
	}
}

func TestAuthenticationRequestCancellation(t *testing.T) {
	for _, tt := range []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		want error
	}{
		{name: "request canceled", ctx: func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, want: context.Canceled},
		{name: "earlier request deadline", ctx: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), 20*time.Millisecond)
		}, want: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.ctx()
			defer cancel()
			a := authenticateFunc(func(authCtx context.Context, _ string) (*authenticationv1.UserInfo, error) {
				<-authCtx.Done()
				if !errors.Is(authCtx.Err(), tt.want) {
					t.Error("authentication did not inherit request cancellation")
				}
				// 已取消的认证不能因为校验器返回成功而放行业务。
				return &authenticationv1.UserInfo{Username: "system:serviceaccount:team-a:ci"}, nil
			})
			router := authRouter(a, func(*gin.Context) { t.Error("canceled authentication executed handler") })
			w := httptest.NewRecorder()
			router.ServeHTTP(w, bearerRequest(ctx))
			assertAuthRejection(t, w, http.StatusServiceUnavailable)
		})
	}
}

func authRouter(a middleware.Authenticator, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.Authentication(a))
	router.GET("/protected", handler)
	return router
}

func bearerRequest(ctx context.Context) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer secret-token")
	return req
}

func assertAuthRejection(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Errorf("status = %d, want %d", w.Code, status)
	}
	wantBody := fmt.Sprintf(`{"error":%q}`, http.StatusText(status))
	if w.Body.String() != wantBody {
		t.Error("authentication response must contain only a generic error")
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Error("authentication error must be JSON")
	}
	challenge := w.Header().Get("WWW-Authenticate")
	if (status == http.StatusUnauthorized && challenge != "Bearer") || (status != http.StatusUnauthorized && challenge != "") {
		t.Error("unexpected Bearer authentication challenge")
	}
	response := w.Body.String() + fmt.Sprint(w.Header())
	for _, secret := range []string{"secret-token", "opaque-token", "query-secret", "internal details", "cannot review", "expired"} {
		if strings.Contains(response, secret) {
			t.Error("authentication response leaked credentials or underlying errors")
		}
	}
}
