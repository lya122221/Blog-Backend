package services

import "blog/internal/models"

type ArticlesRepository interface {
	GetArticles(limit int, offset int, tags []string) ([]models.Article, error)
	CreateArticle(authorID, title, content string, tags []string) error
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
