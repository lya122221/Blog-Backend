package models

import (
	"encoding/json"
	"time"
)

type OutboxEvent struct {
	ID        string
	Type      string
	ArticleID string
	Payload   json.RawMessage
	CreatedAt time.Time
}
