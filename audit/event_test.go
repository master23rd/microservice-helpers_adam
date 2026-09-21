package audit

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientIPUsesPeerAddressInsteadOfForwardedHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req.Header.Set("X-Forwarded-For", "198.51.100.10")

	require.Equal(t, "203.0.113.10", ClientIP(req))
}

func TestNewEventSetsAuditContext(t *testing.T) {
	req := httptest.NewRequest("POST", "/roles", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	event := NewEvent(req, Actor{UserID: "u-1", Email: "admin@example.test"}, "role.created", "role", "10", map[string]any{"name": "Admin"})

	require.Equal(t, "127.0.0.1", event.IPAddress)
	require.Equal(t, "role.created", event.Activity)
	require.False(t, event.OccurredAt.IsZero())
}
