package activity_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/radian-solusi/microservice-helpers/activity"
	pubsubtypes "github.com/radian-solusi/pubsub-event-types/v1"
)

func TestBuildActivityDocument_BasicFields(t *testing.T) {
	phone := "08123456789"
	email := "user@example.com"
	metaJSON := `{"username":"test_user","user_type":"Investor","actor_name":"test_user","actor_email":"user@example.com","ticker_code":"ABCD","lu_broker_id":"YP001"}`

	event := pubsubtypes.ActivityEvent{
		Category:    "REGISTRATION",
		Subcategory: "Complete Signup",
		UserID:      "uuid-123",
		IPAddress:   "10.0.0.1",
		Message:     "test_user completed registration",
		CreatedAt:   time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC),
		Metadata:    &metaJSON,
		Phone:       &phone,
		Email:       &email,
	}

	doc := activity.BuildActivityDocument(event, "custom_service")

	assert.NotNil(t, doc)
	assert.Equal(t, "REGISTRATION", doc["category"])
	assert.Equal(t, "Complete Signup", doc["sub_category"])
	assert.Equal(t, "Complete Signup", doc["activity"])
	assert.Equal(t, "uuid-123", doc["user_id"])
	assert.Equal(t, "10.0.0.1", doc["ip_address"])
	assert.Equal(t, "test_user completed registration", doc["descriptions"])
	assert.Equal(t, "user@example.com", doc["email"])
	assert.Equal(t, "test_user", doc["username"])
	assert.Equal(t, "Investor", doc["user_type"])
	assert.Equal(t, "test_user", doc["actor_name"])
	assert.Equal(t, "user@example.com", doc["actor_email"])
	assert.Equal(t, "ABCD", doc["ticker_code"])
	assert.Equal(t, "YP001", doc["lu_broker_id"])
	assert.Equal(t, "2026-04-15 10:30:00", doc["timestamp"])
	assert.Equal(t, "IDXEIPOWS1", doc["server"])
	assert.Equal(t, "custom_service", doc["producer"])
	assert.Equal(t, "1.0", doc["schema_version"])
	assert.Equal(t, "REGISTRATION.Complete Signup", doc["event_type"])
}

func TestBuildActivityDocument_Defaults(t *testing.T) {
	event := pubsubtypes.ActivityEvent{
		Category:    "SYSTEM",
		Subcategory: "Ping",
		Message:     "Ping event",
	}

	doc := activity.BuildActivityDocument(event)

	assert.NotNil(t, doc)
	assert.Equal(t, "System", doc["user_id"])
	assert.Equal(t, "System", doc["username"])
	assert.Equal(t, "System", doc["email"])
	assert.Equal(t, "System", doc["user_type"])
	assert.Equal(t, "-", doc["ip_address"])
	assert.Equal(t, "-", doc["filename"])
	assert.Equal(t, "-", doc["lu_broker_id"])
	assert.Equal(t, "e-ipo", doc["producer"])
	assert.NotEmpty(t, doc["event_id"])
	assert.NotEmpty(t, doc["timestamp"])
}

func TestEnrichActivityEvent(t *testing.T) {
	event := pubsubtypes.ActivityEvent{
		Category:    "AUTH",
		Subcategory: "LOGIN",
	}

	activity.EnrichActivityEvent(&event, "test-auth")

	assert.NotNil(t, event.Metadata)
	assert.False(t, event.CreatedAt.IsZero())

	var meta map[string]any
	err := json.Unmarshal([]byte(*event.Metadata), &meta)
	assert.NoError(t, err)

	assert.NotEmpty(t, meta["event_id"])
	assert.NotEmpty(t, meta["occurred_at"])
	assert.Equal(t, "test-auth", meta["producer"])
	assert.Equal(t, "AUTH.LOGIN", meta["event_type"])
	assert.Equal(t, "1.0", meta["schema_version"])
}

// TestBuildActivityDocument_MetadataSanitized_AllRedundant verifies that when
// every key in metadata is already a top-level field, the stored "metadata"
// value is nil — matching the monolith's flat schema (no metadata column).
func TestBuildActivityDocument_MetadataSanitized_AllRedundant(t *testing.T) {
	// All of these keys have dedicated top-level columns — none should survive.
	metaJSON := `{
		"username":"alice","email":"alice@example.com",
		"user_type":"Admin","actor_name":"alice","actor_email":"alice@example.com",
		"activity":"Login","event_id":"ev-1","occurred_at":"2026-01-01T00:00:00Z",
		"producer":"auth","event_type":"AUTH.LOGIN","schema_version":"1.0",
		"ip_address":"1.2.3.4","timestamp":"2026-01-01 00:00:00","server":"IDXEIPOWS1",
		"category":"AUTH","sub_category":"LOGIN","subcategory":"LOGIN",
		"descriptions":"Login successful","message":"Login successful",
		"ticker_code":"ABCD","code":"ABCD","order_qty":100,"allotment_qty":50,
		"view":"some-view","broker":"BRK","actor_name":"alice","actor_email":"alice@example.com",
		"old_code":"X","new_code":"Y","filename":"test.pdf",
		"lu_broker_id":"YP001","broker_id":"YP001","correlation_id":"corr-1","name":"Login"
	}`

	email := "alice@example.com"
	event := pubsubtypes.ActivityEvent{
		Category:    "AUTH",
		Subcategory: "LOGIN",
		UserID:      "user-1",
		Email:       &email,
		Message:     "Login successful",
		Metadata:    &metaJSON,
	}

	doc := activity.BuildActivityDocument(event, "auth")

	// metadata should be omitted completely from document when all keys are redundant
	_, exists := doc["metadata"]
	assert.False(t, exists, "expected metadata key to be omitted completely from document")
	assert.Nil(t, doc["metadata"],
		"expected metadata to be nil when all keys are top-level duplicates")
}

