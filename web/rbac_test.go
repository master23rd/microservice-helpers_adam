package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func defaultForbidden(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
}

func defaultGetUser(payload any) func(c *gin.Context) any {
	return func(c *gin.Context) any {
		return payload
	}
}

func TestRBAC_AllowsMatchingEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	payload := map[string]any{
		"user_related_id": "u123",
		"scope": []map[string]any{
			{
				"permission":   "api",
				"is_allowed":   true,
				"service_code": "ebus_order",
				"method":       "GET",
				"url":          "/orders/:orderNo",
			},
		},
	}

	cfg := RBACConfig{
		ServiceCode: "ebus_order",
		GetUser:     defaultGetUser(payload),
		Forbidden:   defaultForbidden,
	}

	r.GET("/orders/:orderNo", RBAC(cfg), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/orders/123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRBAC_ParamRouteMatchesPatternNotRawPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	payload := map[string]any{
		"user_related_id": "u123",
		"scope": []map[string]any{
			{
				"permission":   "api",
				"is_allowed":   true,
				"service_code": "ebus_order",
				"method":       "GET",
				"url":          "/orders/123", // literal path in scope, but c.FullPath() will be /orders/:orderNo
			},
		},
	}

	cfg := RBACConfig{
		ServiceCode: "ebus_order",
		GetUser:     defaultGetUser(payload),
		Forbidden:   defaultForbidden,
	}

	r.GET("/orders/:orderNo", RBAC(cfg), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/orders/123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRBAC_DeniesOtherService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	payload := map[string]any{
		"scope": []map[string]any{
			{
				"permission":   "api",
				"is_allowed":   true,
				"service_code": "users",
				"method":       "GET",
				"url":          "/orders/:orderNo",
			},
		},
	}

	cfg := RBACConfig{
		ServiceCode: "ebus_order",
		GetUser:     defaultGetUser(payload),
		Forbidden:   defaultForbidden,
	}

	r.GET("/orders/:orderNo", RBAC(cfg), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/orders/123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRBAC_MethodMatching(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test 1: Scope POST vs Request GET -> 403
	{
		r := gin.New()
		payload := map[string]any{
			"scope": []map[string]any{
				{
					"permission":   "api",
					"is_allowed":   true,
					"service_code": "ebus_order",
					"method":       "POST",
					"url":          "/orders",
				},
			},
		}
		cfg := RBACConfig{
			ServiceCode: "ebus_order",
			GetUser:     defaultGetUser(payload),
			Forbidden:   defaultForbidden,
		}
		r.GET("/orders", RBAC(cfg), func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest("GET", "/orders", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for mismatched method, got %d", rec.Code)
		}
	}

	// Test 2: Scope "get" vs Request GET -> 200 (case-insensitive)
	{
		r := gin.New()
		payload := map[string]any{
			"scope": []map[string]any{
				{
					"permission":   "api",
					"is_allowed":   true,
					"service_code": "ebus_order",
					"method":       "get",
					"url":          "/orders",
				},
			},
		}
		cfg := RBACConfig{
			ServiceCode: "ebus_order",
			GetUser:     defaultGetUser(payload),
			Forbidden:   defaultForbidden,
		}
		r.GET("/orders", RBAC(cfg), func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest("GET", "/orders", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for case-insensitive method match, got %d", rec.Code)
		}
	}
}

func TestRBAC_IgnoresFrontendAndDisallowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	payload := map[string]any{
		"scope": []map[string]any{
			{
				"permission":   "frontend",
				"is_allowed":   true,
				"service_code": "ebus_order",
				"method":       "GET",
				"url":          "/orders",
			},
			{
				"permission":   "api",
				"is_allowed":   false,
				"service_code": "ebus_order",
				"method":       "GET",
				"url":          "/orders",
			},
		},
	}

	cfg := RBACConfig{
		ServiceCode: "ebus_order",
		GetUser:     defaultGetUser(payload),
		Forbidden:   defaultForbidden,
	}

	r.GET("/orders", RBAC(cfg), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/orders", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRBAC_SuperAdminBypasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	payload := map[string]any{
		"admin_privileges": map[string]any{
			"super_admin": true,
		},
		"scope": []map[string]any{},
	}

	cfg := RBACConfig{
		ServiceCode: "ebus_order",
		GetUser:     defaultGetUser(payload),
		Forbidden:   defaultForbidden,
	}

	r.GET("/orders", RBAC(cfg), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/orders", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for super_admin, got %d", rec.Code)
	}
}

func TestRBAC_UnmatchedRouteDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	payload := map[string]any{
		"scope": []map[string]any{
			{
				"permission":   "api",
				"is_allowed":   true,
				"service_code": "ebus_order",
				"method":       "GET",
				"url":          "/unknown",
			},
		},
	}

	cfg := RBACConfig{
		ServiceCode: "ebus_order",
		GetUser:     defaultGetUser(payload),
		Forbidden:   defaultForbidden,
	}

	r.NoRoute(RBAC(cfg), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/unknown", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unmatched route (empty FullPath), got %d", rec.Code)
	}
}

func TestRBAC_TrailingSlashDoesNotWiden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.RedirectTrailingSlash = false

	payload := map[string]any{
		"scope": []map[string]any{
			{
				"permission":   "api",
				"is_allowed":   true,
				"service_code": "ebus_order",
				"method":       "GET",
				"url":          "/orders",
			},
		},
	}

	cfg := RBACConfig{
		ServiceCode: "ebus_order",
		GetUser:     defaultGetUser(payload),
		Forbidden:   defaultForbidden,
	}

	r.GET("/orders/", RBAC(cfg), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/orders/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when scope is /orders and route registered is /orders/, got %d", rec.Code)
	}
}

