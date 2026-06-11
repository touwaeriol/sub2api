package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// 网关请求重试（报错换号）策略常量。
const (
	// statusCodeMin / statusCodeMax 限定可配置的 HTTP 状态码区间。
	statusCodeMin = 100
	statusCodeMax = 599

	// defaultFailoverCodesSpec 默认换号错误码集合，等价于历史各平台硬编码行为的并集：
	// 401/403/429/529/5xx（三平台共有）+ 402（OpenAI 平台既有行为）。
	defaultFailoverCodesSpec = "401-403,429,500-599"

	// defaultMaxAccountSwitches / defaultMaxAccountSwitchesGemini 与历史 handler 构造期默认值保持一致。
	defaultMaxAccountSwitches       = 10
	defaultMaxAccountSwitchesGemini = 3

	statusCodeRangeSeparator = "-"
	statusCodeListSeparator  = ","
)

// StatusCodeSet 已解析的 HTTP 状态码集合（覆盖 statusCodeMin..statusCodeMax）。
type StatusCodeSet struct {
	codes [statusCodeMax - statusCodeMin + 1]bool
}

// Contains 返回 statusCode 是否在集合内；越界状态码恒为 false。
func (s *StatusCodeSet) Contains(statusCode int) bool {
	if s == nil || statusCode < statusCodeMin || statusCode > statusCodeMax {
		return false
	}
	return s.codes[statusCode-statusCodeMin]
}

// ParseStatusCodeSpec 解析逗号分隔的状态码描述，支持单值与区间混合，
// 例如 "100-199,300-399,401-407,429,500-503"。非法 token、越界或区间反序均返回错误。
func ParseStatusCodeSpec(spec string) (*StatusCodeSet, error) {
	set := &StatusCodeSet{}
	tokens := strings.Split(spec, statusCodeListSeparator)
	nonEmpty := 0
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		nonEmpty++
		low, high, err := parseStatusCodeToken(token)
		if err != nil {
			return nil, err
		}
		for code := low; code <= high; code++ {
			set.codes[code-statusCodeMin] = true
		}
	}
	if nonEmpty == 0 {
		return nil, fmt.Errorf("status code spec is empty")
	}
	return set, nil
}

// parseStatusCodeToken 解析单个 token（"429" 或 "500-503"），返回闭区间 [low, high]。
func parseStatusCodeToken(token string) (low, high int, err error) {
	if before, after, found := strings.Cut(token, statusCodeRangeSeparator); found {
		low, err = parseStatusCodeValue(before)
		if err != nil {
			return 0, 0, err
		}
		high, err = parseStatusCodeValue(after)
		if err != nil {
			return 0, 0, err
		}
		if low > high {
			return 0, 0, fmt.Errorf("invalid status code range %q: start greater than end", token)
		}
		return low, high, nil
	}
	low, err = parseStatusCodeValue(token)
	if err != nil {
		return 0, 0, err
	}
	return low, low, nil
}

func parseStatusCodeValue(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	code, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid status code %q: not a number", raw)
	}
	if code < statusCodeMin || code > statusCodeMax {
		return 0, fmt.Errorf("invalid status code %d: must be within [%d, %d]", code, statusCodeMin, statusCodeMax)
	}
	return code, nil
}

// GatewayRetryPolicy 网关"报错换号"策略的单点权威实现，四平台共用。
// 它只回答"是否换号 / 最多换几次"，账号状态标记由账号级自定义错误码独立控制（两者正交）。
type GatewayRetryPolicy struct {
	failoverCodes        *StatusCodeSet
	networkErrorFailover bool
	maxSwitches          int
	maxSwitchesGemini    int
}

// newGatewayRetryPolicy 构造策略；maxSwitches/maxSwitchesGemini 非正值时回退默认值。
func newGatewayRetryPolicy(codes *StatusCodeSet, networkErrorFailover bool, maxSwitches, maxSwitchesGemini int) *GatewayRetryPolicy {
	if maxSwitches <= 0 {
		maxSwitches = defaultMaxAccountSwitches
	}
	if maxSwitchesGemini <= 0 {
		maxSwitchesGemini = defaultMaxAccountSwitchesGemini
	}
	return &GatewayRetryPolicy{
		failoverCodes:        codes,
		networkErrorFailover: networkErrorFailover,
		maxSwitches:          maxSwitches,
		maxSwitchesGemini:    maxSwitchesGemini,
	}
}

// defaultGatewayRetryPolicy 返回与历史硬编码行为等价的默认策略（fail-open 兜底用）。
func defaultGatewayRetryPolicy() *GatewayRetryPolicy {
	codes, err := ParseStatusCodeSpec(defaultFailoverCodesSpec)
	if err != nil {
		// defaultFailoverCodesSpec 是编译期常量，解析失败属于程序错误。
		panic(fmt.Sprintf("invalid defaultFailoverCodesSpec: %v", err))
	}
	return newGatewayRetryPolicy(codes, true, defaultMaxAccountSwitches, defaultMaxAccountSwitchesGemini)
}

// ShouldFailover 返回该上游状态码是否应触发换号重试。
func (p *GatewayRetryPolicy) ShouldFailover(statusCode int) bool {
	if p == nil {
		return defaultGatewayRetryPolicy().ShouldFailover(statusCode)
	}
	return p.failoverCodes.Contains(statusCode)
}

// ShouldFailoverOnNetworkError 返回网络层错误（连接失败/超时等）是否应触发换号重试。
func (p *GatewayRetryPolicy) ShouldFailoverOnNetworkError() bool {
	if p == nil {
		return defaultGatewayRetryPolicy().networkErrorFailover
	}
	return p.networkErrorFailover
}

// MaxSwitches 返回指定平台的最大换号次数。
func (p *GatewayRetryPolicy) MaxSwitches(platform string) int {
	if p == nil {
		return defaultGatewayRetryPolicy().MaxSwitches(platform)
	}
	if platform == PlatformGemini {
		return p.maxSwitchesGemini
	}
	return p.maxSwitches
}

// resolveGatewayRetryPolicy 四平台共用的策略解析入口：
// settingSvc 未注入（测试/降级场景）时回退与历史行为等价的默认策略。
func resolveGatewayRetryPolicy(ctx context.Context, settingSvc *SettingService) *GatewayRetryPolicy {
	if settingSvc == nil {
		return defaultGatewayRetryPolicy()
	}
	return settingSvc.GetGatewayRetryPolicy(ctx)
}
