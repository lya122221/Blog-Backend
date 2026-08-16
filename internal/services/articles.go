package services

import (
	"blog/internal/models"

	"github.com/google/uuid"
)

type ArticlesRepository interface {
	GetArticles(limit int, offset int, tags []string) ([]models.Article, error)
	CreateArticle(authorID, title, content string, tags []string) error
	GetArticleWithID(articleID uuid.UUID) (*models.Article, error)
	UpdateArticle(authorID string, articleID uuid.UUID, request models.UpdateArticleRequest) error
}

type ArticlesService struct {
	repo ArticlesRepository
}

func NewArticlesService(repo ArticlesRepository) *ArticlesService {
	return &ArticlesService{repo: repo}
}

func (s *ArticlesService) GetArticles(page int, limit int, tags []string) ([]models.Article, error) {
	var offset int = (page - 1) * limit

	articles, err := s.repo.GetArticles(limit, offset, tags)
	if err != nil {
		return nil, err
	}

	return articles, nil
}

func (s *ArticlesService) CreateArticle(article models.Article) error {
	return s.repo.CreateArticle(article.Author.ID, article.Title, article.Content, article.Tags)
}

func (s *ArticlesService) GetArticleWithID(idString string) (*models.Article, error) {
	articleID, err := uuid.Parse(idString)
	if err != nil {
		return nil, err
	}

	article, err := s.repo.GetArticleWithID(articleID)
	if err != nil {
		return nil, err
	}

	return article, err
}

func (s *ArticlesService) UpdateArticle(authorID string, articleIDStr string, request models.UpdateArticleRequest) error {
	articleID, err := uuid.Parse(articleIDStr)
	if err != nil {
		return err
	}

	err = s.repo.UpdateArticle(authorID, articleID, request)
	if err != nil {
		return err
	}

	return nil
}
