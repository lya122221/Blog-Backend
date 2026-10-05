package models

type ViewRequest struct {
	Events []ViewEvent `json:"events"`
}
