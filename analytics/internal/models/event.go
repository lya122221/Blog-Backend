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

	var requireAuthor, requireUser, requireVisitor, requireTitle bool
	switch event.Type {
	case ArticleImpression, ArticleOpened:
		requireVisitor = true
	case ArticleLiked, ArticleUnliked, CommentCreated:
		requireUser = true
	case ArticleCreated, ArticleUpdated, ArticleDeleted:
		requireAuthor = true
		requireTitle = event.Type != ArticleDeleted
	default:
		return fmt.Errorf("unsupported event type %q", event.Type)
	}
	for _, field := range []struct {
		name     string
		id       string
		required bool
	}{
		{"author", event.AuthorID, requireAuthor},
		{"user", event.UserID, requireUser},
		{"visitor", event.VisitorID, requireVisitor},
	} {
		if field.id == "" {
			if field.required {
				return fmt.Errorf("missing %s ID", field.name)
			}
			continue
		}
		if _, err := uuid.Parse(field.id); err != nil {
			return fmt.Errorf("invalid %s ID: %w", field.name, err)
		}
	}
	if requireTitle && strings.TrimSpace(event.Title) == "" {
		return fmt.Errorf("missing article title")
	}

	return nil
}
