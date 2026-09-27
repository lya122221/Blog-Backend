package handlers

import (
	"auth/internal/models"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserService interface {
	Register(user *models.UserRegister) error
}

type UserHandler struct {
	service UserService
}

func NewUserHandler(service UserService) *UserHandler {
	return &UserHandler{service: service}
}

func (h *UserHandler) RegisterUser(c *gin.Context) {
	var user models.UserRegister
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Incorrect data" + err.Error()})
		return
	}

	if err := h.service.Register(&user); err != nil {
		if errors.Is(err, models.ErrUserExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, nil)
}
