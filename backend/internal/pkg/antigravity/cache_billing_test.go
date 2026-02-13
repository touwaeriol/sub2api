//go:build unit

package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tokenutil"
	"github.com/stretchr/testify/require"
)

func mustCountTokens(t *testing.T, s string) int {
	t.Helper()
	n, err := tokenutil.CountTokensForText(s)
	require.NoError(t, err, "tiktoken 初始化失败，无法使用分词库计算 tokens")
	return n
}

func TestEstimateInputTokensAfterLastCacheBreakpoint_NoBreakpoint(t *testing.T) {
	t.Parallel()

	req := &ClaudeRequest{
		Messages: []ClaudeMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	tokens, ok, err := EstimateInputTokensAfterLastCacheBreakpoint(req)
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, 0, tokens)
}

func TestEstimateInputTokensAfterLastCacheBreakpoint_SumsSuffix(t *testing.T) {
	t.Parallel()

	req := &ClaudeRequest{
		Messages: []ClaudeMessage{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"cached","cache_control":{"type":"ephemeral"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"abcd"}]`)},
			{Role: "assistant", Content: json.RawMessage(`"你好"`)},
		},
	}

	tokens, ok, err := EstimateInputTokensAfterLastCacheBreakpoint(req)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, mustCountTokens(t, "abcd")+mustCountTokens(t, "你好"), tokens)
}

func TestEstimateInputTokensAfterLastCacheBreakpoint_LastBreakpointWins(t *testing.T) {
	t.Parallel()

	req := &ClaudeRequest{
		Messages: []ClaudeMessage{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"b","cache_control":{"type":"ephemeral"}},{"type":"text","text":"abcdefg"}]`)},
		},
	}

	tokens, ok, err := EstimateInputTokensAfterLastCacheBreakpoint(req)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, mustCountTokens(t, "abcdefg"), tokens)
}

func TestSplitUsageForCacheBilling_Clamp(t *testing.T) {
	t.Parallel()

	in, create := splitUsageForCacheBilling(10, 3)
	require.Equal(t, 3, in)
	require.Equal(t, 7, create)

	in, create = splitUsageForCacheBilling(10, 20)
	require.Equal(t, 10, in)
	require.Equal(t, 0, create)

	in, create = splitUsageForCacheBilling(10, -1)
	require.Equal(t, 0, in)
	require.Equal(t, 10, create)
}

func TestStreamingProcessor_SimulateCacheBilling_UsesEstimatedInputTokens(t *testing.T) {
	t.Parallel()

	p := NewStreamingProcessor("claude-sonnet-4-5", true, 3)

	v1 := V1InternalResponse{
		ResponseID: "resp-1",
		Response: GeminiResponse{
			Candidates: []GeminiCandidate{
				{Content: &GeminiContent{Role: "model", Parts: []GeminiPart{{Text: "hi"}}}},
			},
			UsageMetadata: &GeminiUsageMetadata{
				PromptTokenCount:        100,
				CachedContentTokenCount: 20,
				CandidatesTokenCount:    5,
				ThoughtsTokenCount:      0,
			},
		},
	}
	b, err := json.Marshal(v1)
	require.NoError(t, err)

	out := p.ProcessLine("data: " + string(b))
	require.NotNil(t, out)

	_, usage := p.Finish()
	require.NotNil(t, usage)

	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 77, usage.CacheCreationInputTokens)
	require.Equal(t, 20, usage.CacheReadInputTokens)
	require.Equal(t, 5, usage.OutputTokens)
}

func TestTransformGeminiToClaude_SimulateCacheBilling_UsesEstimatedInputTokens(t *testing.T) {
	t.Parallel()

	resp := GeminiResponse{
		Candidates: []GeminiCandidate{
			{Content: &GeminiContent{Role: "model", Parts: []GeminiPart{{Text: "ok"}}}},
		},
		UsageMetadata: &GeminiUsageMetadata{
			PromptTokenCount:        100,
			CachedContentTokenCount: 20,
			CandidatesTokenCount:    5,
			ThoughtsTokenCount:      0,
		},
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)

	_, usage, err := TransformGeminiToClaude(b, "claude-sonnet-4-5", true, 3)
	require.NoError(t, err)
	require.NotNil(t, usage)

	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 77, usage.CacheCreationInputTokens)
	require.Equal(t, 20, usage.CacheReadInputTokens)
	require.Equal(t, 5, usage.OutputTokens)
}
