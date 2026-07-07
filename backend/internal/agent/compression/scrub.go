package compression

import "regexp"

// secretPatterns 常见敏感信息模式
var secretPatterns = []*regexp.Regexp{
	// OpenAI / Anthropic API keys
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	// Bearer tokens (JWT, opaque)
	regexp.MustCompile(`Bearer [A-Za-z0-9\-._~+/]+`),
	// GitHub / GitLab personal access tokens
	regexp.MustCompile(`(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,}`),
	// Generic secret/password/token assignment
	regexp.MustCompile(`(?i)(?:secret|password|token|api_key|apikey)\s*[:=]\s*["']?\S{16,}`),
}

// scrubReplacement 替换文本
const scrubReplacement = "[REDACTED_SECRET]"

// ScrubSensitiveData 从文本中脱敏常见敏感信息。
// 仅对摘要/预览文本操作；完整原始内容仍可通过 lazy-ref 取回。
func ScrubSensitiveData(text string) string {
	for _, p := range secretPatterns {
		text = p.ReplaceAllString(text, scrubReplacement)
	}
	return text
}
