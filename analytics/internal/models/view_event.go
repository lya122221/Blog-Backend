package models

import "time"

type ViewEvent struct {
	ID         string    `json:"event_id"`
	Type       EventType `json:"type"`
	ArticleID  string    `json:"article_id"`
	OccurredAt time.Time `json:"occurred_at"`
}
