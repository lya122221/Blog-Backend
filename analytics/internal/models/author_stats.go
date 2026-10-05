package models

import "time"

type StatsCounters struct {
	Impressions int64 `json:"impressions"`
	Opens       int64 `json:"opens"`
	Likes       int64 `json:"likes"`
	Unlikes     int64 `json:"unlikes"`
	Comments    int64 `json:"comments"`
	Score       int64 `json:"score"`
}

type StatsBucket struct {
	Start time.Time `json:"start"`
	StatsCounters
}

type AuthorStats struct {
	Period string        `json:"period"`
	From   time.Time     `json:"from"`
	To     time.Time     `json:"to"`
	Totals StatsCounters `json:"totals"`
	Series []StatsBucket `json:"series"`
}

type ArticleStats struct {
	ArticleID string   `json:"article_id"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags"`
	AuthorStats
}

type ArticleMetadata struct {
	AuthorID string
	Title    string
	Tags     []string
	Deleted  bool
}
