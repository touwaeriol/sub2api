//go:build unit

package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// =============================================================================
// DeriveSessionID 单元测试
// =============================================================================

func TestDeriveSessionID_Deterministic(t *testing.T) {
	// 相同输入必须产生相同输出（确定性）
	originalID := "-4611686018427387903"
	accountID := int64(123)

	result1 := DeriveSessionID(originalID, accountID)
	result2 := DeriveSessionID(originalID, accountID)

	require.Equal(t, result1, result2, "DeriveSessionID should be deterministic")
}

func TestDeriveSessionID_DifferentAccounts(t *testing.T) {
	// 不同账号必须产生不同的 sessionId
	originalID := "-4611686018427387903"

	result1 := DeriveSessionID(originalID, 123)
	result2 := DeriveSessionID(originalID, 456)

	require.NotEqual(t, result1, result2, "Different accounts should produce different sessionIds")
}

func TestDeriveSessionID_DifferentOriginalIDs(t *testing.T) {
	// 不同原始 sessionId 必须产生不同结果
	accountID := int64(123)

	result1 := DeriveSessionID("-4611686018427387903", accountID)
	result2 := DeriveSessionID("-1234567890123456789", accountID)

	require.NotEqual(t, result1, result2, "Different original sessionIds should produce different results")
}

func TestDeriveSessionID_Format(t *testing.T) {
	// 验证格式：负号 + 数字
	testCases := []struct {
		name      string
		sessionID string
		accountID int64
	}{
		{"standard", "-4611686018427387903", 123},
		{"small_account", "-4611686018427387903", 1},
		{"large_account", "-4611686018427387903", 9999999999},
		{"short_session", "-1", 123},
		{"positive_looking_session", "4611686018427387903", 123},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := DeriveSessionID(tc.sessionID, tc.accountID)

			require.True(t, strings.HasPrefix(result, "-"), "Result should start with '-': got %s", result)
			require.Greater(t, len(result), 1, "Result should have digits after '-'")

			// 验证负号后只有数字
			for _, c := range result[1:] {
				require.True(t, c >= '0' && c <= '9', "Result should only contain digits after '-': got %s", result)
			}
		})
	}
}

func TestDeriveSessionID_EmptyInput(t *testing.T) {
	// 空输入返回空字符串
	result := DeriveSessionID("", 123)
	require.Empty(t, result, "Empty sessionId should return empty string")
}

func TestDeriveSessionID_ZeroAccountID(t *testing.T) {
	// accountID 为 0 也应该正常工作
	result := DeriveSessionID("-4611686018427387903", 0)
	require.True(t, strings.HasPrefix(result, "-"), "Should handle zero accountID")
}

func TestDeriveSessionID_NegativeAccountID(t *testing.T) {
	// 负数 accountID 也应该正常工作
	result := DeriveSessionID("-4611686018427387903", -1)
	require.True(t, strings.HasPrefix(result, "-"), "Should handle negative accountID")
}

func TestDeriveSessionID_NotEqualToOriginal(t *testing.T) {
	// 派生结果不应等于原始值
	originalID := "-4611686018427387903"
	result := DeriveSessionID(originalID, 123)
	require.NotEqual(t, originalID, result, "Derived sessionId should differ from original")
}

// =============================================================================
// ReplaceSessionIDForOAuth 单元测试
// =============================================================================

func TestReplaceSessionIDForOAuth_ReplacesCorrectly(t *testing.T) {
	body := `{"request":{"sessionId":"-4611686018427387903","model":"gemini-2.5-pro"}}`
	accountID := int64(123)

	result, err := ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err)

	var outer map[string]any
	require.NoError(t, json.Unmarshal(result, &outer))

	inner := outer["request"].(map[string]any)
	newSessionID := inner["sessionId"].(string)

	// 验证已被替换（不等于原始值）
	require.NotEqual(t, "-4611686018427387903", newSessionID, "sessionId should have been replaced")

	// 验证格式正确
	require.True(t, strings.HasPrefix(newSessionID, "-"), "Replaced sessionId should start with '-'")

	// 验证其他字段未被修改
	require.Equal(t, "gemini-2.5-pro", inner["model"], "Other fields should not be modified")
}

