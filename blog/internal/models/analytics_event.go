package models

import "time"

const AnalyticsSchemaVersion = 1

const (
	ArticleCreated = "article.created"
	ArticleUpdated = "article.updated"
	ArticleDeleted = "article.deleted"
	ArticleLiked   = "article.liked"
	ArticleUnliked = "article.unliked"
	CommentCreated = "comment.created"
)

type AnalyticsEvent struct {
	Version    int       `json:"version"`
	ID         string    `json:"event_id"`
	Type       string    `json:"type"`
	ArticleID  string    `json:"article_id"`
	AuthorID   string    `json:"author_id,omitempty"`
	UserID     string    `json:"user_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	Title      string    `json:"title,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
}
