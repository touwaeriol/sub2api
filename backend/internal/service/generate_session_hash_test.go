//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateSessionHash_SystemPlusMessages(t *testing.T) {
	svc := &GatewayService{}

	// system + messages 组成完整摘要串
	withSystem := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	withoutSystem := &ParsedRequest{
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}

	h1 := svc.GenerateSessionHash(withSystem)
	h2 := svc.GenerateSessionHash(withoutSystem)
	require.NotEmpty(t, h1)
	require.NotEmpty(t, h2)
	require.NotEqual(t, h1, h2, "system prompt should be part of digest, producing different hash")
}

func TestGenerateSessionHash_SystemOnlyProducesHash(t *testing.T) {
	svc := &GatewayService{}

	parsed := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
	}
	hash := svc.GenerateSessionHash(parsed)
	require.NotEmpty(t, hash, "system prompt alone should produce a hash as part of full digest")
}

func TestGenerateSessionHash_DifferentSystemsSameMessages(t *testing.T) {
	svc := &GatewayService{}

	parsed1 := &ParsedRequest{
		System:    "You are assistant A.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	parsed2 := &ParsedRequest{
		System:    "You are assistant B.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}

	h1 := svc.GenerateSessionHash(parsed1)
	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEqual(t, h1, h2, "different system prompts with same messages should produce different hashes")
}

func TestGenerateSessionHash_SameSystemSameMessages(t *testing.T) {
	svc := &GatewayService{}

	// 同一客户端工具（相同 system）+ 相同对话内容 → 相同 hash（合理的粘性）
	mk := func() *ParsedRequest {
		return &ParsedRequest{
			System:    "You are a helpful assistant.",
			HasSystem: true,
			Messages: []any{
				map[string]any{"role": "user", "content": "hello"},
				map[string]any{"role": "assistant", "content": "hi"},
			},
		}
	}

	h1 := svc.GenerateSessionHash(mk())
	h2 := svc.GenerateSessionHash(mk())
	require.Equal(t, h1, h2, "same system + same messages should produce identical hash")
}

func TestGenerateSessionHash_DifferentMessagesProduceDifferentHash(t *testing.T) {
	svc := &GatewayService{}

	// 相同 system prompt 但不同用户的对话内容不同 → 不同 hash → 分配到不同账号
	parsed1 := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "help me with Go"},
		},
	}
	parsed2 := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "help me with Python"},
		},
	}

	h1 := svc.GenerateSessionHash(parsed1)
	h2 := svc.GenerateSessionHash(parsed2)
	require.NotEqual(t, h1, h2, "same system but different messages should produce different hashes")
}

func TestGenerateSessionHash_MetadataHasHighestPriority(t *testing.T) {
	svc := &GatewayService{}

	parsed := &ParsedRequest{
		MetadataUserID: "session_123e4567-e89b-12d3-a456-426614174000",
		System:         "You are a helpful assistant.",
		HasSystem:      true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}

	hash := svc.GenerateSessionHash(parsed)
	require.Equal(t, "123e4567-e89b-12d3-a456-426614174000", hash, "metadata session_id should have highest priority")
}

func TestGenerateSessionHash_NilParsedRequest(t *testing.T) {
	svc := &GatewayService{}
	require.Empty(t, svc.GenerateSessionHash(nil))
}

func TestGenerateSessionHash_EmptyRequest(t *testing.T) {
	svc := &GatewayService{}
	require.Empty(t, svc.GenerateSessionHash(&ParsedRequest{}))
}
