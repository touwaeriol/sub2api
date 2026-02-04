package service

import (
	"strings"
	"time"
)

const modelRateLimitsKey = "model_rate_limits"

// 模型限流 scope 常量
const (
	modelRateLimitScopeClaudeSonnet = "claude_sonnet"
	modelRateLimitScopeClaudeOpus   = "claude_opus"
	modelRateLimitScopeClaudeHaiku  = "claude_haiku"
	modelRateLimitScopeGeminiFlash  = "gemini_flash"
	modelRateLimitScopeGeminiPro    = "gemini_pro"
)

// resolveModelRateLimitScope 将请求的模型名映射到限流 scope
// 返回 scope 和是否成功映射
//
// 映射逻辑：
// - 服务器返回的模型名（如 claude-sonnet-4-5）会直接存储
// - 客户端请求的模型名（如 claude-3-5-sonnet）需要映射到相同的 scope
// - 使用模型类型（sonnet/opus/haiku/flash/pro）作为 scope，而不是具体版本号
func resolveModelRateLimitScope(requestedModel string) (string, bool) {
	model := strings.ToLower(strings.TrimSpace(requestedModel))
	if model == "" {
		return "", false
	}
	model = strings.TrimPrefix(model, "models/")

	// Claude 模型映射
	// 客户端: claude-3-5-sonnet, claude-sonnet-4 等
	// 服务器: claude-sonnet-4-5 等
	if strings.Contains(model, "sonnet") {
		return modelRateLimitScopeClaudeSonnet, true
	}
	if strings.Contains(model, "opus") {
		return modelRateLimitScopeClaudeOpus, true
	}
	if strings.Contains(model, "haiku") {
		return modelRateLimitScopeClaudeHaiku, true
	}

	// Gemini 模型映射
	// flash 系列
	if strings.Contains(model, "flash") {
		return modelRateLimitScopeGeminiFlash, true
	}
	// pro 系列（gemini-3-pro-low, gemini-3-pro-high 等）
	if strings.HasPrefix(model, "gemini") && strings.Contains(model, "pro") {
		return modelRateLimitScopeGeminiPro, true
	}

	return "", false
}

func (a *Account) isModelRateLimited(requestedModel string) bool {
	// 1. 使用账户的模型映射获取上游实际使用的模型 ID
	mapped := a.GetMappedModel(requestedModel)
	if resetAt := a.modelRateLimitResetAt(mapped); resetAt != nil && time.Now().Before(*resetAt) {
		return true
	}
	// 2. 回退到旧格式的 scope 键（兼容老数据）
	if scope, ok := resolveModelRateLimitScope(requestedModel); ok {
		if resetAt := a.modelRateLimitResetAt(scope); resetAt != nil && time.Now().Before(*resetAt) {
			return true
		}
	}
	return false
}

// GetModelRateLimitRemainingTime 获取模型限流剩余时间
// 返回 0 表示未限流或已过期
func (a *Account) GetModelRateLimitRemainingTime(requestedModel string) time.Duration {
	if a == nil {
		return 0
	}
	// 1. 使用账户的模型映射获取上游实际使用的模型 ID
	mapped := a.GetMappedModel(requestedModel)
	if resetAt := a.modelRateLimitResetAt(mapped); resetAt != nil {
		if remaining := time.Until(*resetAt); remaining > 0 {
			return remaining
		}
	}
	// 2. 回退到旧格式的 scope 键（兼容老数据）
	if scope, ok := resolveModelRateLimitScope(requestedModel); ok {
		if resetAt := a.modelRateLimitResetAt(scope); resetAt != nil {
			if remaining := time.Until(*resetAt); remaining > 0 {
				return remaining
			}
		}
	}
	return 0
}

func (a *Account) modelRateLimitResetAt(scope string) *time.Time {
	if a == nil || a.Extra == nil || scope == "" {
		return nil
	}
	rawLimits, ok := a.Extra[modelRateLimitsKey].(map[string]any)
	if !ok {
		return nil
	}
	rawLimit, ok := rawLimits[scope].(map[string]any)
	if !ok {
		return nil
	}
	resetAtRaw, ok := rawLimit["rate_limit_reset_at"].(string)
	if !ok || strings.TrimSpace(resetAtRaw) == "" {
		return nil
	}
	resetAt, err := time.Parse(time.RFC3339, resetAtRaw)
	if err != nil {
		return nil
	}
	return &resetAt
}
