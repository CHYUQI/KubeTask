package auth_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubetask.io/kubetask/internal/auth"
)

const (
	testTaskResource = "tasks"
	testReviewDetail = "internal-review-detail"
)

type subjectAccessReviewFunc func(context.Context, *authorizationv1.SubjectAccessReview, metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error)

func (f subjectAccessReviewFunc) Create(ctx context.Context, review *authorizationv1.SubjectAccessReview, opts metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
	return f(ctx, review, opts)
}

func TestNewResourceAuthorizer(t *testing.T) {
	if a, err := auth.NewResourceAuthorizer(nil); a != nil || err == nil {
		t.Fatal("missing review client must fail construction")
	}
	reviews := subjectAccessReviewFunc(func(context.Context, *authorizationv1.SubjectAccessReview, metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
		t.Fatal("constructor must not submit a review")
		return nil, nil
	})
	if _, err := auth.NewResourceAuthorizer(reviews); err != nil {
		t.Fatalf("construct authorizer: %v", err)
	}
}

func TestResourceAuthorizerRequest(t *testing.T) {
	user := authenticationv1.UserInfo{
		Username: testServiceAccount,
		UID:      "authorization-account-uid",
		Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:team-a", "system:authenticated"},
		Extra:    map[string]authenticationv1.ExtraValue{"credential": {"first", "second"}},
	}
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "authorization-context")
	for _, attributes := range []authorizationv1.ResourceAttributes{
		testTaskAttributes(),
		{Group: "kubetask.kubetask.io", Version: "v1", Resource: testTaskResource, Subresource: "status", Name: "task-a", Verb: "update"},
		{Namespace: "default", Version: "v1", Resource: "pods", Subresource: "log", Name: "pod-a", Verb: "get"},
	} {
		t.Run(attributes.Resource+"/"+attributes.Subresource, func(t *testing.T) {
			calls := 0
			reviews := subjectAccessReviewFunc(func(gotCtx context.Context, review *authorizationv1.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
				calls++
				if gotCtx.Value(contextKey{}) != "authorization-context" {
					t.Error("request context was not forwarded")
				}
				want := authorizationv1.SubjectAccessReviewSpec{
					User: user.Username, UID: user.UID, Groups: user.Groups,
					Extra:              map[string]authorizationv1.ExtraValue{"credential": {"first", "second"}},
					ResourceAttributes: &attributes,
				}
				if !reflect.DeepEqual(review.Spec, want) {
					t.Errorf("review spec = %#v, want %#v", review.Spec, want)
				}
				return &authorizationv1.SubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true}}, nil
			})
			if err := newAuthorizer(t, reviews).Authorize(ctx, &user, attributes); err != nil {
				t.Fatalf("authorized request: %v", err)
			}
			if calls != 1 {
				t.Fatalf("review calls = %d, want 1", calls)
			}
		})
	}
}

func TestResourceAuthorizerDecisions(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status authorizationv1.SubjectAccessReviewStatus
		want   error
	}{
		{name: "allowed", status: authorizationv1.SubjectAccessReviewStatus{Allowed: true}},
		{name: "allowed despite partial evaluation error", status: authorizationv1.SubjectAccessReviewStatus{Allowed: true, EvaluationError: testReviewDetail}},
		{name: "denied", status: authorizationv1.SubjectAccessReviewStatus{Denied: true, Reason: testReviewDetail}, want: auth.ErrForbidden},
		{name: "no opinion", want: auth.ErrForbidden},
		{name: "evaluation failed", status: authorizationv1.SubjectAccessReviewStatus{EvaluationError: testReviewDetail}, want: auth.ErrAuthorizationUnavailable},
		{name: "denied despite partial evaluation error", status: authorizationv1.SubjectAccessReviewStatus{Denied: true, EvaluationError: testReviewDetail}, want: auth.ErrForbidden},
		{name: "contradictory decision", status: authorizationv1.SubjectAccessReviewStatus{Allowed: true, Denied: true}, want: auth.ErrAuthorizationUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reviews := subjectAccessReviewFunc(func(context.Context, *authorizationv1.SubjectAccessReview, metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
				return &authorizationv1.SubjectAccessReview{Status: tt.status}, nil
			})
			err := newAuthorizer(t, reviews).Authorize(context.Background(), &authenticationv1.UserInfo{Username: testServiceAccount}, testTaskAttributes())
			if !errors.Is(err, tt.want) {
				t.Fatalf("Authorize error = %v, want %v", err, tt.want)
			}
			if err != nil && strings.Contains(err.Error(), testReviewDetail) {
				t.Error("review status details leaked into the authorization error")
			}
		})
	}
}

