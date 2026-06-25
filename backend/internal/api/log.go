package api

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
)

type LogAPI struct{}

func NewLogAPI() *LogAPI {
	return &LogAPI{}
}

type LogEntry struct {
	Level   string `json:"level"`
	Time    string `json:"time"`
	Message string `json:"message"`
	Raw     string `json:"raw"`
}

func (a *LogAPI) List(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("[Logs] panic: %v", r)
			c.JSON(http.StatusOK, dto.Response{Code: 200, Data: []LogEntry{}, Message: fmt.Sprintf("日志接口错误: %v", r)})
		}
	}()

	logFile := filepath.Join(global.GetDataDir(), "logs", "panel.log")
	global.LOG.Infof("[Logs] request levels=%s lines=%s file=%s", c.Query("levels"), c.Query("lines"), logFile)

	file, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			global.LOG.Infof("[Logs] log file not found, return empty")
			c.JSON(http.StatusOK, dto.Response{Code: 200, Data: []LogEntry{}})
			return
		}
		global.LOG.Errorf("[Logs] open file failed: %v", err)
		c.JSON(http.StatusOK, dto.Response{Code: 200, Data: []LogEntry{}, Message: "打开日志文件失败: " + err.Error()})
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		global.LOG.Errorf("[Logs] stat file failed: %v", err)
		c.JSON(http.StatusOK, dto.Response{Code: 200, Data: []LogEntry{}, Message: "获取日志文件信息失败: " + err.Error()})
		return
	}
	global.LOG.Infof("[Logs] file size=%d", stat.Size())

	levelsParam := c.Query("levels")
	var levelsFilter map[string]bool
	if levelsParam != "" {
		levelsFilter = make(map[string]bool)
		for _, lv := range strings.Split(levelsParam, ",") {
			trimmed := strings.TrimSpace(lv)
			if trimmed != "" {
				levelsFilter[strings.ToLower(trimmed)] = true
			}
		}
	}

	linesParam := c.DefaultQuery("lines", "500")
	maxLines, _ := strconv.Atoi(linesParam)
	if maxLines <= 0 || maxLines > 5000 {
		maxLines = 500
	}

	var entries []LogEntry
	var lineCount int
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		lineCount++
		line := scanner.Text()
		if len(line) > 10*1024*1024 {
			global.LOG.Warnf("[Logs] skip oversized line %d, length=%d", lineCount, len(line))
			continue
		}
		entry := parseLogLine(line)
		if levelsFilter != nil && !levelsFilter[strings.ToLower(entry.Level)] {
			continue
		}
		entries = append(entries, entry)
		if len(entries) > maxLines*2 {
			entries = entries[len(entries)-maxLines*2:]
		}
	}

	if err := scanner.Err(); err != nil {
		global.LOG.Errorf("[Logs] scanner error after %d lines: %v", lineCount, err)
	}

	if len(entries) > maxLines {
		entries = entries[len(entries)-maxLines:]
	}

	global.LOG.Infof("[Logs] return entries=%d total_scanned=%d", len(entries), lineCount)
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: entries})
}

func parseLogLine(line string) LogEntry {
	entry := LogEntry{Raw: line, Level: "info", Message: line}
	if len(line) == 0 {
		return entry
	}
	// logrus text format: time="..." level=info msg="..."
	if idx := strings.Index(line, `level=`); idx >= 0 {
		start := idx + 6
		if start < len(line) {
			rest := line[start:]
			end := strings.IndexAny(rest, ` 	"`+"\n\r")
			if end < 0 {
				end = len(rest)
			}
			if end > 0 {
				entry.Level = strings.ToLower(rest[:end])
			}
		}
	}
	if idx := strings.Index(line, `time="`); idx >= 0 {
		start := idx + 6
		if start < len(line) {
			rest := line[start:]
			end := strings.Index(rest, `"`)
			if end > 0 {
				entry.Time = rest[:end]
			}
		}
	}
	if idx := strings.Index(line, `msg="`); idx >= 0 {
		start := idx + 5
		if start < len(line) {
			rest := line[start:]
			end := strings.LastIndex(rest, `"`)
			if end > 0 {
				entry.Message = rest[:end]
			}
		}
	}
	return entry
}
