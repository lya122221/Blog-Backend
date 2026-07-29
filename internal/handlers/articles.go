package handlers

import (
	"blog/internal/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ArticlesService interface {
	GetArticles(page int, limit int, tags []string) ([]models.Article, error)
}

type ArticlesHandler struct {
	service ArticlesService
}

func NewArticlesHandler(service ArticlesService) *ArticlesHandler {
	return &ArticlesHandler{service: service}
}

func (h *ArticlesHandler) GetArticlesHandler(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page <= 0 {
		page = 1
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit <= 0 || limit >= 100 {
		limit = 20
	}

	tags := c.QueryArray("tag")

	articles, err := h.service.GetArticles(page, limit, tags)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get articles"})
		return
	}

	c.JSON(http.StatusOK, articles)
}
