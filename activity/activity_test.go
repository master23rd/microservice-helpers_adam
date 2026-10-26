package activity_test

import (
	"encoding/json"
	"fmt"
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

// TestBuildActivityDocument_PreservesSuppliedEnvelopeFields verifies that when
// metadata supplies occurred_at, event_type, or schema_version, those are preserved
// instead of being unconditionally replaced by derived defaults.
func TestBuildActivityDocument_PreservesSuppliedEnvelopeFields(t *testing.T) {
	customOccurred := "2026-03-01T14:20:30.123456789Z"
	customEventType := "CUSTOM.EVENT.TYPE"
	customSchemaVer := "2.1"
	customProducer := "custom-service-origin"
	customEventID := "custom-uuid-456"

	metaJSON := fmt.Sprintf(`{
		"occurred_at": "%s",
		"event_type": "%s",
		"schema_version": "%s",
		"producer": "%s",
		"event_id": "%s"
	}`, customOccurred, customEventType, customSchemaVer, customProducer, customEventID)

	event := pubsubtypes.ActivityEvent{
		Category:    "FALLBACK_CAT",
		Subcategory: "FALLBACK_SUBCAT",
		UserID:      "user-envelope",
		Message:     "Envelope preservation test",
		Metadata:    &metaJSON,
	}

	doc := activity.BuildActivityDocument(event, "default-producer")

	assert.Equal(t, customOccurred, doc["occurred_at"])
	assert.Equal(t, customEventType, doc["event_type"])
	assert.Equal(t, customSchemaVer, doc["schema_version"])
	assert.Equal(t, customProducer, doc["producer"])
	assert.Equal(t, customEventID, doc["event_id"])
}

// TestBuildActivityDocument_ZeroCreatedAtWithSuppliedOccurredAt verifies that
// an event with zero CreatedAt but supplied occurred_at parses the true occurrence
// time into created_at rather than defaulting to time.Now() (processing time).
func TestBuildActivityDocument_ZeroCreatedAtWithSuppliedOccurredAt(t *testing.T) {
	historicalTimeStr := "2026-01-15T08:30:00Z"
	metaJSON := fmt.Sprintf(`{"occurred_at":"%s"}`, historicalTimeStr)

	event := pubsubtypes.ActivityEvent{
		Category:    "AUTH",
		Subcategory: "LOGIN",
		UserID:      "user-hist",
		Message:     "Historical event",
		Metadata:    &metaJSON,
		// CreatedAt is intentionally zero
	}

	doc := activity.BuildActivityDocument(event, "auth")

	assert.Equal(t, historicalTimeStr, doc["occurred_at"])
	createdAt, ok := doc["created_at"].(time.Time)
	assert.True(t, ok)
	expectedTime, _ := time.Parse(time.RFC3339, historicalTimeStr)
	assert.True(t, createdAt.Equal(expectedTime), "created_at should preserve historical occurrence time")
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
// when an event lacks a metadata event_id but HAS a stable timestamp, redeliveries of
// the identical event generate the exact same event_id so deduplication works properly.
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

// TestBuildActivityDocument_StableFallbackEventIDWithoutTimestamp verifies that when
// an event has no CreatedAt and no occurred_at (forcing a time.Now() fallback),
// the generated event_id remains consistent across redeliveries because time.Now()
// is intentionally excluded from the hash in this scenario.
func TestBuildActivityDocument_StableFallbackEventIDWithoutTimestamp(t *testing.T) {
	// Simulate an un-enriched event with no timestamp at all.
	event := pubsubtypes.ActivityEvent{
		Category:    "AUTH",
		Subcategory: "LOGIN",
		UserID:      "user-no-ts",
		Message:     "Login from unknown time",
		// CreatedAt is zero, no Metadata → falls back to time.Now() internally
	}

	doc1 := activity.BuildActivityDocument(event, "auth-service")
	// A short sleep to ensure time.Now() would differ if it were included in the hash.
	time.Sleep(2 * time.Millisecond)
	doc2 := activity.BuildActivityDocument(event, "auth-service")

	assert.Equal(t, doc1["event_id"], doc2["event_id"],
		"event_id must be stable even when no timestamp is available (time.Now() must NOT be hashed)")
}

// TestBuildActivityDocument_DistinctEventIDsForDifferentProducers verifies that two
// events that share Category, Subcategory, UserID, Message, and timestamp but come
// from different producers receive distinct event IDs so neither masks the other
// during downstream deduplication.
func TestBuildActivityDocument_DistinctEventIDsForDifferentProducers(t *testing.T) {
	createdAt := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	base := pubsubtypes.ActivityEvent{
		Category:    "ORDER",
		Subcategory: "SUBMIT",
		UserID:      "shared-user",
		Message:     "Order submitted",
		CreatedAt:   createdAt,
	}

	docA := activity.BuildActivityDocument(base, "service-A")
	docB := activity.BuildActivityDocument(base, "service-B")

	assert.NotEqual(t, docA["event_id"], docB["event_id"],
		"events from different producers must get distinct event_ids")
}
