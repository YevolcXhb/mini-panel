package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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

func (s *FileService) resolvePath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("invalid path")
	}
	fullPath := filepath.Join(s.root, cleaned)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}
	rootAbs, _ := filepath.Abs(s.root)
	if !strings.HasPrefix(absPath, rootAbs) {
		return "", fmt.Errorf("path traversal detected")
	}
	if s.isDangerousPath(absPath) {
		return "", fmt.Errorf("access denied")
	}
	return absPath, nil
}

func (s *FileService) ResolvePath(path string) (string, error) {
	return s.resolvePath(path)
}

func (s *FileService) List(path string) ([]FileInfo, error) {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return nil, err
	}
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
		isLink := info.Mode()&os.ModeSymlink != 0
		// 对符号链接跟随，获取真实文件信息以正确判断类型/大小
		if isLink {
			followPath := filepath.Join(fullPath, e.Name())
			if realInfo, err := os.Stat(followPath); err == nil {
				info = realInfo
			}
		}
		f := FileInfo{
			Name:    e.Name(),
			Path:    filepath.Join(path, e.Name()),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Unix(),
			IsDir:   info.IsDir(),
			IsLink:  isLink,
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
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(fullPath)
}

func (s *FileService) Create(path string, isDir bool, content string) error {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	if isDir {
		return os.MkdirAll(fullPath, 0755)
	}
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(content), 0644)
}

func (s *FileService) Update(path string, content string) error {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(content), 0644)
}

func (s *FileService) Delete(path string) error {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(fullPath)
}

func (s *FileService) Upload(path string, reader io.Reader) error {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return err
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

// CreateZip 流式压缩目录为 zip
func (s *FileService) CreateZip(dirPath string, writer io.Writer) error {
	fullPath, err := s.resolvePath(dirPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("path not found: %s", dirPath)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dirPath)
	}

	zw := zip.NewWriter(writer)
	defer zw.Close()

	baseDir := filepath.Dir(fullPath)
	return filepath.Walk(fullPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		header.Method = zip.Deflate

		if fi.IsDir() {
			header.Name += "/"
			_, err := zw.CreateHeader(header)
			return err
		}

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
}

// Rename 重命名文件或目录
func (s *FileService) Rename(oldPath, newName string) error {
	oldFull, err := s.resolvePath(oldPath)
	if err != nil {
		return err
	}
	newFull := filepath.Join(filepath.Dir(oldFull), newName)
	return os.Rename(oldFull, newFull)
}

// Chmod 修改文件权限
func (s *FileService) Chmod(path string, mode os.FileMode, recursive bool) error {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	if recursive {
		return filepath.Walk(fullPath, func(p string, _ os.FileInfo, _ error) error {
			return os.Chmod(p, mode)
		})
	}
	return os.Chmod(fullPath, mode)
}

// Compress 压缩文件或目录
func (s *FileService) Compress(paths []string, outputPath, format string) error {
	outputFull, err := s.resolvePath(outputPath)
	if err != nil {
		return err
	}
	// 收集所有文件
	type entry struct {
		fullPath string
		relPath  string
	}
	var entries []entry
	for _, p := range paths {
		fullPath, err := s.resolvePath(p)
		if err != nil {
			return err
		}
		baseDir := filepath.Dir(fullPath)
		filepath.Walk(fullPath, func(walkPath string, fi os.FileInfo, _ error) error {
			relPath, _ := filepath.Rel(baseDir, walkPath)
			entries = append(entries, entry{fullPath: walkPath, relPath: relPath})
			return nil
		})
	}

	outFile, err := os.Create(outputFull)
	if err != nil {
		return err
	}
	defer outFile.Close()

	switch format {
	case "tar.gz":
		gw := gzip.NewWriter(outFile)
		defer gw.Close()
		tw := tar.NewWriter(gw)
		defer tw.Close()
		for _, e := range entries {
			fi, err := os.Stat(e.fullPath)
			if err != nil {
				continue
			}
			hdr, err := tar.FileInfoHeader(fi, "")
			if err != nil {
				continue
			}
			hdr.Name = filepath.ToSlash(e.relPath)
			if err := tw.WriteHeader(hdr); err != nil {
				continue
			}
			if !fi.IsDir() {
				f, _ := os.Open(e.fullPath)
				if f != nil {
					io.Copy(tw, f)
					f.Close()
				}
			}
		}
	default: // zip
		zw := zip.NewWriter(outFile)
		defer zw.Close()
		dirCache := make(map[string]bool)
		for _, e := range entries {
			fi, err := os.Stat(e.fullPath)
			if err != nil {
				continue
			}
			hdr, err := zip.FileInfoHeader(fi)
			if err != nil {
				continue
			}
			hdr.Name = filepath.ToSlash(e.relPath)
			hdr.Method = zip.Deflate
			if fi.IsDir() {
				hdr.Name += "/"
				dirCache[hdr.Name] = true
				zw.CreateHeader(hdr)
			} else {
				// 确保父目录已创建
				parent := filepath.ToSlash(filepath.Dir(e.relPath))
				if parent != "." && !dirCache[parent+"/"] {
					parentHdr := &zip.FileHeader{Name: parent + "/", Method: zip.Deflate}
					parentHdr.SetMode(0755 | os.ModeDir)
					zw.CreateHeader(parentHdr)
					dirCache[parent+"/"] = true
				}
				w, _ := zw.CreateHeader(hdr)
				if w != nil {
					f, _ := os.Open(e.fullPath)
					if f != nil {
						io.Copy(w, f)
						f.Close()
					}
				}
			}
		}
	}
	return nil
}

// Extract 解压文件
func (s *FileService) Extract(archivePath, destDir string) error {
	archiveFull, err := s.resolvePath(archivePath)
	if err != nil {
		return err
	}
	destFull, err := s.resolvePath(destDir)
	if err != nil {
		return err
	}

	file, err := os.Open(archiveFull)
	if err != nil {
		return err
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(archiveFull))
	switch {
	case ext == ".zip":
		fi, _ := file.Stat()
		zr, err := zip.NewReader(file, fi.Size())
		if err != nil {
			return err
		}
		for _, f := range zr.File {
			target := filepath.Join(destFull, f.Name)
			if f.FileInfo().IsDir() {
				os.MkdirAll(target, 0755)
				continue
			}
			os.MkdirAll(filepath.Dir(target), 0755)
			rc, _ := f.Open()
			out, _ := os.Create(target)
			if rc != nil && out != nil {
				io.Copy(out, rc)
				out.Close()
				rc.Close()
			}
		}
	case strings.HasSuffix(archiveFull, ".tar.gz") || strings.HasSuffix(archiveFull, ".tgz"):
		gzr, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer gzr.Close()
		tr := tar.NewReader(gzr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			target := filepath.Join(destFull, hdr.Name)
			switch hdr.Typeflag {
			case tar.TypeDir:
				os.MkdirAll(target, 0755)
			case tar.TypeReg:
				os.MkdirAll(filepath.Dir(target), 0755)
				out, _ := os.Create(target)
				if out != nil {
					io.Copy(out, tr)
					out.Close()
				}
			}
		}
	default:
		return fmt.Errorf("unsupported archive format: %s", ext)
	}
	return nil
}

// CopyFile 复制文件或目录
func (s *FileService) CopyFile(srcPath, destPath string) error {
	srcFull, err := s.resolvePath(srcPath)
	if err != nil {
		return err
	}
	destFull, err := s.resolvePath(destPath)
	if err != nil {
		return err
	}
	srcInfo, err := os.Stat(srcFull)
	if err != nil {
		return err
	}
	if srcInfo.IsDir() {
		return filepath.Walk(srcFull, func(path string, fi os.FileInfo, _ error) error {
			rel, _ := filepath.Rel(srcFull, path)
			target := filepath.Join(destFull, rel)
			if fi.IsDir() {
				return os.MkdirAll(target, fi.Mode())
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, fi.Mode())
		})
	}
	data, err := os.ReadFile(srcFull)
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(destFull), 0755)
	return os.WriteFile(destFull, data, srcInfo.Mode())
}

