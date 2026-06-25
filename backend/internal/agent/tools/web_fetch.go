package tools

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// WebFetchTool 网页抓取工具
type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool { return &WebFetchTool{} }

func (t *WebFetchTool) Name() string { return "web_fetch" }
func (t *WebFetchTool) Description() string {
	return "抓取指定 URL 的网页内容，返回纯文本正文。"
}
func (t *WebFetchTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "url", Type: "string", Description: "要抓取的网页 URL", Required: true},
		{Name: "max_length", Type: "integer", Description: "最大返回字符数(默认10000)", Required: false},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	pageURL := GetString(args, "url")
	maxLength := GetInt(args, "max_length")
	if maxLength <= 0 {
		maxLength = 10000
	}
	if pageURL == "" {
		return ErrorResult("url is required")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return ErrorErr(err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ErrorErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrorResult("fetch failed: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorErr(err)
	}

	text := extractTextFromHTML(string(body))
	if len(text) > maxLength {
		text = text[:maxLength] + "\n\n[内容已截断，超过最大长度限制]"
	}

	if text == "" {
		return SuccessResult("无法提取网页正文内容")
	}
	return SuccessResult(text)
}

func extractTextFromHTML(html string) string {
	// 移除 script 和 style 标签及其内容
	scriptRe := regexp.MustCompile(`(?s)<script.*?</script>`)
	styleRe := regexp.MustCompile(`(?s)<style.*?</style>`)
	html = scriptRe.ReplaceAllString(html, "")
	html = styleRe.ReplaceAllString(html, "")

	// 将常见块级标签替换为换行
	blockRe := regexp.MustCompile(`(?i)</?(p|div|br|h[1-6]|li|tr|pre|blockquote)[^>]*>`)
	html = blockRe.ReplaceAllString(html, "\n")

	// 移除所有 HTML 标签
	tagRe := regexp.MustCompile(`<[^>]+>`)
	html = tagRe.ReplaceAllString(html, "")

	// 解码 HTML 实体
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// 合并多余空行
	lines := strings.Split(html, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}
