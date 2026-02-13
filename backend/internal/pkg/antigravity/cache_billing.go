package antigravity

import (
	"bytes"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tokenutil"
)

type cacheBreakpoint struct {
	messageIndex int
	blockIndex   int
}

// EstimateInputTokensAfterLastCacheBreakpoint 估算“最后一个 cache breakpoint 之后”的 tokens。
//
// Claude Prompt Caching 的 token 拆分语义：
// - cache_read_input_tokens / cache_creation_input_tokens：最后一个 cache breakpoint 之前（含）的 tokens（读/写缓存）
// - input_tokens：最后一个 cache breakpoint 之后的 tokens
//
// 这里我们只做 best-effort 估算，用于“模拟缓存计费”的拆分；不追求与上游 tokenizer 完全一致。
// 如果请求中没有 cache_control（即没有 breakpoint），返回 (0, false)。
func EstimateInputTokensAfterLastCacheBreakpoint(req *ClaudeRequest) (int, bool) {
	if req == nil {
		return 0, false
	}

	bp, ok := findLastCacheBreakpoint(req.Messages)
	if !ok {
		return 0, false
	}

	total := 0
	if bp.messageIndex >= 0 && bp.messageIndex < len(req.Messages) {
		total += estimateTokensForMessageContent(req.Messages[bp.messageIndex].Content, bp.blockIndex+1)
	}
	for i := bp.messageIndex + 1; i < len(req.Messages); i++ {
		total += estimateTokensForMessageContent(req.Messages[i].Content, 0)
	}
	if total < 0 {
		return 0, true
	}
	return total, true
}

// splitUsageForCacheBilling 将“未命中缓存的 prompt tokens”（即 promptTokenCount - cachedContentTokenCount）
// 拆分为 input_tokens 与 cache_creation_input_tokens。
//
// estimatedInputTokens 来自 EstimateInputTokensAfterLastCacheBreakpoint 的估算结果。
func splitUsageForCacheBilling(uncachedPromptTokens, estimatedInputTokens int) (input, cacheCreation int) {
	if uncachedPromptTokens <= 0 {
		return 0, 0
	}
	if estimatedInputTokens < 0 {
		estimatedInputTokens = 0
	}
	if estimatedInputTokens > uncachedPromptTokens {
		estimatedInputTokens = uncachedPromptTokens
	}
	return estimatedInputTokens, uncachedPromptTokens - estimatedInputTokens
}

func findLastCacheBreakpoint(messages []ClaudeMessage) (cacheBreakpoint, bool) {
	for mi := len(messages) - 1; mi >= 0; mi-- {
		blocks, ok := parseContentBlocks(messages[mi].Content)
		if !ok {
			continue
		}
		for bi := len(blocks) - 1; bi >= 0; bi-- {
			if hasCacheControl(blocks[bi].CacheControl) {
				return cacheBreakpoint{messageIndex: mi, blockIndex: bi}, true
			}
		}
	}
	return cacheBreakpoint{}, false
}

func parseContentBlocks(content json.RawMessage) ([]ContentBlock, bool) {
	if len(content) == 0 {
		return nil, false
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, false
	}
	return blocks, true
}

func hasCacheControl(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

func estimateTokensForMessageContent(content json.RawMessage, startBlockIndex int) int {
	if len(content) == 0 {
		return 0
	}

	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		if startBlockIndex > 0 {
			return 0
		}
		return tokenutil.EstimateTokensForText(text)
	}

	blocks, ok := parseContentBlocks(content)
	if !ok {
		return 0
	}
	return estimateTokensForBlocks(blocks, startBlockIndex)
}

func estimateTokensForBlocks(blocks []ContentBlock, start int) int {
	if start < 0 {
		start = 0
	}
	if start >= len(blocks) {
		return 0
	}

	total := 0
	for i := start; i < len(blocks); i++ {
		total += estimateTokensForBlock(blocks[i])
	}
	if total < 0 {
		return 0
	}
	return total
}

func estimateTokensForBlock(block ContentBlock) int {
	switch block.Type {
	case "text":
		return tokenutil.EstimateTokensForText(block.Text)
	case "thinking":
		return tokenutil.EstimateTokensForText(block.Thinking)
	case "tool_use":
		total := tokenutil.EstimateTokensForText(block.Name)
		if block.Input == nil {
			return total
		}
		if b, err := json.Marshal(block.Input); err == nil {
			total += tokenutil.EstimateTokensForText(string(b))
		}
		return total
	case "tool_result":
		return tokenutil.EstimateTokensForText(parseToolResultContent(block.Content, block.IsError))
	default:
		return 0
	}
}
