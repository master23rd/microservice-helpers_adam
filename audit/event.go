// Package audit contains transport-safe primitives for recording audit trails.
// Persistence is intentionally left to the owning service/database.
package audit

import (
	"net"
	"net/http"
	"strings"
	"time"
)

type Actor struct {
	UserID   string
	Name     string
	Email    string
	UserType int
}

type Event struct {
	Actor      Actor
	IPAddress  string
	Activity   string
	Resource   string
	ResourceID string
	Metadata   map[string]any
	OccurredAt time.Time
}

// ClientIP returns the immediate peer IP. Forwarded headers are deliberately
// ignored: accepting them here would let an untrusted client forge audit data.
func ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

// NewEvent creates an event with a stable timestamp and the request peer IP.
func NewEvent(r *http.Request, actor Actor, activity, resource, resourceID string, metadata map[string]any) Event {
	return Event{
		Actor:      actor,
		IPAddress:  ClientIP(r),
		Activity:   activity,
		Resource:   resource,
		ResourceID: resourceID,
		Metadata:   metadata,
		OccurredAt: time.Now().UTC(),
	}
}
