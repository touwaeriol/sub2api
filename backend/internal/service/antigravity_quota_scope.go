package service

import (
	"slices"
	"strings"
)

const antigravityQuotaScopesKey = "antigravity_quota_scopes"

// AntigravityQuotaScope 表示 Antigravity 的配额域
type AntigravityQuotaScope string

const (
	AntigravityQuotaScopeClaude      AntigravityQuotaScope = "claude"
	AntigravityQuotaScopeGeminiText  AntigravityQuotaScope = "gemini_text"
	AntigravityQuotaScopeGeminiImage AntigravityQuotaScope = "gemini_image"
)

// IsScopeSupported 检查给定的 scope 是否在分组支持的 scope 列表中
func IsScopeSupported(supportedScopes []string, scope AntigravityQuotaScope) bool {
	if len(supportedScopes) == 0 {
		// 未配置时默认全部支持
		return true
	}
	supported := slices.Contains(supportedScopes, string(scope))
	return supported
}

// ResolveAntigravityQuotaScope 根据模型名称解析配额域（导出版本）
func ResolveAntigravityQuotaScope(requestedModel string) (AntigravityQuotaScope, bool) {
	return resolveAntigravityQuotaScope(requestedModel)
}

// resolveAntigravityQuotaScope 根据模型名称解析配额域
func resolveAntigravityQuotaScope(requestedModel string) (AntigravityQuotaScope, bool) {
	model := normalizeAntigravityModelName(requestedModel)
	if model == "" {
		return "", false
	}
	switch {
	case strings.HasPrefix(model, "claude-"):
		return AntigravityQuotaScopeClaude, true
	case strings.HasPrefix(model, "gemini-"):
		if isImageGenerationModel(model) {
			return AntigravityQuotaScopeGeminiImage, true
		}
		return AntigravityQuotaScopeGeminiText, true
	default:
		return "", false
	}
}

func normalizeAntigravityModelName(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.TrimPrefix(normalized, "models/")
	return normalized
}

// IsSchedulableForModel 判断账号是否可调度（只检查模型级限流）
func (a *Account) IsSchedulableForModel(requestedModel string) bool {
	if a == nil {
		return false
	}
	if !a.IsSchedulable() {
		return false
	}
	// 只检查模型级限流，不再检查 Scope 级限流
	if a.isModelRateLimited(requestedModel) {
		return false
	}
	return true
}
