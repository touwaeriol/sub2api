package service

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// CleanGeminiNativeThoughtSignatures 将 Gemini 原生 API 请求中的 thoughtSignature 字段替换为 dummy signature，
// 以避免跨账号签名验证错误。
//
// 当粘性会话切换账号时（例如原账号异常、不可调度等），旧账号返回的 thoughtSignature
// 会导致新账号的签名验证失败。通过替换为 dummy signature，让新账号跳过验证。
//
// CleanGeminiNativeThoughtSignatures replaces thoughtSignature fields in Gemini native API requests
// with dummy signature to avoid cross-account signature validation errors.
//
// When sticky session switches accounts (e.g., original account becomes unavailable),
// thoughtSignatures from the old account will cause validation failures on the new account.
// By replacing these signatures with dummy, we allow the new account to skip validation.
func CleanGeminiNativeThoughtSignatures(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	// 解析 JSON
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		// 如果解析失败，返回原始 body（可能不是 JSON 或格式不正确）
		return body
	}

	// 递归替换 thoughtSignature 为 dummy signature
	cleaned := replaceThoughtSignaturesRecursive(data)

	// 重新序列化
	result, err := json.Marshal(cleaned)
	if err != nil {
		// 如果序列化失败，返回原始 body
		return body
	}

	return result
}

// replaceThoughtSignaturesRecursive 递归遍历数据结构，将所有 thoughtSignature 字段替换为 dummy signature
func replaceThoughtSignaturesRecursive(data any) any {
	switch v := data.(type) {
	case map[string]any:
		// 创建新的 map
		result := make(map[string]any, len(v))
		for key, value := range v {
			// 将 thoughtSignature 替换为 dummy signature
			if key == "thoughtSignature" {
				result[key] = antigravity.DummyThoughtSignature
				continue
			}
			// 递归处理嵌套结构
			result[key] = replaceThoughtSignaturesRecursive(value)
		}
		return result

	case []any:
		// 递归处理数组中的每个元素
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = replaceThoughtSignaturesRecursive(item)
		}
		return result

	default:
		// 基本类型（string, number, bool, null）直接返回
		return v
	}
}
