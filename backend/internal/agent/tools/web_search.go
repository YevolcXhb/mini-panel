package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// WebSearchTool 网页搜索工具
type WebSearchTool struct{}

func NewWebSearchTool() *WebSearchTool { return &WebSearchTool{} }

func (t *WebSearchTool) Name() string        { return "web_search" }
func (t *WebSearchTool) Description() string { return "使用 DuckDuckGo 搜索引擎搜索网页内容。无需 API Key。" }
func (t *WebSearchTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "query", Type: "string", Description: "搜索关键词", Required: true},
		{Name: "num_results", Type: "integer", Description: "返回结果数量(1-10)", Required: false},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	query := GetString(args, "query")
	numResults := GetInt(args, "num_results")
	if numResults <= 0 || numResults > 10 {
		numResults = 5
	}
	if query == "" {
		return ErrorResult("query is required")
	}

	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return ErrorErr(err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ErrorErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrorResult("search request failed: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorErr(err)
	}

	results := parseDuckDuckGoResults(string(body), numResults)
	if len(results) == 0 {
		return SuccessResult("未找到搜索结果")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索 '%s' 的结果:\n\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   URL: %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}
	return SuccessResult(sb.String())
}

type searchResult struct {
	Title  string
	URL    string
	Snippet string
}

func parseDuckDuckGoResults(html string, limit int) []searchResult {
	var results []searchResult

	// 匹配结果标题和链接
	resultRe := regexp.MustCompile(`<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`<a[^>]+class="result__snippet"[^>]*>(.*?)</a>`)

	titleMatches := resultRe.FindAllStringSubmatch(html, -1)
	snippetMatches := snippetRe.FindAllStringSubmatch(html, -1)

	for i := 0; i < len(titleMatches) && i < limit; i++ {
		result := searchResult{
			Title: stripHTML(titleMatches[i][2]),
			URL:   titleMatches[i][1],
		}
		if i < len(snippetMatches) {
			result.Snippet = stripHTML(snippetMatches[i][1])
		}
		results = append(results, result)
	}
	return results
}

func stripHTML(s string) string {
	// 移除 HTML 标签
	re := regexp.MustCompile(`<[^>]+>`)
	s = re.ReplaceAllString(s, "")
	// 解码 HTML 实体
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&#39;", "'")
	return strings.TrimSpace(s)
}
