package models

import "errors"

var ErrUserExists = errors.New("user already exists")

type User struct {
	Email        string
	Username     string
	PasswordHash string
}

type UserRegister struct {
	Email    string `json:"email" binding:"required,email"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}
