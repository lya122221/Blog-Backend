package models

type PopularArticle struct {
	ArticleID string   `json:"article_id"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags"`
	Score     int64    `json:"score"`
}
