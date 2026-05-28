package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime int64  `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
	IsLink  bool   `json:"is_link"`
}

type FileService struct {
	root string
}

func NewFileService() *FileService {
	return &FileService{root: "/"}
}

func (s *FileService) List(path string) ([]FileInfo, error) {
	fullPath := filepath.Join(s.root, path)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}
	var files []FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		f := FileInfo{
			Name:    e.Name(),
			Path:    filepath.Join(path, e.Name()),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Unix(),
			IsDir:   e.IsDir(),
			IsLink:  info.Mode()&os.ModeSymlink != 0,
		}
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})
	return files, nil
}

func (s *FileService) GetContent(path string) ([]byte, error) {
	fullPath := filepath.Join(s.root, path)
	if s.isDangerousPath(fullPath) {
		return nil, fmt.Errorf("access denied")
	}
	return os.ReadFile(fullPath)
}

func (s *FileService) Create(path string, isDir bool, content string) error {
	fullPath := filepath.Join(s.root, path)
	if s.isDangerousPath(fullPath) {
		return fmt.Errorf("access denied")
	}
	if isDir {
		return os.MkdirAll(fullPath, 0755)
	}
	return os.WriteFile(fullPath, []byte(content), 0644)
}

func (s *FileService) Update(path string, content string) error {
	fullPath := filepath.Join(s.root, path)
	if s.isDangerousPath(fullPath) {
		return fmt.Errorf("access denied")
	}
	return os.WriteFile(fullPath, []byte(content), 0644)
}

func (s *FileService) Delete(path string) error {
	fullPath := filepath.Join(s.root, path)
	if s.isDangerousPath(fullPath) {
		return fmt.Errorf("access denied")
	}
	return os.RemoveAll(fullPath)
}

func (s *FileService) Upload(path string, reader io.Reader) error {
	fullPath := filepath.Join(s.root, path)
	if s.isDangerousPath(fullPath) {
		return fmt.Errorf("access denied")
	}
	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, reader)
	return err
}

func (s *FileService) isDangerousPath(path string) bool {
	dangerous := []string{"/proc", "/sys", "/dev", "/boot"}
	for _, d := range dangerous {
		if strings.HasPrefix(path, d) {
			return true
		}
	}
	return false
}
