package models

type PopularResponse struct {
	Window   string           `json:"window"`
	Articles []PopularArticle `json:"articles"`
}
