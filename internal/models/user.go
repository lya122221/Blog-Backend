package models

type User struct {
	Email    string `json:"email" binding:"required,email"`
	Username string `json:"username" binding:"required"`
	PasswordHash string `json:"password" binding:"required"`
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
