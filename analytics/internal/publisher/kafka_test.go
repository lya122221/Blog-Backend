package publisher

import (
	"analytics/internal/models"
	"encoding/json"
	"testing"
	"time"
	"uuid"
)

func TestEncodeRecordsPreservesEventAndArticleKey(t *testing.T) {
	event := models.Event{
		Version:    models.SchemaVersion,
		ID:         uuid.New().String(),
		Type:       models.ArticleOpened,
		ArticleID:  uuid.New().String(),
		VisitorID:  uuid.New().String(),
		OccurredAt: time.Now().UTC(),
	}
	records, err := encodeRecords("article-events", []models.Event{event})
	if err != nil {
		t.Fatalf("encodeRecords: %v", err)
	}
	if len(records) != 1 || records[0].Topic != "article-events" || string(records[0].Key) != event.ArticleID {
		t.Fatalf("unexpected Kafka records: %+v", records)
	}
	var decoded models.Event
	if err := json.Unmarshal(records[0].Value, &decoded); err != nil {
		t.Fatalf("decode Kafka record: %v", err)
	}
	if decoded.ID != event.ID || decoded.Type != event.Type || decoded.VisitorID != event.VisitorID || !decoded.OccurredAt.Equal(event.OccurredAt) {
		t.Fatalf("record event = %+v, want %+v", decoded, event)
	}
}
