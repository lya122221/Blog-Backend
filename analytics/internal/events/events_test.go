package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const (
	eventID   = "11111111-1111-4111-8111-111111111111"
	articleID = "22222222-2222-4222-8222-222222222222"
	visitorID = "33333333-3333-4333-8333-333333333333"
	userID    = "44444444-4444-4444-8444-444444444444"
	authorID  = "55555555-5555-4555-8555-555555555555"
)

func validEvent(eventType Type) Event {
	return Event{
		Version:    SchemaVersion,
		ID:         eventID,
		Type:       eventType,
		ArticleID:  articleID,
		VisitorID:  visitorID,
		UserID:     userID,
		AuthorID:   authorID,
		OccurredAt: time.Date(2026, time.September, 30, 12, 0, 0, 0, time.UTC),
		Title:      "Go article",
	}
}

func TestValidateAcceptsEveryEventType(t *testing.T) {
	for _, eventType := range []Type{
		ArticleImpression, ArticleOpened, ArticleLiked, ArticleUnliked,
		CommentCreated, ArticleCreated, ArticleUpdated, ArticleDeleted,
	} {
		t.Run(string(eventType), func(t *testing.T) {
			if err := validEvent(eventType).Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateRejectsInvalidEvents(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*Event)
		message string
	}{
		{"version", func(e *Event) { e.Version = 2 }, "version"},
		{"event ID", func(e *Event) { e.ID = "bad" }, "event ID"},
		{"article ID", func(e *Event) { e.ArticleID = "bad" }, "article ID"},
		{"time", func(e *Event) { e.OccurredAt = time.Time{} }, "event time"},
		{"visitor", func(e *Event) { e.VisitorID = "" }, "visitor ID"},
		{"user", func(e *Event) { e.Type = ArticleLiked; e.UserID = "" }, "user ID"},
		{"author", func(e *Event) { e.Type = ArticleDeleted; e.AuthorID = "" }, "author ID"},
		{"title", func(e *Event) { e.Type = ArticleCreated; e.Title = "  " }, "article title"},
		{"type", func(e *Event) { e.Type = "unknown" }, "event type"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := validEvent(ArticleImpression)
			test.change(&event)
			if err := event.Validate(); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Validate error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestEventJSONContract(t *testing.T) {
	event := validEvent(ArticleImpression)
	event.UserID = ""
	event.AuthorID = ""
	event.Title = ""
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	for _, field := range []string{`"event_id"`, `"article_id"`, `"visitor_id"`, `"occurred_at"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("missing %s in %s", field, encoded)
		}
	}
	if strings.Contains(string(encoded), `"user_id"`) || strings.Contains(string(encoded), `"author_id"`) {
		t.Fatalf("unexpected optional fields in %s", encoded)
	}

	var decoded Event
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("decoded event invalid: %v", err)
	}
}

func TestDefaultWeights(t *testing.T) {
	weights := DefaultWeights()
	for _, test := range []struct {
		eventType Type
		want      int
	}{
		{ArticleImpression, 1},
		{ArticleOpened, 3},
		{ArticleLiked, 6},
		{ArticleUnliked, -6},
		{CommentCreated, 6},
		{ArticleCreated, 0},
		{ArticleUpdated, 0},
		{ArticleDeleted, 0},
	} {
		got, err := weights.ScoreDelta(test.eventType)
		if err != nil || got != test.want {
			t.Fatalf("ScoreDelta(%q) = %d, %v; want %d", test.eventType, got, err, test.want)
		}
	}
	if _, err := weights.ScoreDelta("unknown"); err == nil {
		t.Fatal("expected unknown event type to fail")
	}
}

func TestCustomWeightsKeepUnlikeSymmetric(t *testing.T) {
	weights := Weights{Impression: 2, Open: 4, Like: 9, Comment: 5}
	liked, err := weights.ScoreDelta(ArticleLiked)
	if err != nil {
		t.Fatal(err)
	}
	unliked, err := weights.ScoreDelta(ArticleUnliked)
	if err != nil {
		t.Fatal(err)
	}
	if liked+unliked != 0 {
		t.Fatalf("like and unlike do not cancel: %d + %d", liked, unliked)
	}
}