func TestResourceAuthorizerRejectsIncompleteInput(t *testing.T) {
	reviews := subjectAccessReviewFunc(func(context.Context, *authorizationv1.SubjectAccessReview, metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
		t.Fatal("incomplete input must not reach Kubernetes")
		return nil, nil
	})
	a := newAuthorizer(t, reviews)
	for _, tt := range []struct {
		name       string
		user       *authenticationv1.UserInfo
		attributes authorizationv1.ResourceAttributes
		want       error
	}{
		{name: "missing identity", attributes: testTaskAttributes(), want: auth.ErrUnauthenticated},
		{name: "empty username", user: &authenticationv1.UserInfo{}, attributes: testTaskAttributes(), want: auth.ErrUnauthenticated},
		{name: "blank username", user: &authenticationv1.UserInfo{Username: " \t"}, attributes: testTaskAttributes(), want: auth.ErrUnauthenticated},
		{name: "missing resource", user: &authenticationv1.UserInfo{Username: testServiceAccount}, attributes: authorizationv1.ResourceAttributes{Verb: "list"}, want: auth.ErrAuthorizationUnavailable},
		{name: "missing verb", user: &authenticationv1.UserInfo{Username: testServiceAccount}, attributes: authorizationv1.ResourceAttributes{Resource: testTaskResource}, want: auth.ErrAuthorizationUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.Authorize(context.Background(), tt.user, tt.attributes); !errors.Is(err, tt.want) {
				t.Fatalf("Authorize error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestResourceAuthorizerReviewFailure(t *testing.T) {
	apiErr := errors.New("subject access review request failed")
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "API failure", err: apiErr},
		{name: "empty response"},
		{name: "canceled", err: context.Canceled},
		{name: "deadline exceeded", err: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reviews := subjectAccessReviewFunc(func(context.Context, *authorizationv1.SubjectAccessReview, metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
				return nil, tt.err
			})
			err := newAuthorizer(t, reviews).Authorize(context.Background(), &authenticationv1.UserInfo{Username: testServiceAccount}, testTaskAttributes())
			if !errors.Is(err, auth.ErrAuthorizationUnavailable) || errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("Authorize error = %v, want ErrAuthorizationUnavailable", err)
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Error("underlying API error was not preserved")
			}
		})
	}
}

func TestResourceAuthorizerHonorsContext(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	reviews := subjectAccessReviewFunc(func(ctx context.Context, _ *authorizationv1.SubjectAccessReview, _ metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	err := newAuthorizer(t, reviews).Authorize(ctx, &authenticationv1.UserInfo{Username: testServiceAccount}, testTaskAttributes())
	if !errors.Is(err, auth.ErrAuthorizationUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Authorize error = %v, want preserved context deadline", err)
	}
}

func testTaskAttributes() authorizationv1.ResourceAttributes {
	return authorizationv1.ResourceAttributes{Group: "kubetask.kubetask.io", Version: "v1", Resource: testTaskResource, Verb: "list"}
}

func newAuthorizer(t *testing.T, reviews subjectAccessReviewFunc) *auth.ResourceAuthorizer {
	t.Helper()
	a, err := auth.NewResourceAuthorizer(reviews)
	if err != nil {
		t.Fatalf("create authorizer: %v", err)
	}
	return a
}
