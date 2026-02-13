package tokenutil

import "strings"

// EstimateTokensForText 以非常粗略的方式估算文本 token 数量。
//
// 说明：
// - ASCII/英文类文本：按约 4 chars / token 估算
// - CJK 为主文本：按约 1 rune / token 估算
//
// 该函数用于“计费模拟/近似统计”，不追求与上游 tokenizer 完全一致，但必须：
// - 确定性（相同输入 -> 相同输出）
// - 对空串/空白安全
func EstimateTokensForText(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	runes := []rune(s)
	if len(runes) == 0 {
		return 0
	}

	ascii := 0
	for _, r := range runes {
		if r <= 0x7f {
			ascii++
		}
	}

	asciiRatio := float64(ascii) / float64(len(runes))
	if asciiRatio >= 0.8 {
		// Roughly 4 chars per token for English-like text.
		return (len(runes) + 3) / 4
	}

	// For CJK-heavy text, approximate 1 rune per token.
	return len(runes)
}
