package service

import (
	"fmt"
	"net"
	"time"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
)

type DatabaseService struct {
	repo *repository.DatabaseRepository
}

func NewDatabaseService() *DatabaseService {
	return &DatabaseService{repo: repository.NewDatabaseRepository(global.DB)}
}

func (s *DatabaseService) Create(item *model.DatabaseInstance) error {
	return s.repo.Create(item)
}

func (s *DatabaseService) List() ([]model.DatabaseInstance, error) {
	return s.repo.List()
}

func (s *DatabaseService) GetByID(id uint) (*model.DatabaseInstance, error) {
	return s.repo.GetByID(id)
}

func (s *DatabaseService) Update(item *model.DatabaseInstance) error {
	return s.repo.Update(item)
}

func (s *DatabaseService) Delete(id uint) error {
	return s.repo.Delete(id)
}

func (s *DatabaseService) TestConnection(item *model.DatabaseInstance) (string, error) {
	addr := fmt.Sprintf("%s:%d", item.Host, item.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("connection failed: %v", err)
	}
	conn.Close()
	return "Connection successful", nil
}
