package service

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/minipanel/minipanel/internal/model"
)

func TestRestoreFilesBackup(t *testing.T) {
	src := t.TempDir()
	sub := filepath.Join(src, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	// 用与 backupFiles 相同的结构生成 zip（相对源目录的条目）
	zipPath := filepath.Join(t.TempDir(), "backup.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, rel := range []string{"a.txt", "sub/b.txt"} {
		w, err := zw.Create(rel)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(src, rel))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// 删除源文件后恢复
	if err := os.Remove(filepath.Join(src, "a.txt")); err != nil {
		t.Fatal(err)
	}
	svc := &BackupService{}
	task := &model.BackupTask{Type: "files", SourcePath: src}
	rec := &model.BackupRecord{FilePath: zipPath}
	if err := svc.restoreFilesBackup(task, rec); err != nil {
		t.Fatalf("restoreFilesBackup failed: %v", err)
	}

	for _, rel := range []string{"a.txt", "sub/b.txt"} {
		data, err := os.ReadFile(filepath.Join(src, rel))
		if err != nil {
			t.Fatalf("restored file %s missing: %v", rel, err)
		}
		if len(data) == 0 {
			t.Fatalf("restored file %s is empty", rel)
		}
	}
}
