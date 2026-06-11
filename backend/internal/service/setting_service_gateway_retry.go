package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// 网关请求重试设置的进程内缓存参数。
// 该配置位于网关热路径（每次换号判定都会读取），采用 atomic.Value + singleflight
// 的零锁缓存（同 gatewayForwardingCache 模式），不走 Redis。
const (
	gatewayRetryPolicyCacheTTL  = 60 * time.Second
	gatewayRetryPolicyErrorTTL  = 5 * time.Second
	gatewayRetryPolicyDBTimeout = 5 * time.Second

	gatewayRetryPolicySFKey = "gateway_retry_policy"
)

// cachedGatewayRetryPolicy 进程内缓存条目。
type cachedGatewayRetryPolicy struct {
	policy    *GatewayRetryPolicy
	expiresAt int64 // unix nano
}

// GetGatewayRetrySettings 获取网关请求重试配置（管理端读路径，直查 DB）。
func (s *SettingService) GetGatewayRetrySettings(ctx context.Context) (*GatewayRetrySettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyGatewayRetrySettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return s.defaultGatewayRetrySettings(), nil
		}
		return nil, fmt.Errorf("get gateway retry settings: %w", err)
	}
	if value == "" {
		return s.defaultGatewayRetrySettings(), nil
	}

	// NetworkErrorFailover 用 *bool 接收：区分"旧 JSON 缺字段"（回填默认 true）
	// 与"用户显式保存 false"（尊重用户配置）。
	var raw struct {
		FailoverCodes        string `json:"failover_codes"`
		MaxSwitches          int    `json:"max_switches"`
		MaxSwitchesGemini    int    `json:"max_switches_gemini"`
		NetworkErrorFailover *bool  `json:"network_error_failover"`
	}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return s.defaultGatewayRetrySettings(), nil
	}
	settings := GatewayRetrySettings{
		FailoverCodes:        raw.FailoverCodes,
		MaxSwitches:          raw.MaxSwitches,
		MaxSwitchesGemini:    raw.MaxSwitchesGemini,
		NetworkErrorFailover: raw.NetworkErrorFailover == nil || *raw.NetworkErrorFailover,
	}
	s.applyGatewayRetryDefaults(&settings)
	return &settings, nil
}

// defaultGatewayRetrySettings 返回默认配置；yaml 配置的 max_account_switches(_gemini)
// 优先于内置常量（保持升级前 cfg 配置的语义）。
func (s *SettingService) defaultGatewayRetrySettings() *GatewayRetrySettings {
	settings := DefaultGatewayRetrySettings()
	s.applyGatewayRetryDefaults(settings)
	return settings
}

// applyGatewayRetryDefaults 对缺省/非法字段回填默认值（旧 JSON 升级兼容 + cfg fallback）。
func (s *SettingService) applyGatewayRetryDefaults(settings *GatewayRetrySettings) {
	if settings.FailoverCodes == "" {
		settings.FailoverCodes = defaultFailoverCodesSpec
	}
	if settings.MaxSwitches <= 0 {
		settings.MaxSwitches = defaultMaxAccountSwitches
		if s.cfg != nil && s.cfg.Gateway.MaxAccountSwitches > 0 {
			settings.MaxSwitches = s.cfg.Gateway.MaxAccountSwitches
		}
	}
	if settings.MaxSwitchesGemini <= 0 {
		settings.MaxSwitchesGemini = defaultMaxAccountSwitchesGemini
		if s.cfg != nil && s.cfg.Gateway.MaxAccountSwitchesGemini > 0 {
			settings.MaxSwitchesGemini = s.cfg.Gateway.MaxAccountSwitchesGemini
		}
	}
}

// SetGatewayRetrySettings 写入网关请求重试配置并刷新进程内缓存。
// FailoverCodes 必须是合法的区间语法，否则返回解析错误。
func (s *SettingService) SetGatewayRetrySettings(ctx context.Context, settings *GatewayRetrySettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	s.applyGatewayRetryDefaults(settings)
	codes, err := ParseStatusCodeSpec(settings.FailoverCodes)
	if err != nil {
		return fmt.Errorf("invalid failover codes: %w", err)
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal gateway retry settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyGatewayRetrySettings, string(data)); err != nil {
		return err
	}

	// 先使 inflight singleflight 失效，再刷新缓存，缩小旧值覆盖新值的竞态窗口。
	// 多实例部署下其他实例最长经过一个 TTL 窗口后拿到新值。
	s.gatewayRetryPolicySF.Forget(gatewayRetryPolicySFKey)
	s.gatewayRetryPolicyCache.Store(&cachedGatewayRetryPolicy{
		policy:    newGatewayRetryPolicy(codes, settings.NetworkErrorFailover, settings.MaxSwitches, settings.MaxSwitchesGemini),
		expiresAt: time.Now().Add(gatewayRetryPolicyCacheTTL).UnixNano(),
	})
	return nil
}

// GetGatewayRetryPolicy 获取已解析的换号重试策略（网关热路径，进程内缓存）。
// 任何失败（DB 错误 / JSON 损坏 / 区间非法）都 fail-open 回退默认策略，保证网关可用。
func (s *SettingService) GetGatewayRetryPolicy(ctx context.Context) *GatewayRetryPolicy {
	if cached, ok := s.gatewayRetryPolicyCache.Load().(*cachedGatewayRetryPolicy); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.policy
		}
	}

	val, _, _ := s.gatewayRetryPolicySF.Do(gatewayRetryPolicySFKey, func() (any, error) {
		if cached, ok := s.gatewayRetryPolicyCache.Load().(*cachedGatewayRetryPolicy); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.policy, nil
			}
		}
		return s.loadGatewayRetryPolicy(ctx), nil
	})
	if policy, ok := val.(*GatewayRetryPolicy); ok && policy != nil {
		return policy
	}
	return defaultGatewayRetryPolicy()
}

// loadGatewayRetryPolicy 从 DB 加载并解析策略，写入进程内缓存。
func (s *SettingService) loadGatewayRetryPolicy(ctx context.Context) *GatewayRetryPolicy {
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayRetryPolicyDBTimeout)
	defer cancel()

	settings, err := s.GetGatewayRetrySettings(dbCtx)
	if err != nil {
		slog.Warn("failed to load gateway retry settings, falling back to default policy", "error", err)
		policy := defaultGatewayRetryPolicy()
		s.storeGatewayRetryPolicy(policy, gatewayRetryPolicyErrorTTL)
		return policy
	}

	policy := buildGatewayRetryPolicy(settings)
	s.storeGatewayRetryPolicy(policy, gatewayRetryPolicyCacheTTL)
	return policy
}

func (s *SettingService) storeGatewayRetryPolicy(policy *GatewayRetryPolicy, ttl time.Duration) {
	s.gatewayRetryPolicyCache.Store(&cachedGatewayRetryPolicy{
		policy:    policy,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
}

// buildGatewayRetryPolicy 将配置解析为策略；FailoverCodes 非法时回退默认码集（fail-open）。
func buildGatewayRetryPolicy(settings *GatewayRetrySettings) *GatewayRetryPolicy {
	codes, err := ParseStatusCodeSpec(settings.FailoverCodes)
	if err != nil {
		slog.Warn("invalid gateway retry failover codes, falling back to default spec",
			"spec", settings.FailoverCodes, "error", err)
		codes, _ = ParseStatusCodeSpec(defaultFailoverCodesSpec)
	}
	return newGatewayRetryPolicy(codes, settings.NetworkErrorFailover, settings.MaxSwitches, settings.MaxSwitchesGemini)
}
