package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kubetask.io/kubetask/internal/tenant"
)

// TenantIsolation scopes an authenticated ServiceAccount to its Namespace.
func TenantIsolation() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := AuthenticatedUser(c)
		if !ok {
			rejectAuthentication(c, http.StatusUnauthorized)
			return
		}
		namespace := tenant.ServiceAccountNamespace(user.Username)
		if namespace == "" {
			rejectAuthentication(c, http.StatusUnauthorized)
			return
		}
		c.Request = c.Request.WithContext(tenant.WithNamespace(c.Request.Context(), namespace))
		c.Next()
	}
}

// TenantNamespace returns the scope assigned by TenantIsolation.
func TenantNamespace(c *gin.Context) string {
	return tenant.Namespace(c.Request.Context())
}
