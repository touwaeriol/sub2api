//go:build unit

package service

import "testing"

func TestParseStatusCodeSpecValid(t *testing.T) {
	tests := []struct {
		name     string
		spec     string
		contains []int
		excludes []int
	}{
		{"single value", "429", []int{429}, []int{428, 430}},
		{"single range", "500-503", []int{500, 501, 503}, []int{499, 504}},
		{"mixed", "100-199,401-407,429,500-503", []int{100, 199, 401, 407, 429, 500}, []int{200, 400, 408, 428, 504}},
		{"boundary codes", "100,599", []int{100, 599}, []int{101, 598}},
		{"full boundary range", "100-599", []int{100, 350, 599}, nil},
		{"whitespace tolerated", " 429 , 500 - 503 ", []int{429, 500, 503}, []int{504}},
		{"trailing comma", "429,", []int{429}, nil},
		{"user example", "100-199,300-399,401-407,409-499,500-503,505-523,525-599", []int{100, 399, 401, 409, 503, 505, 525, 599}, []int{200, 400, 408, 504, 524}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, err := ParseStatusCodeSpec(tt.spec)
			if err != nil {
				t.Fatalf("ParseStatusCodeSpec(%q) error: %v", tt.spec, err)
			}
			for _, code := range tt.contains {
				if !set.Contains(code) {
					t.Errorf("spec %q: expected Contains(%d) = true", tt.spec, code)
				}
			}
			for _, code := range tt.excludes {
				if set.Contains(code) {
					t.Errorf("spec %q: expected Contains(%d) = false", tt.spec, code)
				}
			}
		})
	}
}

func TestParseStatusCodeSpecInvalid(t *testing.T) {
	specs := []string{
		"",            // 空串
		" , ",         // 仅分隔符
		"abc",         // 非数字
		"99",          // 低于下界
		"600",         // 高于上界
		"400-99",      // 区间含越界值
		"503-500",     // 区间反序
		"429,600-610", // 混合中含非法区间
		"4xx",         // 通配写法不支持
	}
	for _, spec := range specs {
		if _, err := ParseStatusCodeSpec(spec); err == nil {
			t.Errorf("ParseStatusCodeSpec(%q): expected error, got nil", spec)
		}
	}
}

func TestStatusCodeSetContainsOutOfRange(t *testing.T) {
	set, err := ParseStatusCodeSpec("100-599")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, code := range []int{0, 99, 600, -1} {
		if set.Contains(code) {
			t.Errorf("expected Contains(%d) = false for out-of-range code", code)
		}
	}
	var nilSet *StatusCodeSet
	if nilSet.Contains(429) {
		t.Error("nil StatusCodeSet should not contain any code")
	}
}

// legacyShouldFailover 历史硬编码行为（401/403/429/529/5xx 三平台共有 + 402 OpenAI 既有）。
// 默认策略必须与该并集逐码等价，保证升级后零行为变化。
func legacyShouldFailover(statusCode int) bool {
	switch statusCode {
	case 401, 402, 403, 429, 529:
		return true
	default:
		return statusCode >= 500
	}
}

func TestDefaultGatewayRetryPolicyMatchesLegacyBehavior(t *testing.T) {
	policy := defaultGatewayRetryPolicy()
	for code := statusCodeMin; code <= statusCodeMax; code++ {
		if got, want := policy.ShouldFailover(code), legacyShouldFailover(code); got != want {
			t.Errorf("ShouldFailover(%d) = %v, legacy behavior = %v", code, got, want)
		}
	}
}

func TestGatewayRetryPolicyMaxSwitches(t *testing.T) {
	codes, err := ParseStatusCodeSpec(defaultFailoverCodesSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	policy := newGatewayRetryPolicy(codes, true, 7, 2)
	if got := policy.MaxSwitches(PlatformAnthropic); got != 7 {
		t.Errorf("MaxSwitches(anthropic) = %d, want 7", got)
	}
	if got := policy.MaxSwitches(PlatformOpenAI); got != 7 {
		t.Errorf("MaxSwitches(openai) = %d, want 7", got)
	}
	if got := policy.MaxSwitches(PlatformGemini); got != 2 {
		t.Errorf("MaxSwitches(gemini) = %d, want 2", got)
	}

	// 非正值回退默认
	fallback := newGatewayRetryPolicy(codes, false, 0, -1)
	if got := fallback.MaxSwitches(PlatformAnthropic); got != defaultMaxAccountSwitches {
		t.Errorf("MaxSwitches fallback = %d, want %d", got, defaultMaxAccountSwitches)
	}
	if got := fallback.MaxSwitches(PlatformGemini); got != defaultMaxAccountSwitchesGemini {
		t.Errorf("MaxSwitches gemini fallback = %d, want %d", got, defaultMaxAccountSwitchesGemini)
	}
	if fallback.ShouldFailoverOnNetworkError() {
		t.Error("ShouldFailoverOnNetworkError() = true, want false")
	}
}

func TestNilGatewayRetryPolicyFallsBackToDefault(t *testing.T) {
	var policy *GatewayRetryPolicy
	if !policy.ShouldFailover(429) {
		t.Error("nil policy should fall back to default: ShouldFailover(429) = false")
	}
	if policy.ShouldFailover(404) {
		t.Error("nil policy should fall back to default: ShouldFailover(404) = true")
	}
	if !policy.ShouldFailoverOnNetworkError() {
		t.Error("nil policy should fall back to default network error failover = true")
	}
	if got := policy.MaxSwitches(PlatformGemini); got != defaultMaxAccountSwitchesGemini {
		t.Errorf("nil policy MaxSwitches(gemini) = %d, want %d", got, defaultMaxAccountSwitchesGemini)
	}
}
