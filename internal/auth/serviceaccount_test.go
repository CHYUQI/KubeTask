package auth_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authenticationclient "k8s.io/client-go/kubernetes/typed/authentication/v1"

	"kubetask.io/kubetask/internal/auth"
)

const (
	testAudience       = "kubetask"
	testServiceAccount = "system:serviceaccount:team-a:ci"
)

// TokenReviewInterface 仅暴露 Create；替身可检查请求并模拟 API Server 的校验结果。
type tokenReviewFunc func(context.Context, *authenticationv1.TokenReview, metav1.CreateOptions) (*authenticationv1.TokenReview, error)

func (f tokenReviewFunc) Create(ctx context.Context, review *authenticationv1.TokenReview, opts metav1.CreateOptions) (*authenticationv1.TokenReview, error) {
	return f(ctx, review, opts)
}

func TestNewServiceAccountAuthenticator(t *testing.T) {
	reviews := tokenReviewFunc(func(context.Context, *authenticationv1.TokenReview, metav1.CreateOptions) (*authenticationv1.TokenReview, error) {
		t.Fatal("constructor must not call Kubernetes")
		return nil, nil
	})
	for _, tt := range []struct {
		name     string
		reviews  authenticationclient.TokenReviewInterface
		audience string
		wantErr  bool
	}{
		{name: "valid", reviews: reviews, audience: testAudience},
		{name: "missing client", audience: testAudience, wantErr: true},
		{name: "empty audience", reviews: reviews, wantErr: true},
		{name: "blank audience", reviews: reviews, audience: " \t", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, err := auth.NewServiceAccountAuthenticator(tt.reviews, tt.audience)
			if (err != nil) != tt.wantErr || (a == nil) != tt.wantErr {
				t.Fatalf("constructor = (%v, %v), want error %v", a, err, tt.wantErr)
			}
		})
	}
}

func TestServiceAccountAuthenticatorRequestAndIdentity(t *testing.T) {
	wantUser := authenticationv1.UserInfo{
		Username: testServiceAccount,
		UID:      "account-uid",
		Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:team-a", "system:authenticated"},
		Extra:    map[string]authenticationv1.ExtraValue{"authentication.kubernetes.io/credential-id": {"credential-id"}},
	}
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-context")
	calls := 0
	reviews := tokenReviewFunc(func(gotCtx context.Context, review *authenticationv1.TokenReview, _ metav1.CreateOptions) (*authenticationv1.TokenReview, error) {
		calls++
		if gotCtx.Value(contextKey{}) != "request-context" {
			t.Error("request context was not passed to Kubernetes")
		}
		if review.Spec.Token != "opaque-token" || !reflect.DeepEqual(review.Spec.Audiences, []string{testAudience}) {
			t.Error("TokenReview must carry the original token and explicit audience")
		}
		return &authenticationv1.TokenReview{Status: authenticationv1.TokenReviewStatus{
			Authenticated: true,
			Audiences:     []string{"other", testAudience},
			User:          wantUser,
		}}, nil
	})
	a := newAuthenticator(t, reviews)
	user, err := a.Authenticate(ctx, "opaque-token")
	if err != nil || user == nil || !reflect.DeepEqual(*user, wantUser) {
		t.Fatalf("identity = (%v, %v), want full Kubernetes UserInfo", user, err)
	}
	if calls != 1 {
		t.Fatalf("TokenReview calls = %d, want 1", calls)
	}
}

