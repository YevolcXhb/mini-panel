package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/service"
)

// FileReadTool 读取文件
type FileReadTool struct{}

func NewFileReadTool() *FileReadTool { return &FileReadTool{} }

func (t *FileReadTool) Name() string        { return "read_file" }
func (t *FileReadTool) Description() string { return "读取指定文件的内容。" }
func (t *FileReadTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "path", Type: "string", Description: "文件绝对路径", Required: true},
		{Name: "limit", Type: "integer", Description: "最大读取行数，默认 200", Required: false},
	}
}
func (t *FileReadTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	svc := service.NewFileService()
	path := GetString(args, "path")
	limit := GetInt(args, "limit")
	if limit <= 0 {
		limit = 200
	}
	contentBytes, err := svc.GetContent(path)
	if err != nil {
		return ErrorErr(err)
	}
	content := string(contentBytes)
	lines := strings.Split(content, "\n")
	if len(lines) > limit {
		lines = lines[:limit]
		content = strings.Join(lines, "\n") + fmt.Sprintf("\n... (已截断，共 %d 行)", len(lines))
	}
	return SuccessResult(content)
}

// FileListTool 列出目录
type FileListTool struct{}

func NewFileListTool() *FileListTool { return &FileListTool{} }

func (t *FileListTool) Name() string        { return "list_files" }
func (t *FileListTool) Description() string { return "列出指定目录下的文件和子目录。" }
func (t *FileListTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "path", Type: "string", Description: "目录绝对路径", Required: true},
	}
}
func (t *FileListTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	svc := service.NewFileService()
	path := GetString(args, "path")
	files, err := svc.List(path)
	if err != nil {
		return ErrorErr(err)
	}
	var sb strings.Builder
	for _, f := range files {
		ftype := "文件"
		if f.IsDir {
			ftype = "目录"
		}
		size := fmt.Sprintf("%d B", f.Size)
		if f.Size > 1024*1024 {
			size = fmt.Sprintf("%.2f MB", float64(f.Size)/1024/1024)
		} else if f.Size > 1024 {
			size = fmt.Sprintf("%.2f KB", float64(f.Size)/1024)
		}
		modTime := time.Unix(f.ModTime, 0).Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf("%s | %s | %s | %s\n", f.Name, ftype, size, modTime))
	}
	return SuccessResult(sb.String())
}

// FileWriteTool 写入文件
type FileWriteTool struct{}

func NewFileWriteTool() *FileWriteTool { return &FileWriteTool{} }

func (t *FileWriteTool) Name() string        { return "write_file" }
func (t *FileWriteTool) Description() string { return "创建新文件或覆盖写入文件内容。" }
func (t *FileWriteTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "path", Type: "string", Description: "文件绝对路径", Required: true},
		{Name: "content", Type: "string", Description: "文件内容", Required: true},
	}
}
func (t *FileWriteTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	svc := service.NewFileService()
	path := GetString(args, "path")
	content := GetString(args, "content")
	if err := svc.Update(path, content); err != nil {
		if err := svc.Create(path, false, content); err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(fmt.Sprintf("文件 %s 已创建", path))
	}
	return SuccessResult(fmt.Sprintf("文件 %s 已更新", path))
}