func TestRBAC_ShadowAllowsAndLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	payload := map[string]any{
		"user_related_id": "usr_999",
		"scope":           []map[string]any{},
	}

	var logged []string
	cfg := RBACConfig{
		ServiceCode: "ebus_order",
		Shadow:      true,
		GetUser:     defaultGetUser(payload),
		Forbidden:   defaultForbidden,
		Logf: func(format string, args ...any) {
			logged = append(logged, fmt.Sprintf(format, args...))
		},
	}

	r.GET("/orders", RBAC(cfg), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/orders", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 in shadow mode, got %d", rec.Code)
	}

	if len(logged) != 1 {
		t.Fatalf("expected exactly 1 log line, got %d: %v", len(logged), logged)
	}

	msg := logged[0]
	if !strings.Contains(msg, "shadow-deny") || !strings.Contains(msg, "service=ebus_order") || !strings.Contains(msg, "user_related_id=usr_999") {
		t.Fatalf("unexpected shadow log message: %q", msg)
	}
}

func TestRBAC_BadPayloadDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	cfg := RBACConfig{
		ServiceCode: "ebus_order",
		GetUser: func(c *gin.Context) any {
			return make(chan int) // Unmarshalable via JSON
		},
		Forbidden: defaultForbidden,
	}

	r.GET("/orders", RBAC(cfg), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/orders", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on bad payload decode, got %d", rec.Code)
	}
}

func TestRBAC_PanicsOnMissingConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  RBACConfig
	}{
		{
			name: "missing service_code",
			cfg: RBACConfig{
				GetUser:   defaultGetUser(nil),
				Forbidden: defaultForbidden,
			},
		},
		{
			name: "missing get_user",
			cfg: RBACConfig{
				ServiceCode: "ebus_order",
				Forbidden:   defaultForbidden,
			},
		},
		{
			name: "missing forbidden",
			cfg: RBACConfig{
				ServiceCode: "ebus_order",
				GetUser:     defaultGetUser(nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for %s", tt.name)
				}
			}()
			RBAC(tt.cfg)
		})
	}
}