func TestServiceAccountAuthenticatorRejectsInvalidIdentity(t *testing.T) {
	for _, tt := range []struct {
		name          string
		authenticated bool
		audiences     []string
		username      string
		reviewError   string
	}{
		{name: "unauthenticated", audiences: []string{testAudience}, username: testServiceAccount},
		{name: "expired token", reviewError: "service account token has expired: opaque-token"},
		{name: "authenticated with error", authenticated: true, audiences: []string{testAudience}, username: testServiceAccount, reviewError: "cannot inspect opaque-token"},
		{name: "missing audience", authenticated: true, username: testServiceAccount},
		{name: "wrong audience", authenticated: true, audiences: []string{"kubernetes"}, username: testServiceAccount},
		{name: "normal user", authenticated: true, audiences: []string{testAudience}, username: "alice"},
		{name: "anonymous user", authenticated: true, audiences: []string{testAudience}, username: "system:anonymous"},
		{name: "empty username", authenticated: true, audiences: []string{testAudience}},
		{name: "empty namespace", authenticated: true, audiences: []string{testAudience}, username: "system:serviceaccount::ci"},
		{name: "empty account name", authenticated: true, audiences: []string{testAudience}, username: "system:serviceaccount:team-a:"},
		{name: "extra username segment", authenticated: true, audiences: []string{testAudience}, username: "system:serviceaccount:team-a:ci:extra"},
		{name: "wrong identity prefix", authenticated: true, audiences: []string{testAudience}, username: "other:serviceaccount:team-a:ci"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reviews := tokenReviewFunc(func(context.Context, *authenticationv1.TokenReview, metav1.CreateOptions) (*authenticationv1.TokenReview, error) {
				return &authenticationv1.TokenReview{Status: authenticationv1.TokenReviewStatus{
					Authenticated: tt.authenticated,
					Audiences:     tt.audiences,
					User:          authenticationv1.UserInfo{Username: tt.username},
					Error:         tt.reviewError,
				}}, nil
			})
			user, err := newAuthenticator(t, reviews).Authenticate(context.Background(), "opaque-token")
			if user != nil || !errors.Is(err, auth.ErrUnauthenticated) {
				t.Fatalf("Authenticate = (%v, %v), want nil identity and ErrUnauthenticated", user, err)
			}
			if strings.Contains(err.Error(), "opaque-token") {
				t.Error("authentication error leaked token review details")
			}
		})
	}
}

func TestServiceAccountAuthenticatorRejectsEmptyToken(t *testing.T) {
	reviews := tokenReviewFunc(func(context.Context, *authenticationv1.TokenReview, metav1.CreateOptions) (*authenticationv1.TokenReview, error) {
		t.Fatal("empty token must not reach Kubernetes")
		return nil, nil
	})
	a := newAuthenticator(t, reviews)
	for _, token := range []string{"", " \t\n"} {
		user, err := a.Authenticate(context.Background(), token)
		if user != nil || !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("empty token = (%v, %v), want ErrUnauthenticated", user, err)
		}
	}
}

func TestServiceAccountAuthenticatorReviewFailure(t *testing.T) {
	apiErr := errors.New("API server unavailable")
	for _, tt := range []struct {
		name   string
		review *authenticationv1.TokenReview
		err    error
	}{
		{name: "API failure", err: apiErr},
		{name: "nil response"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reviews := tokenReviewFunc(func(context.Context, *authenticationv1.TokenReview, metav1.CreateOptions) (*authenticationv1.TokenReview, error) {
				return tt.review, tt.err
			})
			user, err := newAuthenticator(t, reviews).Authenticate(context.Background(), "secret-token")
			if user != nil || !errors.Is(err, auth.ErrAuthenticationUnavailable) || errors.Is(err, auth.ErrUnauthenticated) {
				t.Fatalf("Authenticate = (%v, %v), want ErrAuthenticationUnavailable", user, err)
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Error("underlying API error was not preserved")
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Error("authentication error leaked token review details")
			}
		})
	}
}

func TestServiceAccountAuthenticatorHonorsContext(t *testing.T) {
	for _, tt := range []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		want error
	}{
		{name: "cancellation", ctx: func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, want: context.Canceled},
		{name: "deadline", ctx: func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}, want: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.ctx()
			defer cancel()
			reviews := tokenReviewFunc(func(ctx context.Context, _ *authenticationv1.TokenReview, _ metav1.CreateOptions) (*authenticationv1.TokenReview, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			})
			user, err := newAuthenticator(t, reviews).Authenticate(ctx, "opaque-token")
			if user != nil || !errors.Is(err, auth.ErrAuthenticationUnavailable) || !errors.Is(err, tt.want) {
				t.Fatalf("Authenticate = (%v, %v), want preserved context error %v", user, err, tt.want)
			}
		})
	}
}

func newAuthenticator(t *testing.T, reviews authenticationclient.TokenReviewInterface) *auth.ServiceAccountAuthenticator {
	t.Helper()
	a, err := auth.NewServiceAccountAuthenticator(reviews, testAudience)
	if err != nil {
		t.Fatalf("create authenticator: %v", err)
	}
	return a
}
