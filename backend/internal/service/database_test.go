package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/minipanel/minipanel/internal/model"
)

func TestSQLiteFilePath(t *testing.T) {
	s := &DatabaseService{}
	dir := t.TempDir()
	item := &model.DatabaseInstance{Database: dir}

	path, err := s.sqliteFilePath(item, "app")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "app.db" {
		t.Errorf("sqliteFilePath = %q, want file app.db", path)
	}

	path, err = s.sqliteFilePath(item, "data.db")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "data.db" {
		t.Errorf("sqliteFilePath = %q, want file data.db", path)
	}

	if _, err := s.sqliteFilePath(item, "../evil"); err == nil {
		t.Error("expected path traversal to be rejected")
	}
	if _, err := s.sqliteFilePath(item, "/abs/path"); err == nil {
		t.Error("expected absolute path to be rejected")
	}
}

func TestSQLiteDataDirCreates(t *testing.T) {
	s := &DatabaseService{}
	dir := filepath.Join(t.TempDir(), "nested", "sqlite")
	item := &model.DatabaseInstance{Database: dir}

	got, err := s.sqliteDataDir(item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(filepath.Clean(got), filepath.Clean(dir)) {
		t.Errorf("sqliteDataDir = %q, want %q", got, dir)
	}
}
