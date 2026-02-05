package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStripSignatureSensitiveBlocksFromClaudeRequest(t *testing.T) {
	req := &antigravity.ClaudeRequest{
		Model: "claude-sonnet-4-5",
		Thinking: &antigravity.ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: 1024,
		},
		Messages: []antigravity.ClaudeMessage{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"secret plan","signature":""},
					{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}
				]`),
			},
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"tool_result","tool_use_id":"t1","content":"ok","is_error":false},
					{"type":"redacted_thinking","data":"..."}
				]`),
			},
		},
	}

	changed, err := stripSignatureSensitiveBlocksFromClaudeRequest(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Nil(t, req.Thinking)

	require.Len(t, req.Messages, 2)

	var blocks0 []map[string]any
	require.NoError(t, json.Unmarshal(req.Messages[0].Content, &blocks0))
	require.Len(t, blocks0, 2)
	require.Equal(t, "text", blocks0[0]["type"])
	require.Equal(t, "secret plan", blocks0[0]["text"])
	require.Equal(t, "text", blocks0[1]["type"])

	var blocks1 []map[string]any
	require.NoError(t, json.Unmarshal(req.Messages[1].Content, &blocks1))
	require.Len(t, blocks1, 1)
	require.Equal(t, "text", blocks1[0]["type"])
	require.NotEmpty(t, blocks1[0]["text"])
}

func TestStripThinkingFromClaudeRequest_DoesNotDowngradeTools(t *testing.T) {
	req := &antigravity.ClaudeRequest{
		Model: "claude-sonnet-4-5",
		Thinking: &antigravity.ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: 1024,
		},
		Messages: []antigravity.ClaudeMessage{
			{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"thinking","thinking":"secret plan"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]`),
			},
		},
	}

	changed, err := stripThinkingFromClaudeRequest(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Nil(t, req.Thinking)

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal(req.Messages[0].Content, &blocks))
	require.Len(t, blocks, 2)
	require.Equal(t, "text", blocks[0]["type"])
	require.Equal(t, "secret plan", blocks[0]["text"])
	require.Equal(t, "tool_use", blocks[1]["type"])
}

func TestIsPromptTooLongError(t *testing.T) {
	require.True(t, isPromptTooLongError([]byte(`{"error":{"message":"Prompt is too long"}}`)))
	require.True(t, isPromptTooLongError([]byte(`{"message":"Prompt is too long"}`)))
	require.False(t, isPromptTooLongError([]byte(`{"error":{"message":"other"}}`)))
}

// =============================================================================
// SessionID 替换单元测试
// =============================================================================

// TestSessionIDReplacementDeterminism 验证相同输入产生相同输出
func TestSessionIDReplacementDeterminism(t *testing.T) {
	originalSessionID := "-4611686018427387903"
	accountID := int64(12345)

	result1 := antigravity.DeriveSessionID(originalSessionID, accountID)
	result2 := antigravity.DeriveSessionID(originalSessionID, accountID)

	require.Equal(t, result1, result2, "Same input should produce same output")
}

// TestSessionIDReplacementIsolation 验证不同账号产生不同的 sessionId
func TestSessionIDReplacementIsolation(t *testing.T) {
	originalSessionID := "-4611686018427387903"

	result1 := antigravity.DeriveSessionID(originalSessionID, 100)
	result2 := antigravity.DeriveSessionID(originalSessionID, 200)

	require.NotEqual(t, result1, result2, "Different accounts should produce different sessionIds")
}

// TestReplaceSessionIDForOAuth_Integration 测试完整的替换流程
func TestReplaceSessionIDForOAuth_Integration(t *testing.T) {
	// 模拟 v1internal 请求体
	body := `{
		"request": {
			"sessionId": "-4611686018427387903",
			"model": "gemini-2.5-pro",
			"contents": [{"role": "user", "parts": [{"text": "hello"}]}]
		},
		"projectId": "project-123"
	}`

	accountID := int64(12345)

	result, err := antigravity.ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err)

	var outer map[string]any
	require.NoError(t, json.Unmarshal(result, &outer))

	inner := outer["request"].(map[string]any)

	// 验证 sessionId 已被替换
	newSessionID := inner["sessionId"].(string)
	require.NotEqual(t, "-4611686018427387903", newSessionID)

	// 验证是确定性的
	expected := antigravity.DeriveSessionID("-4611686018427387903", accountID)
	require.Equal(t, expected, newSessionID)

	// 验证其他字段未被修改
	require.Equal(t, "gemini-2.5-pro", inner["model"])
	require.Equal(t, "project-123", outer["projectId"])
}

// TestOAuthAccountShouldReplaceSessionID 验证 OAuth 账号类型判断
func TestOAuthAccountShouldReplaceSessionID(t *testing.T) {
	oauthAccount := &Account{
		ID:   12345,
		Type: AccountTypeOAuth,
	}
	require.True(t, oauthAccount.IsOAuth(), "OAuth account should return true for IsOAuth()")

	apiKeyAccount := &Account{
		ID:   12345,
		Type: AccountTypeAPIKey,
	}
	require.False(t, apiKeyAccount.IsOAuth(), "API Key account should return false for IsOAuth()")
}

// TestSessionIDReplacementWithEmptySessionID 验证空 sessionId 不会被替换
func TestSessionIDReplacementWithEmptySessionID(t *testing.T) {
	body := `{"request":{"sessionId":"","model":"gemini-2.5-pro"}}`
	accountID := int64(12345)

	result, err := antigravity.ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err)

	var outer map[string]any
	require.NoError(t, json.Unmarshal(result, &outer))

	inner := outer["request"].(map[string]any)
	sessionID, _ := inner["sessionId"].(string)
	require.Empty(t, sessionID, "Empty sessionId should remain empty")
}

// TestSessionIDReplacementWithoutSessionID 验证没有 sessionId 字段时不会添加
func TestSessionIDReplacementWithoutSessionID(t *testing.T) {
	body := `{"request":{"model":"gemini-2.5-pro"}}`
	accountID := int64(12345)

	result, err := antigravity.ReplaceSessionIDForOAuth([]byte(body), accountID)
	require.NoError(t, err)

	var outer map[string]any
	require.NoError(t, json.Unmarshal(result, &outer))

	inner := outer["request"].(map[string]any)
	_, exists := inner["sessionId"]
	require.False(t, exists, "Should not add sessionId when not present")
}

type httpUpstreamStub struct {
	resp *http.Response
	err  error
}

func (s *httpUpstreamStub) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return s.resp, s.err
}

func (s *httpUpstreamStub) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, _ bool) (*http.Response, error) {
	return s.resp, s.err
}

func TestAntigravityGatewayService_Forward_PromptTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	body, err := json.Marshal(map[string]any{
		"model": "claude-opus-4-5",
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
		"max_tokens": 1,
		"stream":     false,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request = req

	respBody := []byte(`{"error":{"message":"Prompt is too long"}}`)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"X-Request-Id": []string{"req-1"}},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{},
		httpUpstream:  &httpUpstreamStub{resp: resp},
	}

	account := &Account{
		ID:          1,
		Name:        "acc-1",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
		},
	}

	result, err := svc.Forward(context.Background(), c, account, body, false)
	require.Nil(t, result)

	var promptErr *PromptTooLongError
	require.ErrorAs(t, err, &promptErr)
	require.Equal(t, http.StatusBadRequest, promptErr.StatusCode)
	require.Equal(t, "req-1", promptErr.RequestID)
	require.NotEmpty(t, promptErr.Body)

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "prompt_too_long", events[0].Kind)
}
