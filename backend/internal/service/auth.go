package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/middleware"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxLoginFailures = 5
	lockDuration     = 15 * time.Minute
	windowDuration   = 30 * time.Minute
)

type AuthService struct {
	userRepo    *repository.UserRepository
	attemptRepo *repository.LoginAttemptRepository
}

func NewAuthService() *AuthService {
	return &AuthService{
		userRepo:    repository.NewUserRepository(global.DB),
		attemptRepo: repository.NewLoginAttemptRepository(global.DB),
	}
}

func (s *AuthService) CheckLock(username, ip string) (*time.Time, error) {
	lock, err := s.attemptRepo.GetActiveLock(username, ip)
	if err != nil {
		return nil, nil
	}
	return lock.LockedUntil, nil
}

func (s *AuthService) Login(username, password, ip string) (string, string, []string, error) {
	lockUntil, _ := s.CheckLock(username, ip)
	if lockUntil != nil {
		remaining := int(time.Until(*lockUntil).Minutes())
		if remaining > 0 {
			s.recordAttempt(username, ip, false, nil)
			return "", "", nil, fmt.Errorf("account locked, try again in %d minutes", remaining)
		}
	}

	user, err := s.userRepo.GetByUsername(username)
	if err != nil {
		s.recordFailedAttempt(username, ip)
		return "", "", nil, fmt.Errorf("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		s.recordFailedAttempt(username, ip)
		return "", "", nil, fmt.Errorf("invalid credentials")
	}

	s.recordAttempt(username, ip, true, nil)
	perms := parsePermissions(user.Permissions)
	if user.Role == "admin" && len(perms) == 0 {
		perms = AdminFeatures()
	}
	token, err := middleware.GenerateToken(username, user.Role, user.ID, perms)
	if err != nil {
		return "", "", nil, err
	}
	return token, user.Role, perms, nil
}

func parsePermissions(raw string) []string {
	if raw == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil
	}
	return arr
}

func (s *AuthService) recordFailedAttempt(username, ip string) {
	failures, _ := s.attemptRepo.CountRecentFailures(username, ip, time.Now().Add(-windowDuration))
	var lockUntil *time.Time
	if failures+1 >= maxLoginFailures {
		t := time.Now().Add(lockDuration)
		lockUntil = &t
	}
	s.recordAttempt(username, ip, false, lockUntil)
}

func (s *AuthService) recordAttempt(username, ip string, success bool, lockedUntil *time.Time) {
	_ = s.attemptRepo.Create(&model.LoginAttempt{
		Username:    username,
		IP:          ip,
		Success:     success,
		LockedUntil: lockedUntil,
	})
}

func (s *AuthService) ChangePassword(username, oldPassword, newPassword string) error {
	if len(newPassword) < 6 {
		return fmt.Errorf("password must be at least 6 characters")
	}
	user, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return fmt.Errorf("incorrect old password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hash)
	return s.userRepo.Update(user)
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

func (s *AuthService) ResetPassword(username, newPassword string) error {
	user, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hash)
	return s.userRepo.Update(user)
}
