package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

var (
	// ErrForbidden 表示调用者未获得所请求资源操作的授权。
	ErrForbidden = errors.New("resource access forbidden")
	// ErrAuthorizationUnavailable 表示无法确定资源授权结果。
	ErrAuthorizationUnavailable = errors.New("authorization service unavailable")
)

// ResourceAuthorizer 委托 Kubernetes 判断已认证身份是否允许执行资源操作。
// 调用方负责传入可信身份和实际操作的属性，本组件不推导租户或 Namespace。
type ResourceAuthorizer struct {
	reviews authorizationclient.SubjectAccessReviewInterface
}

func NewResourceAuthorizer(reviews authorizationclient.SubjectAccessReviewInterface) (*ResourceAuthorizer, error) {
	if reviews == nil {
		return nil, errors.New("subject access review client is required")
	}
	return &ResourceAuthorizer{reviews: reviews}, nil
}

// Authorize 返回 nil 表示操作获准；未获准与无法完成审查分别返回不同错误。
// 超时和取消由 ctx 控制；不缓存权限，也不发送调用者的原始 Token。
func (a *ResourceAuthorizer) Authorize(ctx context.Context, user *authenticationv1.UserInfo, attributes authorizationv1.ResourceAttributes) error {
	if user == nil || strings.TrimSpace(user.Username) == "" {
		return ErrUnauthenticated
	}
	if attributes.Resource == "" || attributes.Verb == "" {
		return fmt.Errorf("%w: resource and verb are required", ErrAuthorizationUnavailable)
	}

	spec := authorizationv1.SubjectAccessReviewSpec{
		User:               user.Username,
		UID:                user.UID,
		Groups:             slices.Clone(user.Groups),
		ResourceAttributes: attributes.DeepCopy(),
	}
	if user.Extra != nil {
		spec.Extra = make(map[string]authorizationv1.ExtraValue, len(user.Extra))
		for key, value := range user.Extra {
			spec.Extra[key] = authorizationv1.ExtraValue(slices.Clone(value))
		}
	}

	review, err := a.reviews.Create(ctx, &authorizationv1.SubjectAccessReview{Spec: spec}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrAuthorizationUnavailable, err)
	}
	if review == nil || (review.Status.Allowed && review.Status.Denied) {
		return ErrAuthorizationUnavailable
	}

	// EvaluationError 可与有效决策并存，明确的允许/拒绝优先；无决策时才归为审查失败。
	// 不透传 Reason 或 EvaluationError，避免向上层暴露审查详情。
	switch {
	case review.Status.Denied:
		return ErrForbidden
	case review.Status.Allowed:
		return nil
	case review.Status.EvaluationError != "":
		return ErrAuthorizationUnavailable
	default:
		// Kubernetes 未明确允许（包括 no opinion）时拒绝操作。
		return ErrForbidden
	}
}
