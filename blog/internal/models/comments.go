package models

import "time"

type Comment struct {
	ID        string    `json:"id"`
	ArticleID string    `json:"article_id"`
	Author    Author    `json:"author"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateCommentRequest struct {
	Content string `json:"content" binding:"required"`
}
