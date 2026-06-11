//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetGatewayRetrySettingsDefaults(t *testing.T) {
	repo := &writableSettingRepoStub{}
	svc := NewSettingService(repo, nil)

	settings, err := svc.GetGatewayRetrySettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, defaultFailoverCodesSpec, settings.FailoverCodes)
	require.Equal(t, defaultMaxAccountSwitches, settings.MaxSwitches)
	require.Equal(t, defaultMaxAccountSwitchesGemini, settings.MaxSwitchesGemini)
	require.True(t, settings.NetworkErrorFailover)
}

func TestGetGatewayRetrySettingsUpgradesPartialJSON(t *testing.T) {
	// 旧版本保存的 JSON 缺字段时回填默认值（含 network_error_failover 缺省 → true）
	repo := &writableSettingRepoStub{values: map[string]string{
		SettingKeyGatewayRetrySettings: `{"failover_codes":"429"}`,
	}}
	svc := NewSettingService(repo, nil)

	settings, err := svc.GetGatewayRetrySettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "429", settings.FailoverCodes)
	require.Equal(t, defaultMaxAccountSwitches, settings.MaxSwitches)
	require.Equal(t, defaultMaxAccountSwitchesGemini, settings.MaxSwitchesGemini)
	require.True(t, settings.NetworkErrorFailover, "缺字段应回填默认 true")
}

func TestGetGatewayRetrySettingsRespectsExplicitNetworkErrorFalse(t *testing.T) {
	// 用户显式保存 false 时不被默认值覆盖
	repo := &writableSettingRepoStub{values: map[string]string{
		SettingKeyGatewayRetrySettings: `{"failover_codes":"429","network_error_failover":false}`,
	}}
	svc := NewSettingService(repo, nil)

	settings, err := svc.GetGatewayRetrySettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.NetworkErrorFailover)
}

func TestSetGatewayRetrySettingsRejectsInvalidSpec(t *testing.T) {
	repo := &writableSettingRepoStub{}
	svc := NewSettingService(repo, nil)

	err := svc.SetGatewayRetrySettings(context.Background(), &GatewayRetrySettings{
		FailoverCodes: "600-700",
	})
	require.Error(t, err)
	require.Empty(t, repo.values)
}

func TestSetGatewayRetrySettingsRefreshesPolicyCache(t *testing.T) {
	repo := &writableSettingRepoStub{}
	svc := NewSettingService(repo, nil)
	ctx := context.Background()

	// 首次读取：默认策略
	policy := svc.GetGatewayRetryPolicy(ctx)
	require.True(t, policy.ShouldFailover(429))
	require.True(t, policy.ShouldFailover(404) == false)

	// 写入窄码集后，策略立即生效（写路径主动刷新缓存）
	err := svc.SetGatewayRetrySettings(ctx, &GatewayRetrySettings{
		FailoverCodes:        "429,500-502",
		MaxSwitches:          5,
		MaxSwitchesGemini:    2,
		NetworkErrorFailover: false,
	})
	require.NoError(t, err)

	updated := svc.GetGatewayRetryPolicy(ctx)
	require.True(t, updated.ShouldFailover(429))
	require.True(t, updated.ShouldFailover(500))
	require.False(t, updated.ShouldFailover(401), "401 不在窄码集中，不应换号")
	require.False(t, updated.ShouldFailover(503))
	require.False(t, updated.ShouldFailoverOnNetworkError())
	require.Equal(t, 5, updated.MaxSwitches(PlatformAnthropic))
	require.Equal(t, 2, updated.MaxSwitches(PlatformGemini))
}

func TestGetGatewayRetryPolicyFailOpenOnCorruptJSON(t *testing.T) {
	repo := &writableSettingRepoStub{values: map[string]string{
		SettingKeyGatewayRetrySettings: `{not-json`,
	}}
	svc := NewSettingService(repo, nil)

	policy := svc.GetGatewayRetryPolicy(context.Background())
	// 损坏 JSON 回退默认策略，与历史行为等价
	require.True(t, policy.ShouldFailover(429))
	require.True(t, policy.ShouldFailover(529))
	require.False(t, policy.ShouldFailover(404))
	require.Equal(t, defaultMaxAccountSwitches, policy.MaxSwitches(PlatformOpenAI))
}
