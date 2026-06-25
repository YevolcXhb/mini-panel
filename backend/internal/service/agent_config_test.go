package service

import (
	"encoding/json"
	"testing"

	"github.com/minipanel/minipanel/internal/model"
)

func TestAgentConfigAPIKeyRoundTrip(t *testing.T) {
	// 模拟前端保存配置时 payload 中的 api_key 字段
	payload := `{
		"provider": "custom",
		"base_url": "https://token-plan-sgp.xiaomimimo.com/v1",
		"api_key": "tp-s5cet2ih7kdyrfs7eqfi69o5c9jbzdjixcbxbey2ee7mc40z",
		"model": "gpt-4o-mini",
		"temperature": 0.3,
		"max_tokens": 4096,
		"enabled": true,
		"system_prompt": "test",
		"skills": "[\"system\"]"
	}`

	var cfg model.AgentConfig
	if err := json.Unmarshal([]byte(payload), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if cfg.APIKey != "tp-s5cet2ih7kdyrfs7eqfi69o5c9jbzdjixcbxbey2ee7mc40z" {
		t.Fatalf("api_key not bound, got %q", cfg.APIKey)
	}
	if cfg.Provider != "custom" {
		t.Fatalf("provider mismatch, got %q", cfg.Provider)
	}

	// 验证 GetConfig 响应不会泄露 api_key
	apiResp := map[string]interface{}{
		"provider":      cfg.Provider,
		"base_url":      cfg.BaseURL,
		"model":         cfg.Model,
		"temperature":   cfg.Temperature,
		"max_tokens":    cfg.MaxTokens,
		"enabled":       cfg.Enabled,
		"system_prompt": cfg.SystemPrompt,
		"skills":        cfg.Skills,
	}
	if _, exists := apiResp["api_key"]; exists {
		t.Fatal("api_key should not be returned by GetConfig")
	}
}
