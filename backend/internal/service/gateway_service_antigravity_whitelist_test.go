//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayService_isModelSupportedByAccount_AntigravityWhitelist(t *testing.T) {
	svc := &GatewayService{}

	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_whitelist": []any{"claude-*"},
		},
	}

	require.True(t, svc.isModelSupportedByAccount(account, "claude-sonnet-4-5"))
	require.False(t, svc.isModelSupportedByAccount(account, "gemini-3-flash"))
	require.False(t, svc.isModelSupportedByAccount(account, "gpt-4"))
	require.True(t, svc.isModelSupportedByAccount(account, ""))
}
