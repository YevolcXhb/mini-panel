package api

import (
	"bufio"
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
	logFile := filepath.Join(global.GetDataDir(), "logs", "panel.log")
	file, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, dto.Response{Code: 200, Data: []LogEntry{}})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	defer file.Close()

	// 支持按级别过滤，例如 ?levels=info,error,warning
	levelsParam := c.Query("levels")
	var levelsFilter map[string]bool
	if levelsParam != "" {
		levelsFilter = make(map[string]bool)
		for _, lv := range strings.Split(levelsParam, ",") {
			levelsFilter[strings.ToLower(strings.TrimSpace(lv))] = true
		}
	}

	// 支持限制返回行数 ?lines=200
	linesParam := c.DefaultQuery("lines", "500")
	maxLines, _ := strconv.Atoi(linesParam)
	if maxLines <= 0 || maxLines > 5000 {
		maxLines = 500
	}

	var entries []LogEntry
	scanner := bufio.NewScanner(file)
	// 防止单条日志过长（如包含命令输出）导致 scanner token too long
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	global.LOG.Infof("[Logs] reading log file %s with max token size 1MB", logFile)
	for scanner.Scan() {
		line := scanner.Text()
		entry := parseLogLine(line)
		if levelsFilter != nil && !levelsFilter[strings.ToLower(entry.Level)] {
			continue
		}
		entries = append(entries, entry)
		if len(entries) > maxLines*2 {
			entries = entries[len(entries)-maxLines*2:]
		}
	}

	// 只保留最后 maxLines 行
	if len(entries) > maxLines {
		entries = entries[len(entries)-maxLines:]
	}

	if err := scanner.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: entries})
}

func parseLogLine(line string) LogEntry {
	entry := LogEntry{Raw: line, Level: "info", Message: line}
	// logrus text format: time="..." level=info msg="..."
	if idx := strings.Index(line, `level=`); idx >= 0 {
		start := idx + 6
		end := strings.IndexAny(line[start:], ` 	"`)
		if end < 0 {
			end = len(line) - start
		}
		entry.Level = strings.ToLower(line[start : start+end])
	}
	if idx := strings.Index(line, `time="`); idx >= 0 {
		start := idx + 6
		end := strings.Index(line[start:], `"`)
		if end >= 0 {
			entry.Time = line[start : start+end]
		}
	}
	if idx := strings.Index(line, `msg="`); idx >= 0 {
		start := idx + 5
		// 找到对应的闭合引号（简单处理）
		end := strings.LastIndex(line[start:], `"`)
		if end >= 0 {
			entry.Message = line[start : start+end]
		}
	}
	return entry
}
