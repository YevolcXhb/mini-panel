package tools

import (
	"context"
	"fmt"

	"github.com/minipanel/minipanel/internal/agent/compression"
	"github.com/minipanel/minipanel/internal/agent/provider"
)

// ResolveLazyRefTool 让 LLM 按需取回被 lazy-ref 替换的大输出内容。
// 当微压缩将大工具输出替换为 [lazy-ref:hash] 时，LLM 可调用此工具取回完整内容。
type ResolveLazyRefTool struct{}

func NewResolveLazyRefTool() *ResolveLazyRefTool { return &ResolveLazyRefTool{} }

func (t *ResolveLazyRefTool) Name() string { return "resolve_lazy_ref" }

func (t *ResolveLazyRefTool) Description() string {
	return "Resolve a lazy reference hash to its full content. Use this when you need to re-examine a large tool output that was compressed. The hash is in the format [lazy-ref:hash]."
}

func (t *ResolveLazyRefTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "hash", Type: "string", Description: "The lazy-ref hash (12 characters)", Required: true},
	}
}

func (t *ResolveLazyRefTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	hash := GetString(args, "hash")
	if hash == "" {
		return ErrorResult("hash parameter is required")
	}
	content, ok := compression.ResolveLazyRef(hash)
	if !ok {
		return ErrorResult("lazy-ref '%s' not found or expired", hash)
	}
	return SuccessResult(fmt.Sprintf("[Resolved lazy-ref:%s]\n%s", hash, content))
}