// TestBuildActivityDocument_MetadataSanitized_ContextualKeysPreserved verifies
// that truly unique contextual keys (e.g. action_url, diff) survive sanitization.
func TestBuildActivityDocument_MetadataSanitized_ContextualKeysPreserved(t *testing.T) {
	// Mixes redundant keys with a unique contextual key.
	metaJSON := `{
		"username":"bob","email":"bob@example.com","user_type":"Investor",
		"actor_name":"bob","actor_email":"bob@example.com",
		"action_url":"https://portal.e-ipo.co.id/reset?token=abc123"
	}`

	email := "bob@example.com"
	event := pubsubtypes.ActivityEvent{
		Category:    "USER",
		Subcategory: "PASSWORD_RESET",
		UserID:      "user-2",
		Email:       &email,
		Message:     "Password reset requested",
		Metadata:    &metaJSON,
	}

	doc := activity.BuildActivityDocument(event, "auth")

	// metadata must not be nil — action_url is non-redundant
	assert.NotNil(t, doc["metadata"],
		"expected metadata to be non-nil when contextual keys are present")

	cleaned, ok := doc["metadata"].(map[string]any)
	assert.True(t, ok, "expected metadata to be map[string]any")

	// action_url should survive
	assert.Equal(t, "https://portal.e-ipo.co.id/reset?token=abc123", cleaned["action_url"])

	// redundant keys must be stripped
	assert.Nil(t, cleaned["username"])
	assert.Nil(t, cleaned["email"])
	assert.Nil(t, cleaned["user_type"])
	assert.Nil(t, cleaned["actor_name"])
	assert.Nil(t, cleaned["actor_email"])
}

// TestEnrichActivityEvent_NullMetadata verifies that decoding JSON literal "null"
// does not reset map to nil and cause panic when defaults are assigned.
func TestEnrichActivityEvent_NullMetadata(t *testing.T) {
	nullMeta := "null"
	event := pubsubtypes.ActivityEvent{
		Category:    "AUTH",
		Subcategory: "LOGIN",
		Metadata:    &nullMeta,
	}

	assert.NotPanics(t, func() {
		activity.EnrichActivityEvent(&event, "test-producer")
	})
	assert.NotNil(t, event.Metadata)
	assert.NotEqual(t, "null", *event.Metadata)
}

// TestBuildActivityDocument_NullMetadata verifies that BuildActivityDocument handles
// Metadata pointing to "null" without panicking.
func TestBuildActivityDocument_NullMetadata(t *testing.T) {
	nullMeta := "null"
	event := pubsubtypes.ActivityEvent{
		Category:    "AUTH",
		Subcategory: "LOGIN",
		Metadata:    &nullMeta,
	}

	var doc map[string]any
	assert.NotPanics(t, func() {
		doc = activity.BuildActivityDocument(event)
	})
	assert.NotNil(t, doc)
	assert.NotEmpty(t, doc["event_id"])
	_, exists := doc["metadata"]
	assert.False(t, exists, "metadata should be omitted")
}

// TestBuildActivityDocument_DeterministicEventIDAcrossRedeliveries verifies that
// when an event lacks a metadata event_id, redeliveries of the identical event
// generate the exact same event_id so deduplication works properly.
func TestBuildActivityDocument_DeterministicEventIDAcrossRedeliveries(t *testing.T) {
	createdAt := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	event := pubsubtypes.ActivityEvent{
		Category:    "ORDER",
		Subcategory: "SUBMIT",
		UserID:      "user-redelivery-test",
		Message:     "Order submitted for ABCD",
		CreatedAt:   createdAt,
	}

	doc1 := activity.BuildActivityDocument(event, "order-service")
	doc2 := activity.BuildActivityDocument(event, "order-service")

	assert.NotEmpty(t, doc1["event_id"])
	assert.NotEmpty(t, doc2["event_id"])
	assert.Equal(t, doc1["event_id"], doc2["event_id"],
		"event_id must be deterministic across redeliveries of the same event")
}
