package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"

	kubetaskv1 "kubetask.io/kubetask/api/v1"
	"kubetask.io/kubetask/internal/auth"
)

const (
	authorizationTimeout = 3 * time.Second
	authorizerContextKey = "kubetask.authorization.checker"
	taskResource         = "tasks"
	workflowResource     = "workflows"
	getVerb              = "get"
	listVerb             = "list"
	updateVerb           = "update"
)

// Authorizer 对可信身份检查一个实际资源操作的权限。
type Authorizer interface {
	Authorize(context.Context, *authenticationv1.UserInfo, authorizationv1.ResourceAttributes) error
}

type resourcePermission struct {
	resource, verb, subresource string
}

// 路由必须显式列出权限；新增路由而未登记时拒绝执行，避免默认放行。
var routePermissions = map[string][]resourcePermission{
	"POST /api/v1/tasks":               {{taskResource, "create", ""}},
	"GET /api/v1/tasks":                {{taskResource, listVerb, ""}},
	"GET /api/v1/tasks/:name":          {{taskResource, getVerb, ""}},
	"PUT /api/v1/tasks/:name":          {{taskResource, getVerb, ""}, {taskResource, updateVerb, ""}},
	"DELETE /api/v1/tasks/:name":       {{taskResource, "delete", ""}},
	"POST /api/v1/tasks/:name/trigger": {{taskResource, getVerb, ""}, {taskResource, updateVerb, ""}, {taskResource, updateVerb, "status"}},
	"POST /api/v1/tasks/:name/suspend": {{taskResource, getVerb, ""}, {taskResource, updateVerb, ""}},
	"POST /api/v1/tasks/:name/resume":  {{taskResource, getVerb, ""}, {taskResource, updateVerb, ""}},
	"GET /api/v1/tasks/:name/logs":     {{taskResource, getVerb, ""}},
	"GET /api/v1/stats":                {{taskResource, listVerb, ""}},
	"GET /api/v1/stats/trend":          {{taskResource, listVerb, ""}},
	"POST /api/v1/workflows":           {{workflowResource, "create", ""}},
	"GET /api/v1/workflows":            {{workflowResource, listVerb, ""}},
	"GET /api/v1/workflows/:name":      {{workflowResource, getVerb, ""}},
	"DELETE /api/v1/workflows/:name":   {{workflowResource, getVerb, ""}, {workflowResource, "delete", ""}},
}

// Authorization 必须在 Authentication 之后挂载；全部检查通过后才执行业务 handler。
func Authorization(authorizer Authorizer) gin.HandlerFunc {
	return func(c *gin.Context) {
		permissions, ok := routePermissions[c.Request.Method+" "+c.FullPath()]
		if !ok || authorizer == nil {
			rejectAuthentication(c, http.StatusServiceUnavailable)
			return
		}
		attributes := make([]authorizationv1.ResourceAttributes, 0, len(permissions))
		for _, permission := range permissions {
			attributes = append(attributes, authorizationv1.ResourceAttributes{
				Group: kubetaskv1.SchemeGroupVersion.Group, Version: kubetaskv1.SchemeGroupVersion.Version,
				Resource: permission.resource, Verb: permission.verb, Subresource: permission.subresource,
				Name: c.Param("name"), Namespace: TenantNamespace(c),
			})
		}
		if !checkResourceAccess(c, authorizer, attributes...) {
			return
		}
		// 动态资源（Pod 日志）由 handler 在确定实际目标后再次检查。
		c.Set(authorizerContextKey, authorizer)
		c.Next()
	}
}

// AuthorizeResource 检查动态目标；未挂载授权中间件时保持既有匿名模式。
func AuthorizeResource(c *gin.Context, attributes authorizationv1.ResourceAttributes) bool {
	value, enabled := c.Get(authorizerContextKey)
	if !enabled {
		return true
	}
	authorizer, ok := value.(Authorizer)
	if !ok || authorizer == nil {
		rejectAuthentication(c, http.StatusServiceUnavailable)
		return false
	}
	return checkResourceAccess(c, authorizer, attributes)
}

func checkResourceAccess(c *gin.Context, authorizer Authorizer, attributes ...authorizationv1.ResourceAttributes) bool {
	user, ok := AuthenticatedUser(c)
	if !ok {
		rejectAuthentication(c, http.StatusUnauthorized)
		return false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), authorizationTimeout)
	defer cancel() // 不替换业务请求的 context，日志流不受这个期限限制。
	for _, attribute := range attributes {
		err := authorizer.Authorize(ctx, user, attribute)
		status := http.StatusServiceUnavailable
		switch {
		case ctx.Err() != nil:
		case errors.Is(err, auth.ErrUnauthenticated):
			status = http.StatusUnauthorized
		case errors.Is(err, auth.ErrForbidden):
			status = http.StatusForbidden
		case err == nil:
			continue
		}
		rejectAuthentication(c, status)
		return false
	}
	return true
}
