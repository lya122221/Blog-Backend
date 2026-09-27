package handlers

import (
	"blog/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

type InteractionsService interface {
	GetComments(articleIDStr string) ([]models.Comment, error)
	CreateComment(articleIDStr string, userID string, content string) error
	ToggleLike(articleIDStr string, userID string) (bool, int, error)
}

type InteractionsHandler struct {
	service InteractionsService
}

func NewInteractionsHandler(service InteractionsService) *InteractionsHandler {
	return &InteractionsHandler{service: service}
}

func (h *InteractionsHandler) GetCommentsHandler(c *gin.Context) {
	articleID := c.Param("id")

	comments, err := h.service.GetComments(articleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get comments"})
		return
	}

	c.JSON(http.StatusOK, comments)
}

func (h *InteractionsHandler) CreateCommentHandler(c *gin.Context) {
	var request models.CreateCommentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	articleID := c.Param("id")

	userID, exist := c.Get("userID")
	if !exist {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID is missing"})
		return
	}
	authorID, ok := userID.(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID type"})
		return
	}

	err := h.service.CreateComment(articleID, authorID, request.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create comment"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Comment created"})
}

func (h *InteractionsHandler) ToggleLikeHandler(c *gin.Context) {
	articleID := c.Param("id")

	userID, exist := c.Get("userID")
	if !exist {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID is missing"})
		return
	}
	authorID, ok := userID.(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID type"})
		return
	}

	liked, likesCount, err := h.service.ToggleLike(articleID, authorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle like"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"liked": liked, "likes_count": likesCount})
}
