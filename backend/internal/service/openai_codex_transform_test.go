package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApplyCodexOAuthTransform_StoreDefaultsFalse(t *testing.T) {
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.2",
		"input": []any{
			map[string]any{"type": "text", "text": "hello"},
		},
	}

	applyCodexOAuthTransform(reqBody, false)

	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)
}

func TestApplyCodexOAuthTransform_ExplicitStoreFalsePreserved(t *testing.T) {
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"store": false,
		"input": []any{
			map[string]any{"type": "text", "text": "hello"},
		},
	}

	applyCodexOAuthTransform(reqBody, false)

	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)
}

func TestApplyCodexOAuthTransform_ExplicitStoreTrueForcedFalse(t *testing.T) {
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"store": true,
		"input": []any{
			map[string]any{"type": "text", "text": "hello"},
		},
	}

	applyCodexOAuthTransform(reqBody, false)

	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)
}

func TestApplyCodexOAuthTransform_StripsIDs(t *testing.T) {
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{
			map[string]any{"type": "text", "id": "t1", "text": "hi"},
		},
	}

	applyCodexOAuthTransform(reqBody, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)
	item, ok := input[0].(map[string]any)
	require.True(t, ok)
	_, hasID := item["id"]
	require.False(t, hasID)
}

func TestFilterCodexInput_RemovesItemReference(t *testing.T) {
	input := []any{
		map[string]any{"type": "item_reference", "id": "ref1"},
		map[string]any{"type": "text", "id": "t1", "text": "hi"},
	}

	filtered := filterCodexInput(input)
	require.Len(t, filtered, 1)
	item, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", item["type"])
	_, hasID := item["id"]
	require.False(t, hasID)
}

func TestApplyCodexOAuthTransform_EmptyInput(t *testing.T) {
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{},
	}

	applyCodexOAuthTransform(reqBody, false)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 0)
}

func TestApplyCodexOAuthTransform_CodexCLI_PreservesExistingInstructions(t *testing.T) {
	// Codex CLI 场景：已有 instructions 时保持不变
	setupCodexCache(t)

	reqBody := map[string]any{
		"model":        "gpt-5.1",
		"instructions": "user custom instructions",
		"input":        []any{},
	}

	result := applyCodexOAuthTransform(reqBody, true)

	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.Equal(t, "user custom instructions", instructions)
	// instructions 未变，但其他字段（如 store、stream）可能被修改
	require.True(t, result.Modified)
}

func TestApplyCodexOAuthTransform_CodexCLI_AddsInstructionsWhenEmpty(t *testing.T) {
	// Codex CLI 场景：无 instructions 时补充 opencode 指令
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{},
	}

	result := applyCodexOAuthTransform(reqBody, true)

	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.NotEmpty(t, instructions)
	require.True(t, result.Modified)
}

func TestApplyCodexOAuthTransform_NonCodexCLI_UsesOpenCodeInstructions(t *testing.T) {
	// 非 Codex CLI 场景：使用 opencode 指令（缓存中有 header）
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{},
	}

	result := applyCodexOAuthTransform(reqBody, false)

	instructions, ok := reqBody["instructions"].(string)
	require.True(t, ok)
	require.Equal(t, "header", instructions) // setupCodexCache 设置的缓存内容
	require.True(t, result.Modified)
}

func TestIsInstructionsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		reqBody  map[string]any
		expected bool
	}{
		{"field not exists", map[string]any{}, true},
		{"field is nil", map[string]any{"instructions": nil}, true},
		{"field is empty string", map[string]any{"instructions": ""}, true},
		{"field is whitespace", map[string]any{"instructions": "   "}, true},
		{"field has value", map[string]any{"instructions": "hello"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInstructionsEmpty(tt.reqBody)
			require.Equal(t, tt.expected, result)
		})
	}
}

func setupCodexCache(t *testing.T) {
	t.Helper()

	// 使用临时 HOME 避免触发网络拉取 header。
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	cacheDir := filepath.Join(tempDir, ".opencode", "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header.txt"), []byte("header"), 0o644))

	meta := map[string]any{
		"etag":        "",
		"lastFetch":   time.Now().UTC().Format(time.RFC3339),
		"lastChecked": time.Now().UnixMilli(),
	}
	data, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header-meta.json"), data, 0o644))
}
