package services

import (
	"blog/internal/models"

	"github.com/google/uuid"
)

type InteractionsRepository interface {
	GetComments(articleID uuid.UUID) ([]models.Comment, error)
	CreateComment(articleID uuid.UUID, userID string, content string) error
	ToggleLike(articleID uuid.UUID, userID string) (bool, int, error)
}

type InteractionsService struct {
	repo InteractionsRepository
}

func NewInteractionsService(repo InteractionsRepository) *InteractionsService {
	return &InteractionsService{repo: repo}
}

func (s *InteractionsService) GetComments(articleIDStr string) ([]models.Comment, error) {
	articleID, err := uuid.Parse(articleIDStr)
	if err != nil {
		return nil, err
	}

	comments, err := s.repo.GetComments(articleID)
	if err != nil {
		return nil, err
	}

	return comments, nil
}

func (s *InteractionsService) CreateComment(articleIDStr string, userID string, content string) error {
	articleID, err := uuid.Parse(articleIDStr)
	if err != nil {
		return err
	}

	return s.repo.CreateComment(articleID, userID, content)
}

func (s *InteractionsService) ToggleLike(articleIDStr string, userID string) (bool, int, error) {
	articleID, err := uuid.Parse(articleIDStr)
	if err != nil {
		return false, 0, err
	}

	return s.repo.ToggleLike(articleID, userID)
}
