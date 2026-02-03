package service

import (
	"testing"
	"time"
)

func TestResolveModelRateLimitScope(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		expectedScope string
		expectedOK    bool
	}{
		// Claude sonnet 变体
		{"claude-sonnet-4-5", "claude-sonnet-4-5", modelRateLimitScopeClaudeSonnet, true},
		{"claude-3-5-sonnet", "claude-3-5-sonnet", modelRateLimitScopeClaudeSonnet, true},
		{"claude-3-5-sonnet-20241111", "claude-3-5-sonnet-20241111", modelRateLimitScopeClaudeSonnet, true},

		// Claude opus 变体
		{"claude-opus-4-5", "claude-opus-4-5", modelRateLimitScopeClaudeOpus, true},
		{"claude-opus-4-5-thinking", "claude-opus-4-5-thinking", modelRateLimitScopeClaudeOpus, true},

		// Claude haiku 变体
		{"claude-haiku-4-5", "claude-haiku-4-5", modelRateLimitScopeClaudeHaiku, true},
		{"claude-3-haiku", "claude-3-haiku", modelRateLimitScopeClaudeHaiku, true},

		// Gemini flash 变体
		{"gemini-3-flash", "gemini-3-flash", modelRateLimitScopeGeminiFlash, true},
		{"gemini-2.5-flash", "gemini-2.5-flash", modelRateLimitScopeGeminiFlash, true},

		// Gemini pro 变体
		{"gemini-3-pro-high", "gemini-3-pro-high", modelRateLimitScopeGeminiPro, true},
		{"gemini-3-pro-low", "gemini-3-pro-low", modelRateLimitScopeGeminiPro, true},
		{"gemini-2.5-pro", "gemini-2.5-pro", modelRateLimitScopeGeminiPro, true},

		// 带 models/ 前缀
		{"models/claude-sonnet-4-5", "models/claude-sonnet-4-5", modelRateLimitScopeClaudeSonnet, true},

		// 不支持的模型
		{"gpt-4", "gpt-4", "", false},
		{"empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, ok := resolveModelRateLimitScope(tt.model)
			if ok != tt.expectedOK {
				t.Errorf("ok = %v, want %v", ok, tt.expectedOK)
			}
			if scope != tt.expectedScope {
				t.Errorf("scope = %q, want %q", scope, tt.expectedScope)
			}
		})
	}
}

func TestIsModelRateLimited(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * time.Minute).Format(time.RFC3339)
	past := now.Add(-10 * time.Minute).Format(time.RFC3339)

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		expected       bool
	}{
		{
			name: "official model ID hit - claude-sonnet-4-5",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			expected:       true,
		},
		{
			name: "official model ID hit via mapping - request claude-3-5-sonnet, mapped to claude-sonnet-4-5",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"claude-3-5-sonnet": "claude-sonnet-4-5",
					},
				},
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "claude-3-5-sonnet",
			expected:       true,
		},
		{
			name: "no rate limit - expired",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": past,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			expected:       false,
		},
		{
			name: "no rate limit - no matching key",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-flash": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			expected:       false,
		},
		{
			name:           "no rate limit - unsupported model",
			account:        &Account{},
			requestedModel: "gpt-4",
			expected:       false,
		},
		{
			name:           "no rate limit - empty model",
			account:        &Account{},
			requestedModel: "",
			expected:       false,
		},
		{
			name: "gemini model hit",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-pro-high": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "gemini-3-pro-high",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.isModelRateLimited(tt.requestedModel)
			if result != tt.expected {
				t.Errorf("isModelRateLimited(%q) = %v, want %v", tt.requestedModel, result, tt.expected)
			}
		})
	}
}