// MoveFile 移动文件或目录
func (s *FileService) MoveFile(srcPath, destPath string) error {
	srcFull, err := s.resolvePath(srcPath)
	if err != nil {
		return err
	}
	destFull, err := s.resolvePath(destPath)
	if err != nil {
		return err
	}
	return os.Rename(srcFull, destFull)
}

// ListWithSearch 列出目录内容，支持搜索过滤
func (s *FileService) ListWithSearch(dirPath, search string) ([]FileInfo, error) {
	files, err := s.List(dirPath)
	if err != nil {
		return nil, err
	}
	if search == "" {
		return files, nil
	}
	var filtered []FileInfo
	for _, f := range files {
		matched, _ := filepath.Match(strings.ToLower(search), strings.ToLower(f.Name))
		if matched || strings.Contains(strings.ToLower(f.Name), strings.ToLower(search)) {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}

// GetRecycleDir 获取回收站目录
func (s *FileService) GetRecycleDir() string {
	return filepath.Join(s.root, "tmp", ".minipanel_recycle")
}

// MoveToRecycle 移动文件到回收站
func (s *FileService) MoveToRecycle(path string) error {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	recycleDir := s.GetRecycleDir()
	if err := os.MkdirAll(recycleDir, 0755); err != nil {
		return err
	}
	// 保留原始路径结构以防冲突
	safeName := strings.ReplaceAll(fullPath, "/", "_")
	safeName = strings.ReplaceAll(safeName, string(filepath.Separator), "_")
	ts := strconv.FormatInt(time.Now().UnixNano(), 10)
	dest := filepath.Join(recycleDir, safeName+"_"+ts)
	return os.Rename(fullPath, dest)
}

// ListRecycleBin 列出回收站内容
func (s *FileService) ListRecycleBin() ([]FileInfo, error) {
	recycleDir := s.GetRecycleDir()
	if _, err := os.Stat(recycleDir); os.IsNotExist(err) {
		return []FileInfo{}, nil
	}
	entries, err := os.ReadDir(recycleDir)
	if err != nil {
		return nil, err
	}
	var files []FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    e.Name(),
			Path:    filepath.Join(recycleDir, e.Name()),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Unix(),
			IsDir:   e.IsDir(),
			IsLink:  false,
		})
	}
	return files, nil
}

// RestoreFromRecycle 从回收站恢复
func (s *FileService) RestoreFromRecycle(recyclePath string) error {
	// 回收站路径格式：{recycleDir}/{safeName}_{timestamp}
	// 恢复时尝试恢复原始名称
	recycleDir := s.GetRecycleDir()
	fullPath := filepath.Join(recycleDir, recyclePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("recycle item not found")
	}
	// 恢复到一个临时名，用户需手动处理
	restoreName := strings.TrimSuffix(recyclePath, filepath.Ext(recyclePath))
	// 简单恢复：移到 /tmp/restored_name
	destDir := filepath.Join(s.root, "tmp", "restored")
	os.MkdirAll(destDir, 0755)
	dest := filepath.Join(destDir, restoreName)
	return os.Rename(fullPath, dest)
}

// ClearRecycleBin 清空回收站
func (s *FileService) ClearRecycleBin() error {
	return os.RemoveAll(s.GetRecycleDir())
}
