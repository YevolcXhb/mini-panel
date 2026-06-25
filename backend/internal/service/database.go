package service

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	syscmd "github.com/minipanel/minipanel/internal/utils/cmd"
)

type DatabaseService struct {
	repo *repository.DatabaseRepository
}

func NewDatabaseService() *DatabaseService {
	return &DatabaseService{repo: repository.NewDatabaseRepository(global.DB)}
}

func (s *DatabaseService) Create(item *model.DatabaseInstance) error {
	if item.Port == 0 {
		item.Port = 3306
	}
	if item.Host == "" {
		item.Host = "127.0.0.1"
	}
	if item.Type == "" {
		item.Type = "mysql"
	}
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

func (s *DatabaseService) getMysqlArgs(item *model.DatabaseInstance, dbName string) []string {
	args := []string{
		fmt.Sprintf("-h%s", item.Host),
		fmt.Sprintf("-P%d", item.Port),
		fmt.Sprintf("-u%s", item.Username),
	}
	if item.Password != "" {
		args = append(args, fmt.Sprintf("-p%s", item.Password))
	}
	if dbName != "" {
		args = append(args, dbName)
	}
	return args
}

func (s *DatabaseService) TestConnection(item *model.DatabaseInstance) (string, error) {
	addr := fmt.Sprintf("%s:%d", item.Host, item.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("connection failed: %v", err)
	}
	conn.Close()

	if item.Type == "mysql" && item.Username != "" && syscmd.Which("mysql") {
		args := s.getMysqlArgs(item, "")
		args = append(args, "-e", "SELECT 1")
		mysqlCmd := exec.Command("mysql", args...)
		if out, err := mysqlCmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("mysql auth failed: %s: %v", string(out), err)
		}
		return "Connection successful", nil
	}
	return "TCP connection successful", nil
}

func (s *DatabaseService) CreateDatabase(item *model.DatabaseInstance, dbName string) error {
	if item.Type != "mysql" {
		return fmt.Errorf("only mysql database creation is supported currently")
	}
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found, please install mysql first")
	}
	args := s.getMysqlArgs(item, "")
	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName)
	args = append(args, "-e", query)
	mysqlCmd := exec.Command("mysql", args...)
	if out, err := mysqlCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create database failed: %s: %v", string(out), err)
	}
	return nil
}

func (s *DatabaseService) CreateUser(item *model.DatabaseInstance, username, password string, privDB string) error {
	if item.Type != "mysql" {
		return fmt.Errorf("only mysql user creation is supported currently")
	}
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found, please install mysql first")
	}
	if privDB == "" || privDB == "*" {
		privDB = "*.*"
	} else {
		privDB = fmt.Sprintf("`%s`.*", privDB)
	}
	queries := []string{
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", username, password),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON %s TO '%s'@'%%'", privDB, username),
		"FLUSH PRIVILEGES",
	}
	fullQuery := strings.Join(queries, "; ")
	args := s.getMysqlArgs(item, "")
	args = append(args, "-e", fullQuery)
	mysqlCmd := exec.Command("mysql", args...)
	if out, err := mysqlCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create user failed: %s: %v", string(out), err)
	}
	return nil
}

func (s *DatabaseService) ListDatabases(item *model.DatabaseInstance) ([]string, error) {
	if item.Type != "mysql" {
		return nil, fmt.Errorf("only mysql list databases is supported currently")
	}
	if !syscmd.Which("mysql") {
		return nil, fmt.Errorf("mysql client not found, please install mysql first")
	}
	args := s.getMysqlArgs(item, "")
	args = append(args, "-N", "-e", "SHOW DATABASES")
	mysqlCmd := exec.Command("mysql", args...)
	out, err := mysqlCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list databases failed: %s: %v", string(out), err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var dbs []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			dbs = append(dbs, line)
		}
	}
	return dbs, nil
}

func (s *DatabaseService) ListTables(item *model.DatabaseInstance) ([]string, error) {
	if item.Type != "mysql" || item.Database == "" {
		return nil, fmt.Errorf("please select a database first")
	}
	if !syscmd.Which("mysql") {
		return nil, fmt.Errorf("mysql client not found, please install mysql first")
	}
	args := s.getMysqlArgs(item, item.Database)
	args = append(args, "-N", "-e", "SHOW TABLES")
	mysqlCmd := exec.Command("mysql", args...)
	out, err := mysqlCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list tables failed: %s: %v", string(out), err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var tables []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			tables = append(tables, line)
		}
	}
	return tables, nil
}

func (s *DatabaseService) ChangePassword(item *model.DatabaseInstance, newPassword string) error {
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found")
	}
	return nil
}
