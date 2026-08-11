package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	// 解析符号链接（已存在的部分），防止通过链接绕过 /proc、/sys 等拦截
	resolved, err := evalPath(absPath)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}
	if !strings.HasPrefix(resolved, rootAbs) {
		return "", fmt.Errorf("path traversal detected")
	}
	if s.isDangerousPath(resolved) {
		return "", fmt.Errorf("access denied")
	}
	return resolved, nil
}

// evalPath 解析路径中已存在部分的符号链接；文件本身不存在时解析父目录。
func evalPath(p string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	}
	parent := filepath.Dir(p)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(p)), nil
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
		if path == d || strings.HasPrefix(path, d+string(filepath.Separator)) {
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
		_, err = io.Copy(writer, file)
		file.Close()
		return err
	})
}

// Rename 重命名文件或目录
func (s *FileService) Rename(oldPath, newName string) error {
	oldFull, err := s.resolvePath(oldPath)
	if err != nil {
		return err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" || newName == "." || newName == ".." ||
		strings.ContainsAny(newName, "/\\") {
		return fmt.Errorf("invalid file name")
	}
	newFull := filepath.Join(filepath.Dir(oldFull), newName)
	if s.isDangerousPath(newFull) {
		return fmt.Errorf("access denied")
	}
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

const (
	maxExtractEntries = 100000
	maxExtractTotal   = 8 << 30 // 8GB，防 zip 炸弹
)

// safeExtractPath 校验压缩包条目路径不会逃逸目标目录。
func safeExtractPath(destFull, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry contains invalid path: %s", name)
	}
	target := filepath.Join(destFull, clean)
	prefix := destFull + string(filepath.Separator)
	if target != destFull && !strings.HasPrefix(target, prefix) {
		return "", fmt.Errorf("archive entry escapes destination: %s", name)
	}
	return target, nil
}

// Extract 解压文件（防 zip-slip、符号链接与解压炸弹）。
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

	entries := 0
	var total int64
	checkLimits := func(size int64) error {
		entries++
		if entries > maxExtractEntries {
			return fmt.Errorf("压缩包条目数超过限制")
		}
		if total+size > maxExtractTotal {
			return fmt.Errorf("压缩包解压总大小超过限制")
		}
		return nil
	}

	ext := strings.ToLower(filepath.Ext(archiveFull))
	switch {
	case ext == ".zip":
		fi, _ := file.Stat()
		zr, err := zip.NewReader(file, fi.Size())
		if err != nil {
			return err
		}
		for _, f := range zr.File {
			target, err := safeExtractPath(destFull, f.Name)
			if err != nil {
				return err
			}
			mode := f.Mode()
			if mode&os.ModeSymlink != 0 {
				return fmt.Errorf("压缩包包含符号链接，已拒绝: %s", f.Name)
			}
			if f.FileInfo().IsDir() {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
				continue
			}
			if err := checkLimits(int64(f.UncompressedSize64)); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				rc.Close()
				return err
			}
			n, copyErr := io.Copy(out, rc)
			out.Close()
			rc.Close()
			if copyErr != nil {
				return copyErr
			}
			total += n
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
			if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeDir {
				// 拒绝符号链接、设备等特殊条目
				return fmt.Errorf("压缩包包含不支持的特殊条目: %s", hdr.Name)
			}
			target, err := safeExtractPath(destFull, hdr.Name)
			if err != nil {
				return err
			}
			if hdr.Typeflag == tar.TypeDir {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
				continue
			}
			if err := checkLimits(hdr.Size); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(out, tr)
			out.Close()
			if copyErr != nil {
				return copyErr
			}
			total += n
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

const (
	recycleDataName = "data"
	recycleMetaName = "meta.json"
)

type recycleMeta struct {
	Original string `json:"original"`
	IsDir    bool   `json:"is_dir"`
}

func recycleSuffix() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b)
}

// MoveToRecycle 移动文件到回收站（支持跨文件系统）
func (s *FileService) MoveToRecycle(path string) error {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return err
	}
	recycleDir := s.GetRecycleDir()
	if err := os.MkdirAll(recycleDir, 0755); err != nil {
		return err
	}
	itemDir := filepath.Join(recycleDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), recycleSuffix()))
	if err := os.MkdirAll(itemDir, 0700); err != nil {
		return err
	}

	dataDir := filepath.Join(itemDir, recycleDataName)
	if err := s.copyOrMove(fullPath, dataDir); err != nil {
		os.RemoveAll(itemDir)
		return err
	}
	metaBytes, _ := json.Marshal(recycleMeta{Original: fullPath, IsDir: info.IsDir()})
	if err := os.WriteFile(filepath.Join(itemDir, recycleMetaName), metaBytes, 0600); err != nil {
		os.RemoveAll(itemDir)
		return err
	}
	return os.RemoveAll(fullPath)
}

