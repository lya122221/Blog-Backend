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
