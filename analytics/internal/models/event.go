package models

import (
	"fmt"
	"strings"
	"time"
	"uuid"
)

const SchemaVersion = 1

type EventType string

const (
	ArticleImpression EventType = "article.impression"
	ArticleOpened     EventType = "article.opened"
	ArticleLiked      EventType = "article.liked"
	ArticleUnliked    EventType = "article.unliked"
	CommentCreated    EventType = "comment.created"
	ArticleCreated    EventType = "article.created"
	ArticleUpdated    EventType = "article.updated"
	ArticleDeleted    EventType = "article.deleted"
)

type Event struct {
	Version    int       `json:"version"`
	ID         string    `json:"event_id"`
	Type       EventType `json:"type"`
	ArticleID  string    `json:"article_id"`
	AuthorID   string    `json:"author_id,omitempty"`
	UserID     string    `json:"user_id,omitempty"`
	VisitorID  string    `json:"visitor_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	Title      string    `json:"title,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
}

func (event Event) Validate() error {
	if event.Version != SchemaVersion {
		return fmt.Errorf("unsupported event version %d", event.Version)
	}
	if _, err := uuid.Parse(event.ID); err != nil {
		return fmt.Errorf("invalid event ID: %w", err)
	}
	if _, err := uuid.Parse(event.ArticleID); err != nil {
		return fmt.Errorf("invalid article ID: %w", err)
	}
	if event.OccurredAt.IsZero() {
		return fmt.Errorf("missing event time")
	}

	switch event.Type {
	case ArticleImpression, ArticleOpened:
		if _, err := uuid.Parse(event.VisitorID); err != nil {
			return fmt.Errorf("invalid visitor ID: %w", err)
		}
	case ArticleLiked, ArticleUnliked, CommentCreated:
		if _, err := uuid.Parse(event.UserID); err != nil {
			return fmt.Errorf("invalid user ID: %w", err)
		}
	case ArticleCreated, ArticleUpdated, ArticleDeleted:
		if _, err := uuid.Parse(event.AuthorID); err != nil {
			return fmt.Errorf("invalid author ID: %w", err)
		}
		if event.Type != ArticleDeleted && strings.TrimSpace(event.Title) == "" {
			return fmt.Errorf("missing article title")
		}
	default:
		return fmt.Errorf("unsupported event type %q", event.Type)
	}

	return nil
}
