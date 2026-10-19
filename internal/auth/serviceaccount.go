package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authenticationclient "k8s.io/client-go/kubernetes/typed/authentication/v1"
)

var (
	// ErrUnauthenticated 表示凭据未通过 ServiceAccount 身份校验。
	ErrUnauthenticated = errors.New("invalid service account token")
	// ErrAuthenticationUnavailable 表示 Kubernetes 未能完成凭据校验。
	ErrAuthenticationUnavailable = errors.New("authentication service unavailable")
)

// ServiceAccountAuthenticator 委托 Kubernetes 校验 Token，不自行解析或信任 JWT 声明。
// 它只确认身份；资源访问权限由后续的授权层判断。
type ServiceAccountAuthenticator struct {
	reviews  authenticationclient.TokenReviewInterface
	audience string
}

func NewServiceAccountAuthenticator(reviews authenticationclient.TokenReviewInterface, audience string) (*ServiceAccountAuthenticator, error) {
	if reviews == nil {
		return nil, errors.New("token review client is required")
	}
	if strings.TrimSpace(audience) == "" {
		return nil, errors.New("token audience is required")
	}
	return &ServiceAccountAuthenticator{reviews: reviews, audience: audience}, nil
}

// Authenticate 接收原始 Token，返回 Kubernetes 确认的完整身份。
// 调用方负责从 Authorization 请求头提取 Token，并通过 ctx 控制超时和取消。
func (a *ServiceAccountAuthenticator) Authenticate(ctx context.Context, token string) (*authenticationv1.UserInfo, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrUnauthenticated
	}

	review, err := a.reviews.Create(ctx, &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token:     token,
			Audiences: []string{a.audience},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAuthenticationUnavailable, err)
	}
	if review == nil {
		return nil, ErrAuthenticationUnavailable
	}
	// 过期等凭据错误也可能出现在 Status.Error 中；统一拒绝且不透传错误文本。
	if review.Status.Error != "" || !review.Status.Authenticated || !slices.Contains(review.Status.Audiences, a.audience) {
		return nil, ErrUnauthenticated
	}

	// Kubernetes ServiceAccount 的规范用户名为 system:serviceaccount:<namespace>:<name>。
	parts := strings.Split(review.Status.User.Username, ":")
	if len(parts) != 4 || parts[0] != "system" || parts[1] != "serviceaccount" || parts[2] == "" || parts[3] == "" {
		return nil, ErrUnauthenticated
	}

	return review.Status.User.DeepCopy(), nil
}