func TestReplaceSessionIDForOAuth_PreservesOtherFields(t *testing.T) {
	// 验证复杂请求体中其他字段被保留
	body := `{
		"request": {
			"sessionId": "-4611686018427387903",
			"model": "gemini-2.5-pro",
			"contents": [{"role": "user", "parts": [{"text": "hello"}]}],
			"generationConfig": {"temperature": 0.7}
		},
		"projectId": "project-123"
	}`
	accountID := int64(456)

	result, err := ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err)

	var outer map[string]any
	require.NoError(t, json.Unmarshal(result, &outer))

	// 验证 projectId 未被修改
	require.Equal(t, "project-123", outer["projectId"])

	inner := outer["request"].(map[string]any)
	require.Equal(t, "gemini-2.5-pro", inner["model"])
	require.NotNil(t, inner["contents"])
	require.NotNil(t, inner["generationConfig"])
}

func TestReplaceSessionIDForOAuth_NoSessionID(t *testing.T) {
	// 没有 sessionId 字段时，不应添加
	body := `{"request":{"model":"gemini-2.5-pro"}}`
	accountID := int64(123)

	result, err := ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err)

	var outer map[string]any
	require.NoError(t, json.Unmarshal(result, &outer))

	inner := outer["request"].(map[string]any)
	_, exists := inner["sessionId"]
	require.False(t, exists, "Should not add sessionId when not present")
}

func TestReplaceSessionIDForOAuth_EmptySessionID(t *testing.T) {
	// sessionId 为空字符串时，不应替换
	body := `{"request":{"sessionId":"","model":"gemini-2.5-pro"}}`
	accountID := int64(123)

	result, err := ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err)

	var outer map[string]any
	require.NoError(t, json.Unmarshal(result, &outer))

	inner := outer["request"].(map[string]any)
	sessionID, _ := inner["sessionId"].(string)
	require.Empty(t, sessionID, "Empty sessionId should remain empty")
}

func TestReplaceSessionIDForOAuth_NoRequestField(t *testing.T) {
	// 没有 request 字段时，返回原始 body
	body := `{"other":"data","projectId":"123"}`
	accountID := int64(123)

	result, err := ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err)

	var returned map[string]any
	require.NoError(t, json.Unmarshal(result, &returned))

	require.Equal(t, "data", returned["other"])
	require.Equal(t, "123", returned["projectId"])
}

func TestReplaceSessionIDForOAuth_RequestNotObject(t *testing.T) {
	// request 字段不是对象时，返回原始 body
	testCases := []struct {
		name string
		body string
	}{
		{"request_is_string", `{"request":"not an object"}`},
		{"request_is_array", `{"request":[1,2,3]}`},
		{"request_is_number", `{"request":123}`},
		{"request_is_null", `{"request":null}`},
		{"request_is_bool", `{"request":true}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ReplaceSessionIDForOAuth([]byte(tc.body), 123)
			require.NoError(t, err)

			// 验证结果等价于输入（JSON 格式可能不同，但内容相同）
			var original, returned any
			require.NoError(t, json.Unmarshal([]byte(tc.body), &original))
			require.NoError(t, json.Unmarshal(result, &returned))
			require.Equal(t, original, returned)
		})
	}
}

func TestReplaceSessionIDForOAuth_InvalidJSON(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{"not_json", `not valid json`},
		{"truncated", `{"request":`},
		{"empty", ``},
		{"only_whitespace", `   `},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ReplaceSessionIDForOAuth([]byte(tc.body), 123)
			require.Error(t, err, "Should return error for invalid JSON")
			require.Equal(t, tc.body, string(result), "Should return original body on error")
		})
	}
}

func TestReplaceSessionIDForOAuth_SessionIDNotString(t *testing.T) {
	// sessionId 不是字符串时，不应替换
	testCases := []struct {
		name string
		body string
	}{
		{"sessionId_is_number", `{"request":{"sessionId":12345}}`},
		{"sessionId_is_null", `{"request":{"sessionId":null}}`},
		{"sessionId_is_bool", `{"request":{"sessionId":true}}`},
		{"sessionId_is_object", `{"request":{"sessionId":{}}}`},
		{"sessionId_is_array", `{"request":{"sessionId":[]}}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ReplaceSessionIDForOAuth([]byte(tc.body), 123)
			require.NoError(t, err)

			// 验证 sessionId 未被修改（因为不是字符串）
			var original, returned any
			require.NoError(t, json.Unmarshal([]byte(tc.body), &original))
			require.NoError(t, json.Unmarshal(result, &returned))
			require.Equal(t, original, returned, "Non-string sessionId should not be replaced")
		})
	}
}

