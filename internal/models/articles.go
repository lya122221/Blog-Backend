package models

import "time"

type Author struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type Article struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	ViewsCount int       `json:"views_count"`
	CreatedAt  time.Time `json:"created_at"`
	Author     Author    `json:"author"`
	Tags       []string  `json:"tags"`
}

type UpdateArticleRequest struct {
	Title   string   `json:"title" binding:"required"`
	Content string   `json:"content" binding:"required"`
	Tags    []string `json:"tags" binding:"required"`
}
