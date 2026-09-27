package models

import "errors"

var ErrUserExists = errors.New("user already exists")
var ErrInvalidCredentials = errors.New("invalid email or password")

type User struct {
	ID           string
	Email        string
	Username     string
	PasswordHash string
}

type UserRegister struct {
	Email    string `json:"email" binding:"required,email"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UserLogin struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}