func TestReplaceSessionIDForOAuth_Deterministic(t *testing.T) {
	// 多次调用结果一致
	body := `{"request":{"sessionId":"-4611686018427387903"}}`
	accountID := int64(123)

	result1, err1 := ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err1)

	result2, err2 := ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err2)

	require.Equal(t, string(result1), string(result2), "Should be deterministic")
}

func TestReplaceSessionIDForOAuth_DifferentAccountsDifferentResults(t *testing.T) {
	// 不同账号产生不同结果
	body := `{"request":{"sessionId":"-4611686018427387903"}}`

	result1, _ := ReplaceSessionIDForOAuth([]byte(body), 123)
	result2, _ := ReplaceSessionIDForOAuth([]byte(body), 456)

	require.NotEqual(t, string(result1), string(result2), "Different accounts should produce different results")
}

// =============================================================================
// 边界情况测试
// =============================================================================

func TestDeriveSessionID_SpecialCharacters(t *testing.T) {
	// 测试包含特殊字符的 sessionId
	testCases := []struct {
		name      string
		sessionID string
	}{
		{"with_spaces", "-461168 6018427387903"},
		{"with_unicode", "-4611686018427387903中文"},
		{"with_newline", "-4611686018427387903\n"},
		{"very_long", strings.Repeat("-1234567890", 100)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := DeriveSessionID(tc.sessionID, 123)
			require.True(t, strings.HasPrefix(result, "-"), "Should handle special characters")
			require.Greater(t, len(result), 1)
		})
	}
}

func TestReplaceSessionIDForOAuth_LargeBody(t *testing.T) {
	// 测试大型请求体
	largeContent := strings.Repeat("a", 1000000) // 1MB 文本
	body := `{"request":{"sessionId":"-4611686018427387903","content":"` + largeContent + `"}}`

	result, err := ReplaceSessionIDForOAuth([]byte(body), 123)
	require.NoError(t, err)

	var outer map[string]any
	require.NoError(t, json.Unmarshal(result, &outer))

	inner := outer["request"].(map[string]any)
	newSessionID := inner["sessionId"].(string)
	require.True(t, strings.HasPrefix(newSessionID, "-"))

	// 验证大内容未被截断
	require.Equal(t, largeContent, inner["content"])
}

func TestReplaceSessionIDForOAuth_NestedRequest(t *testing.T) {
	// 测试嵌套结构中只替换顶层 request.sessionId
	body := `{
		"request": {
			"sessionId": "-4611686018427387903",
			"nested": {
				"request": {
					"sessionId": "-9999999999999999999"
				}
			}
		}
	}`

	result, err := ReplaceSessionIDForOAuth([]byte(body), 123)
	require.NoError(t, err)

	var outer map[string]any
	require.NoError(t, json.Unmarshal(result, &outer))

	inner := outer["request"].(map[string]any)
	newSessionID := inner["sessionId"].(string)

	// 顶层 sessionId 被替换
	require.NotEqual(t, "-4611686018427387903", newSessionID)
	require.True(t, strings.HasPrefix(newSessionID, "-"))

	// 嵌套的 sessionId 未被替换
	nested := inner["nested"].(map[string]any)
	nestedRequest := nested["request"].(map[string]any)
	require.Equal(t, "-9999999999999999999", nestedRequest["sessionId"])
}

// =============================================================================
// 性能基准测试
// =============================================================================

func BenchmarkDeriveSessionID(b *testing.B) {
	originalID := "-4611686018427387903"
	accountID := int64(123)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DeriveSessionID(originalID, accountID)
	}
}

func BenchmarkReplaceSessionIDForOAuth(b *testing.B) {
	body := []byte(`{"request":{"sessionId":"-4611686018427387903","model":"gemini-2.5-pro"}}`)
	accountID := int64(123)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ReplaceSessionIDForOAuth(body, accountID)
	}
}

func BenchmarkReplaceSessionIDForOAuth_LargeBody(b *testing.B) {
	largeContent := strings.Repeat("a", 100000)
	body := []byte(`{"request":{"sessionId":"-4611686018427387903","content":"` + largeContent + `"}}`)
	accountID := int64(123)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ReplaceSessionIDForOAuth(body, accountID)
	}
}
