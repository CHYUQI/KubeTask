package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	authenticationv1 "k8s.io/api/authentication/v1"

	"kubetask.io/kubetask/internal/auth"
)

const (
	authenticationTimeout = 3 * time.Second
	identityContextKey    = "kubetask.authentication.user"
)

// Authenticator 只校验凭据并返回完整可信身份，不负责资源授权。
type Authenticator interface {
	Authenticate(context.Context, string) (*authenticationv1.UserInfo, error)
}

// Authentication 从 Authorization 请求头读取 Bearer Token，校验成功后才执行后续 handler。
// 校验器必须遵守 context 的取消和超时；认证超时不会施加到后续业务请求上。
func Authentication(authenticator Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.Request.Header)
		if !ok {
			rejectAuthentication(c, http.StatusUnauthorized)
			return
		}
		if authenticator == nil {
			rejectAuthentication(c, http.StatusServiceUnavailable)
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), authenticationTimeout)
		user, err := authenticator.Authenticate(ctx, token)
		contextErr := ctx.Err()
		cancel() // 只为认证创建子 context，且在执行任何业务 handler 前释放。
		if contextErr != nil {
			rejectAuthentication(c, http.StatusServiceUnavailable)
			return
		}
		if err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, auth.ErrUnauthenticated) {
				status = http.StatusUnauthorized
			}
			rejectAuthentication(c, status)
			return
		}
		if user == nil {
			rejectAuthentication(c, http.StatusServiceUnavailable)
			return
		}

		c.Set(identityContextKey, user.DeepCopy())
		c.Next()
	}
}

// AuthenticatedUser 返回认证身份的深拷贝，避免读取方修改后续 handler 看到的可信身份。
func AuthenticatedUser(c *gin.Context) (*authenticationv1.UserInfo, bool) {
	value, exists := c.Get(identityContextKey)
	user, ok := value.(*authenticationv1.UserInfo)
	if !exists || !ok || user == nil {
		return nil, false
	}
	return user.DeepCopy(), true
}

func rejectAuthentication(c *gin.Context, status int) {
	if status == http.StatusUnauthorized {
		c.Header("WWW-Authenticate", "Bearer")
	}
	c.AbortWithStatusJSON(status, gin.H{"error": http.StatusText(status)})
}

func bearerToken(header http.Header) (string, bool) {
	values := header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	scheme, token, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimLeft(token, " ")
	// RFC 6750 的 b64token 语法；不解码 Token，也不假设它是 JWT。
	content := strings.TrimRight(token, "=")
	if content == "" {
		return "", false
	}
	for _, ch := range content {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || strings.ContainsRune("-._~+/", ch) {
			continue
		}
		return "", false
	}
	return token, true
}
