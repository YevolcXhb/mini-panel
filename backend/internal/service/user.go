package service

import (
	"fmt"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	repo *repository.UserRepository
}

func NewUserService() *UserService {
	return &UserService{repo: repository.NewUserRepository(global.DB)}
}

func (s *UserService) List() ([]model.User, error) {
	return s.repo.List()
}

func (s *UserService) Create(username, password, role string) (*model.User, error) {
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}
	if role == "" {
		role = "user"
	}
	if role != "admin" && role != "user" {
		return nil, fmt.Errorf("invalid role")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &model.User{
		Username: username,
		Password: string(hash),
		Role:     role,
	}
	if err := s.repo.Create(user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) Update(id uint, role string) error {
	user, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if role != "" {
		if role != "admin" && role != "user" {
			return fmt.Errorf("invalid role")
		}
		user.Role = role
	}
	return s.repo.Update(user)
}

func (s *UserService) ResetPassword(id uint, password string) error {
	if password == "" {
		return fmt.Errorf("password is required")
	}
	user, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hash)
	return s.repo.Update(user)
}

func (s *UserService) Delete(id uint) error {
	return s.repo.Delete(id)
}
