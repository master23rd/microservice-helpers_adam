package activity

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	pubsubtypes "github.com/radian-solusi/pubsub-event-types/v1"
)

const (
	ActivityCollectionName = "activity"
	DefaultActivityServer  = "IDXEIPOWS1"
	ActivitySchemaVersion  = "1.0"
	DefaultProducerName    = "e-ipo"
)

// MetaString safely retrieves a string value from a map[string]any.
func MetaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// EnrichActivityEvent enriches the given ActivityEvent by ensuring default metadata fields
// (event_id, occurred_at, producer, event_type, schema_version) are populated in event.Metadata.
func EnrichActivityEvent(event *pubsubtypes.ActivityEvent, defaultProducer ...string) {
	producer := DefaultProducerName
	if len(defaultProducer) > 0 && defaultProducer[0] != "" {
		producer = defaultProducer[0]
	}

	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	meta := make(map[string]any)
	if event.Metadata != nil && *event.Metadata != "" {
		_ = json.Unmarshal([]byte(*event.Metadata), &meta)
	}
	if meta == nil {
		meta = make(map[string]any)
	}

	if MetaString(meta, "event_id") == "" {
		meta["event_id"] = uuid.New().String()
	}
	if MetaString(meta, "occurred_at") == "" {
		meta["occurred_at"] = event.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	if MetaString(meta, "producer") == "" {
		meta["producer"] = producer
	}
	if MetaString(meta, "event_type") == "" {
		meta["event_type"] = fmt.Sprintf("%s.%s", event.Category, event.Subcategory)
	}
	if MetaString(meta, "schema_version") == "" {
		meta["schema_version"] = ActivitySchemaVersion
	}

	if raw, err := json.Marshal(meta); err == nil {
		s := string(raw)
		event.Metadata = &s
	}
}

// BuildActivityDocument maps pubsubtypes.ActivityEvent (plus optional JSON Metadata)
// into a MongoDB document compatible with the e-ipo monolith's "activity" collection (LogActivity)
// as well as target microservices deduplication fields (event_id, occurred_at, producer, event_type, schema_version).
func BuildActivityDocument(event pubsubtypes.ActivityEvent, defaultProducer ...string) map[string]any {
	producerDefault := DefaultProducerName
	if len(defaultProducer) > 0 && defaultProducer[0] != "" {
		producerDefault = defaultProducer[0]
	}

	var meta map[string]any
	if event.Metadata != nil && *event.Metadata != "" {
		_ = json.Unmarshal([]byte(*event.Metadata), &meta)
	}
	if meta == nil {
		meta = make(map[string]any)
	}

	createdAt := event.CreatedAt
	occurredAt := MetaString(meta, "occurred_at")

	// hasStableTime tracks whether the timestamp originates from a reliable source
	// (i.e. the producer-stamped event.CreatedAt or a parsed occurred_at from metadata).
	// When both are absent we fall back to time.Now(), which is NOT stable across
	// redeliveries and must therefore be excluded from the deterministic event_id hash.
	hasStableTime := !createdAt.IsZero()

	if occurredAt != "" && createdAt.IsZero() {
		if t, err := time.Parse(time.RFC3339Nano, occurredAt); err == nil {
			createdAt = t
			hasStableTime = true
		} else if t, err := time.Parse(time.RFC3339, occurredAt); err == nil {
			createdAt = t
			hasStableTime = true
		}
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	if occurredAt == "" {
		occurredAt = createdAt.UTC().Format(time.RFC3339Nano)
	}

	eventType := MetaString(meta, "event_type")
	if eventType == "" {
		eventType = fmt.Sprintf("%s.%s", event.Category, event.Subcategory)
	}

	schemaVersion := MetaString(meta, "schema_version")
	if schemaVersion == "" {
		schemaVersion = ActivitySchemaVersion
	}

	producer := MetaString(meta, "producer")
	if producer == "" {
		producer = producerDefault
	}

	eventID := MetaString(meta, "event_id")
	if eventID == "" {
		// Build a deterministic UUIDv5 identity from stable, distinguishing fields.
		//
		// Rules:
		//  - Always include Category, Subcategory, UserID, Message and producer to
		//    separate events from distinct services that otherwise look identical.
		//  - Include the timestamp (UnixNano) ONLY when it originates from a reliable
		//    source (event.CreatedAt or a parsed occurred_at). When we had to fall back
		//    to time.Now(), the timestamp changes on every redelivery and would break
		//    deduplication – so we omit it from the hash in that case.
		//  - Include correlation_id when present; it naturally distinguishes concurrent
		//    flows that share every other field (e.g. two simultaneous batch orders).
		var identity string
		correlationID := MetaString(meta, "correlation_id")
		if hasStableTime {
			identity = fmt.Sprintf("%s|%s|%s|%d|%s|%s|%s",
				event.Category,
				event.Subcategory,
				event.UserID,
				createdAt.UTC().UnixNano(),
				event.Message,
				producer,
				correlationID,
			)
		} else {
			// No stable time – rely on all other distinguishing dimensions.
			// Note: without a timestamp two genuinely distinct events that share all
			// other fields will collide; however this degenerate case (no CreatedAt,
			// no occurred_at, no pre-assigned event_id) should never occur when
			// publishers properly call EnrichActivityEvent before publishing.
			identity = fmt.Sprintf("%s|%s|%s|%s|%s|%s",
				event.Category,
				event.Subcategory,
				event.UserID,
				event.Message,
				producer,
				correlationID,
			)
		}
		eventID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(identity)).String()
	}

	ipAddress := event.IPAddress
	if ipAddress == "" {
		if v := MetaString(meta, "ip_address"); v != "" {
			ipAddress = v
		} else {
			ipAddress = "-"
		}
	}

	server := MetaString(meta, "server")
	if server == "" {
		server = DefaultActivityServer
	}

	activityName := MetaString(meta, "activity")
	if activityName == "" {
		activityName = MetaString(meta, "name")
	}
	if activityName == "" {
		activityName = event.Subcategory
	}

	userID := event.UserID
	if userID == "" {
		userID = "System"
	}

	username := MetaString(meta, "username")
	if username == "" {
		username = "System"
	}

	email := ""
	if event.Email != nil && *event.Email != "" {
		email = *event.Email
	} else if v := MetaString(meta, "email"); v != "" {
		email = v
	} else {
		email = "System"
	}

	var phone any
	if event.Phone != nil && *event.Phone != "" {
		phone = *event.Phone
	} else if v := MetaString(meta, "phone"); v != "" {
		phone = v
	}

	userType := MetaString(meta, "user_type")
	if userType == "" {
		userType = "System"
	}

	actorName := MetaString(meta, "actor_name")
	if actorName == "" {
		actorName = username
	}

	actorEmail := MetaString(meta, "actor_email")
	if actorEmail == "" {
		actorEmail = email
	}

	var view any = userID
	if v, ok := meta["view"]; ok && v != nil && v != "" {
		view = v
	}

	var tickerCode any
	if v := MetaString(meta, "ticker_code"); v != "" {
		tickerCode = v
	} else if v := MetaString(meta, "code"); v != "" {
		tickerCode = v
	}

	filename := MetaString(meta, "filename")
	if filename == "" {
		filename = "-"
	}

	var luBrokerID any = "-"
	if v, ok := meta["lu_broker_id"]; ok && v != nil && v != "" {
		luBrokerID = v
	} else if v, ok := meta["broker_id"]; ok && v != nil && v != "" {
		luBrokerID = v
	}

	timestamp := MetaString(meta, "timestamp")
	if timestamp == "" {
		timestamp = createdAt.Format("2006-01-02 15:04:05")
	}

	// sanitizedMeta strips every key that is already represented as a dedicated
	// top-level field in the MongoDB document so we comply with DRY and keep the
	// stored document free of redundant data. Only truly contextual attributes
	// (e.g. action_url, diff, custom params) remain.
	sanitizedMeta := sanitizeMetadata(meta)

	doc := map[string]any{
		"event_id":       eventID,
		"occurred_at":    occurredAt,
		"producer":       producer,
		"event_type":     eventType,
		"schema_version": schemaVersion,
		"correlation_id": meta["correlation_id"],
		"ip_address":     ipAddress,
		"timestamp":      timestamp,
		"created_at":     createdAt,
		"server":         server,
		"category":       event.Category,
		"sub_category":   event.Subcategory,
		"activity":       activityName,
		"user_id":        userID,
		"username":       username,
		"email":          email,
		"phone":          phone,
		"user_type":      userType,
		"descriptions":   event.Message,
		"ticker_code":    tickerCode,
		"order_qty":      meta["order_qty"],
		"allotment_qty":  meta["allotment_qty"],
		"view":           view,
		"broker":         meta["broker"],
		"actor_name":     actorName,
		"actor_email":    actorEmail,
		"old_code":       meta["old_code"],
		"new_code":       meta["new_code"],
		"filename":       filename,
		"lu_broker_id":   luBrokerID,
	}

	// Actually omit metadata key from the document when empty or redundant,
	// preserving the monolith's 100% flat schema in MongoDB.
	if sanitizedMeta != nil {
		doc["metadata"] = sanitizedMeta
	}

	return doc
}

// topLevelMetaKeys is the set of metadata keys that are already promoted to
// dedicated top-level fields in the MongoDB activity document.
// Any key in this set is stripped from the stored metadata object to prevent
// duplication and to keep the schema consistent with the monolith's flat layout.
var topLevelMetaKeys = map[string]struct{}{
	// event envelope fields
	"event_id":       {},
	"occurred_at":    {},
	"producer":       {},
	"event_type":     {},
	"schema_version": {},
	"correlation_id": {},
	// monolith-compatible top-level columns (mirrors LogActivity.php attributes())
	"ip_address":    {},
	"timestamp":     {},
	"server":        {},
	"category":      {},
	"sub_category":  {},
	"subcategory":   {},
	"activity":      {},
	"name":          {},
	"user_id":       {},
	"username":      {},
	"email":         {},
	"phone":         {},
	"user_type":     {},
	"descriptions":  {},
	"message":       {},
	"ticker_code":   {},
	"code":          {},
	"order_qty":     {},
	"allotment_qty": {},
	"view":          {},
	"broker":        {},
	"actor_name":    {},
	"actor_email":   {},
	"old_code":      {},
	"new_code":      {},
	"filename":      {},
	"lu_broker_id":  {},
	"broker_id":     {},
}

// sanitizeMetadata returns a copy of meta with all top-level-redundant keys
// removed. Returns nil when the resulting map is empty so the field is omitted
// entirely from the MongoDB document — matching the monolith's flat schema.
func sanitizeMetadata(meta map[string]any) any {
	if len(meta) == 0 {
		return nil
	}
	cleaned := make(map[string]any, len(meta))
	for k, v := range meta {
		if _, redundant := topLevelMetaKeys[k]; redundant {
			continue
		}
		cleaned[k] = v
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}
