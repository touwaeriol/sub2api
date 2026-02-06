//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayService_isModelSupportedByAccount_AntigravityModelMapping(t *testing.T) {
	svc := &GatewayService{}

	// 使用 model_mapping 作为白名单（通配符匹配）
	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"claude-*":  "claude-sonnet-4-5",
				"gemini-3-*": "gemini-3-flash",
			},
		},
	}

	// claude-* 通配符匹配
	require.True(t, svc.isModelSupportedByAccount(account, "claude-sonnet-4-5"))
	require.True(t, svc.isModelSupportedByAccount(account, "claude-3-5-sonnet-20241022"))
	require.True(t, svc.isModelSupportedByAccount(account, "claude-opus-4-5"))

	// gemini-3-* 通配符匹配
	require.True(t, svc.isModelSupportedByAccount(account, "gemini-3-flash"))
	require.True(t, svc.isModelSupportedByAccount(account, "gemini-3-pro-high"))

	// gemini-2.5-* 不匹配（不在 model_mapping 中）
	require.False(t, svc.isModelSupportedByAccount(account, "gemini-2.5-flash"))
	require.False(t, svc.isModelSupportedByAccount(account, "gemini-2.5-pro"))

	// 其他平台模型不支持
	require.False(t, svc.isModelSupportedByAccount(account, "gpt-4"))

	// 空模型允许
	require.True(t, svc.isModelSupportedByAccount(account, ""))
}

func TestGatewayService_isModelSupportedByAccount_AntigravityNoMapping(t *testing.T) {
	svc := &GatewayService{}

	// 未配置 model_mapping 时，允许所有 claude-/gemini- 前缀模型
	account := &Account{
		Platform:    PlatformAntigravity,
		Credentials: map[string]any{},
	}

	require.True(t, svc.isModelSupportedByAccount(account, "claude-sonnet-4-5"))
	require.True(t, svc.isModelSupportedByAccount(account, "claude-3-5-sonnet-20241022"))
	require.True(t, svc.isModelSupportedByAccount(account, "gemini-3-flash"))
	require.True(t, svc.isModelSupportedByAccount(account, "gemini-2.5-pro"))

	// 非 claude-/gemini- 前缀仍然不支持
	require.False(t, svc.isModelSupportedByAccount(account, "gpt-4"))
}