// copyOrMove 复制文件或目录到目标路径
func (s *FileService) copyOrMove(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if srcInfo.IsDir() {
		return filepath.Walk(src, func(path string, fi os.FileInfo, _ error) error {
			rel, _ := filepath.Rel(src, path)
			target := filepath.Join(dest, rel)
			if fi.IsDir() {
				return os.MkdirAll(target, fi.Mode())
			}
			return s.copyFile(path, target)
		})
	}
	os.MkdirAll(filepath.Dir(dest), 0755)
	return s.copyFile(src, dest)
}

// copyFile 复制单个文件
func (s *FileService) copyFile(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}
	return os.Chmod(dest, srcInfo.Mode())
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
		itemDir := filepath.Join(recycleDir, e.Name())
		if !e.IsDir() {
			// 兼容旧版扁平文件回收项
			files = append(files, FileInfo{
				Name:    e.Name(),
				Path:    itemDir,
				Size:    info.Size(),
				Mode:    info.Mode().String(),
				ModTime: info.ModTime().Unix(),
				IsDir:   false,
				IsLink:  false,
			})
			continue
		}
		metaBytes, err := os.ReadFile(filepath.Join(itemDir, recycleMetaName))
		if err != nil {
			continue
		}
		var meta recycleMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil || meta.Original == "" {
			continue
		}
		dataInfo, err := os.Stat(filepath.Join(itemDir, recycleDataName))
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    filepath.Base(meta.Original),
			Path:    itemDir,
			Size:    dataInfo.Size(),
			Mode:    dataInfo.Mode().String(),
			ModTime: dataInfo.ModTime().Unix(),
			IsDir:   meta.IsDir,
			IsLink:  false,
		})
	}
	return files, nil
}

// RestoreFromRecycle 从回收站恢复到原始路径
func (s *FileService) RestoreFromRecycle(recyclePath string) error {
	recycleDir := s.GetRecycleDir()
	fullPath := filepath.Join(recycleDir, recyclePath)
	clean := filepath.Clean(fullPath)
	if clean != recycleDir && !strings.HasPrefix(clean, recycleDir+string(filepath.Separator)) {
		return fmt.Errorf("invalid recycle path")
	}
	info, err := os.Stat(clean)
	if os.IsNotExist(err) {
		return fmt.Errorf("recycle item not found")
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return s.restoreLegacyRecycle(clean, filepath.Base(clean))
	}

	metaBytes, err := os.ReadFile(filepath.Join(clean, recycleMetaName))
	if err != nil {
		return fmt.Errorf("recycle item metadata not found")
	}
	var meta recycleMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil || meta.Original == "" || !filepath.IsAbs(meta.Original) {
		return fmt.Errorf("recycle item metadata invalid")
	}

	originalPath := meta.Original
	os.MkdirAll(filepath.Dir(originalPath), 0755)

	dest := originalPath
	if _, err := os.Stat(dest); err == nil {
		dest = originalPath + "_restored"
	}

	if err := s.copyOrMove(filepath.Join(clean, recycleDataName), dest); err != nil {
		return err
	}
	return os.RemoveAll(clean)
}

// restoreLegacyRecycle 兼容旧版扁平文件回收项（文件名编码了原始路径）。
func (s *FileService) restoreLegacyRecycle(fullPath, recyclePath string) error {
	baseName := recyclePath
	// 去掉末尾的 _timestamp（最后一段以 _ 开头且全为数字）
	lastUnderscore := strings.LastIndex(baseName, "_")
	if lastUnderscore > 0 {
		suffix := baseName[lastUnderscore+1:]
		if _, err := strconv.ParseInt(suffix, 10, 64); err == nil {
			baseName = baseName[:lastUnderscore]
		}
	}
	// _ 还原为 /
	originalPath := strings.ReplaceAll(baseName, "_", "/")

	// 确保原始路径的父目录存在
	os.MkdirAll(filepath.Dir(originalPath), 0755)

	// 如果目标已存在，追加 _restored 后缀
	dest := originalPath
	if _, err := os.Stat(dest); err == nil {
		dest = originalPath + "_restored"
	}

	// 使用 copy + remove 支持跨分区
	if err := s.copyOrMove(fullPath, dest); err != nil {
		return err
	}
	return os.RemoveAll(fullPath)
}

// ClearRecycleBin 清空回收站
func (s *FileService) ClearRecycleBin() error {
	return os.RemoveAll(s.GetRecycleDir())
}
