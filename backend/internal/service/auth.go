package service

import (
	"fmt"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/middleware"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo *repository.UserRepository
}

func NewAuthService() *AuthService {
	return &AuthService{
		userRepo: repository.NewUserRepository(global.DB),
	}
}

func (s *AuthService) Login(username, password string) (string, error) {
	user, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return "", fmt.Errorf("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", fmt.Errorf("invalid credentials")
	}
	return middleware.GenerateToken(username)
}

func (s *AuthService) InitAdmin(username, password string) error {
	_, err := s.userRepo.GetByUsername(username)
	if err == nil {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.userRepo.Create(&model.User{
		Username: username,
		Password: string(hash),
		Role:     "admin",
	})
}
