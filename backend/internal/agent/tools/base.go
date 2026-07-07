package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/agent/compression"
	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/global"
)

const maxOutputLength = 4000

// lazyRefThreshold 超过此长度则注册 lazy-ref，返回预览+引用
const lazyRefThreshold = 4000

type ToolExecResult struct {
	Output string
	Error  string
}

type ToolResult struct {
	CallID   string
	Name     string
	Success  bool
	Result   string
	Error    string
	CacheKey string // 命中缓存时填充
	Cached   bool   // 是否来自缓存
}

type Tool interface {
	Name() string
	Description() string
	Parameters() []provider.ToolParam
	Execute(ctx context.Context, args map[string]interface{}) ToolExecResult
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.tools[normalizeName(t.Name())] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[normalizeName(name)]
	return t, ok
}

func (r *Registry) List() []Tool {
	var list []Tool
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

func normalizeName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

// truncateOutput 简单截断（用于引擎层兜底）
func truncateOutput(s string) string {
	if len(s) > maxOutputLength {
		return s[:maxOutputLength] + fmt.Sprintf("\n... [输出过长已截断，原长度 %d 字符]", len(s))
	}
	return s
}

// prepareOutput 对工具输出进行处理：超阈值时注册 lazy-ref，返回预览+引用。
// LLM 可通过 resolve_lazy_ref 工具按需取回完整内容。
func prepareOutput(s string) string {
	if len(s) <= lazyRefThreshold {
		return s
	}
	preview := s[:maxOutputLength]
	ref := compression.RegisterLazyRef(s)
	global.LOG.Infof("[Tools] 输出过长(len=%d)，注册 lazy-ref=%s", len(s), ref)
	return fmt.Sprintf("%s\n\n[lazy-ref:%s] 完整内容已缓存，可调用 resolve_lazy_ref 工具取回（hash=%s）",
		preview, ref, ref)
}

func (r *Registry) ToDefinitions() []provider.ToolDefinition {
	var defs []provider.ToolDefinition
	for _, t := range r.tools {
		params := map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
		properties := params["properties"].(map[string]interface{})
		var required []string
		for _, p := range t.Parameters() {
			prop := map[string]interface{}{
				"type":        p.Type,
				"description": p.Description,
			}
			properties[p.Name] = prop
			if p.Required {
				required = append(required, p.Name)
			}
		}
		if len(required) > 0 {
			params["required"] = required
		}

		defs = append(defs, provider.ToolDefinition{
			Type: "function",
			Function: struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				Parameters  map[string]interface{} `json:"parameters"`
			}{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  params,
			},
		})
	}
	return defs
}

type Executor struct {
	registry *Registry
}

func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

func (e *Executor) Execute(ctx context.Context, call provider.ToolCall) ToolResult {
	toolName := call.Function.Name
	tool, ok := e.registry.Get(toolName)
	if !ok {
		err := fmt.Sprintf("Tool '%s' not found", toolName)
		global.LOG.Warnf("[Tools] %s", err)
		return ToolResult{
			CallID:  call.ID,
			Name:    toolName,
			Success: false,
			Error:   err,
		}
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		errMsg := fmt.Sprintf("Failed to parse arguments: %v", err)
		global.LOG.Warnf("[Tools] %s args=%s", errMsg, call.Function.Arguments)
		return ToolResult{
			CallID:  call.ID,
			Name:    toolName,
			Success: false,
			Error:   errMsg,
		}
	}

	// 缓存检查：仅对实现 CacheableTool 接口且返回 true 的工具生效
	cacheKey := ""
	if cacheable, ok := tool.(CacheableTool); ok && cacheable.Cacheable() {
		cacheKey = CacheKey(toolName, call.Function.Arguments)
		if cached, hit := GetGlobalToolCache().Get(cacheKey); hit {
			global.LOG.Infof("[Tools] 缓存命中: %s key=%s", toolName, cacheKey)
			return ToolResult{
				CallID:   call.ID,
				Name:     toolName,
				Success:  true,
				Result:   cached,
				CacheKey: cacheKey,
				Cached:   true,
			}
		}
	}

	result := e.executeSafely(ctx, tool, args)
	result.CallID = call.ID
	result.Name = toolName
	result.CacheKey = cacheKey

	// 缓存写入：仅成功的查询类工具结果
	if cacheKey != "" && result.Success {
		GetGlobalToolCache().Set(cacheKey, result.Result, 5*time.Minute)
		global.LOG.Debugf("[Tools] 缓存写入: %s key=%s", toolName, cacheKey)
	}

	return result
}

func (e *Executor) executeSafely(ctx context.Context, tool Tool, args map[string]interface{}) (res ToolResult) {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("[Tools] tool %s panic: %v", tool.Name(), r)
			res.Success = false
			res.Error = fmt.Sprintf("Tool execution panic: %v", r)
		}
	}()

	execRes := tool.Execute(ctx, args)
	if execRes.Error != "" {
		return ToolResult{
			Success: false,
			Result:  prepareOutput(execRes.Output),
			Error:   execRes.Error,
		}
	}
	return ToolResult{
		Success: true,
		Result:  prepareOutput(execRes.Output),
	}
}

func (e *Executor) ExecuteAll(ctx context.Context, calls []provider.ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))
	for i, call := range calls {
		results[i] = e.Execute(ctx, call)
	}
	return results
}

func GetString(args map[string]interface{}, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func GetInt(args map[string]interface{}, key string) int {
	v, ok := args[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		var n int
		fmt.Sscanf(val, "%d", &n)
		return n
	default:
		return 0
	}
}

func GetInt64(args map[string]interface{}, key string) int64 {
	v, ok := args[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case int64:
		return val
	default:
		return 0
	}
}

func GetBool(args map[string]interface{}, key string) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.ToLower(val) == "true" || val == "1"
	case float64:
		return val != 0
	default:
		return false
	}
}

func GetStringArray(args map[string]interface{}, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []interface{}:
		res := make([]string, 0, len(arr))
		for _, item := range arr {
			res = append(res, fmt.Sprintf("%v", item))
		}
		return res
	case []string:
		return arr
	default:
		return nil
	}
}

func FormatJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func ErrorResult(format string, args ...interface{}) ToolExecResult {
	return ToolExecResult{
		Error: fmt.Sprintf(format, args...),
	}
}

func ErrorErr(err error) ToolExecResult {
	return ToolExecResult{
		Error: err.Error(),
	}
}

func SuccessResult(output string) ToolExecResult {
	return ToolExecResult{
		Output: output,
	}
}
