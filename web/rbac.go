package web

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
)

// RBACConfig configures the RBAC middleware.
type RBACConfig struct {
	ServiceCode string                           // e.g. "ebus_order"; required
	Shadow      bool                             // true: log would-be denials, then c.Next()
	GetUser     func(c *gin.Context) any         // session payload; each service passes its existing stateless/stateful lookup
	Forbidden   func(c *gin.Context)             // writes the service's standard 403 body
	Logf        func(format string, args ...any) // nil → log.Printf
}

type rbacAdminPrivileges struct {
	SuperAdmin bool `json:"super_admin"`
}

type rbacSessionPayload struct {
	UserRelatedID   any                 `json:"user_related_id"` // int64 in go-models; string in some test fixtures
	Scope           []ScopeAccess       `json:"scope"`
	AdminPrivileges rbacAdminPrivileges `json:"admin_privileges"`
}

// RBAC returns a gin middleware that authorizes requests against the session scope.
// It panics at construction when ServiceCode, GetUser or Forbidden is nil/empty.
func RBAC(cfg RBACConfig) gin.HandlerFunc {
	if cfg.ServiceCode == "" {
		panic("rbac: ServiceCode is required")
	}
	if cfg.GetUser == nil {
		panic("rbac: GetUser is required")
	}
	if cfg.Forbidden == nil {
		panic("rbac: Forbidden is required")
	}
	logf := cfg.Logf
	if logf == nil {
		logf = log.Printf
	}

	return func(c *gin.Context) {
		payload, ok := decodeRBACPayload(cfg.GetUser(c))
		if ok && payload.AdminPrivileges.SuperAdmin {
			c.Next()
			return
		}

		fullPath := c.FullPath()
		allowed := false
		if ok && fullPath != "" {
			allowed = scopeAllows(payload.Scope, cfg.ServiceCode, c.Request.Method, fullPath)
		}

		if allowed {
			c.Next()
			return
		}

		if cfg.Shadow {
			logf("rbac shadow-deny service=%s method=%s path=%s user_related_id=%v",
				cfg.ServiceCode, c.Request.Method, fullPath, payload.UserRelatedID)
			c.Next()
			return
		}

		cfg.Forbidden(c)
		c.Abort()
	}
}

// decodeRBACPayload normalizes the service-specific session payload via a JSON round-trip.
func decodeRBACPayload(v any) (rbacSessionPayload, bool) {
	var payload rbacSessionPayload
	raw, err := json.Marshal(v)
	if err != nil {
		return payload, false
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, false
	}
	return payload, true
}

// scopeAllows performs a linear scan of the scope; a few hundred entries, no cache.
func scopeAllows(scope []ScopeAccess, serviceCode, method, fullPath string) bool {
	for _, entry := range scope {
		if entry.Permission == "api" &&
			entry.IsAllowed &&
			entry.ServiceCode == serviceCode &&
			strings.EqualFold(entry.Method, method) &&
			entry.URL == fullPath {
			return true
		}
	}
	return false
}
