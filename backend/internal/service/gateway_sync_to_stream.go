package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// collectAnthropicStreamEvents reads all SSE events from an Anthropic streaming
// response and assembles them into a complete AnthropicResponse + ClaudeUsage.
// This is the single-point implementation for buffered stream collection, shared by
// sync-to-stream, ForwardAsChatCompletions, and ForwardAsResponses paths.
func (s *GatewayService) collectAnthropicStreamEvents(resp *http.Response) (*apicompat.AnthropicResponse, *ClaudeUsage, error) {
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var finalResp *apicompat.AnthropicResponse
	var usage ClaudeUsage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "event: ") {
			continue
		}
		eventType := line[7:]

		if !scanner.Scan() {
			break
		}
		payload, ok := extractAnthropicSSEDataLine(scanner.Text())
		if !ok {
			continue
		}

		s.parseSSEUsagePassthrough(payload, &usage)

		switch eventType {
		case "message_start":
			var event apicompat.AnthropicStreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err == nil && event.Message != nil {
				finalResp = event.Message
			}
		case "content_block_start":
			if finalResp == nil {
				continue
			}
			var event apicompat.AnthropicStreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err == nil && event.ContentBlock != nil {
				// tool_use 的 content_block_start 携带 input:{}（占位符），
				// 实际内容由后续 input_json_delta 累积；清除以避免 appendRawJSON 拼出畸形 JSON。
				if event.ContentBlock.Type == "tool_use" {
					event.ContentBlock.Input = nil
				}
				finalResp.Content = append(finalResp.Content, *event.ContentBlock)
			}
		case "content_block_delta":
			if finalResp == nil {
				continue
			}
			var event apicompat.AnthropicStreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err == nil && event.Delta != nil && event.Index != nil {
				idx := *event.Index
				if idx < len(finalResp.Content) {
					switch event.Delta.Type {
					case "text_delta":
						finalResp.Content[idx].Text += event.Delta.Text
					case "thinking_delta":
						finalResp.Content[idx].Thinking += event.Delta.Thinking
					case "input_json_delta":
						finalResp.Content[idx].Input = appendRawJSON(finalResp.Content[idx].Input, event.Delta.PartialJSON)
					case "signature_delta":
						finalResp.Content[idx].Signature += event.Delta.Signature
					}
				}
			}
		case "message_delta":
			var event apicompat.AnthropicStreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err == nil && event.Delta != nil && finalResp != nil {
				if event.Delta.StopReason != "" {
					finalResp.StopReason = event.Delta.StopReason
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("upstream stream read error: %w", err)
	}

	if finalResp == nil {
		return nil, nil, fmt.Errorf("upstream stream ended without a response")
	}

	finalResp.Usage = apicompat.AnthropicUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
	}

	return finalResp, &usage, nil
}

// marshalAnthropicResponseWithCacheDetails marshals an AnthropicResponse to JSON
// and appends the extended cache_creation nested fields from ClaudeUsage.
func marshalAnthropicResponseWithCacheDetails(resp *apicompat.AnthropicResponse, usage *ClaudeUsage) ([]byte, error) {
	body, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal assembled response: %w", err)
	}
	if usage != nil && (usage.CacheCreation5mTokens > 0 || usage.CacheCreation1hTokens > 0) {
		if newBody, err := sjson.SetBytes(body, "usage.cache_creation.ephemeral_5m_input_tokens", usage.CacheCreation5mTokens); err == nil {
			body = newBody
		}
		if newBody, err := sjson.SetBytes(body, "usage.cache_creation.ephemeral_1h_input_tokens", usage.CacheCreation1hTokens); err == nil {
			body = newBody
		}
	}
	return body, nil
}

// applySyncToStream checks whether sync-to-stream conversion should be applied
// and modifies the body + stream flag accordingly.
func applySyncToStream(body []byte, reqStream bool, account *Account) ([]byte, bool, bool) {
	if reqStream || !account.IsSyncToStreamEnabled() {
		return body, reqStream, false
	}
	if modified, err := sjson.SetBytes(body, "stream", true); err == nil {
		body = modified
	}
	return body, true, true
}

// handleSyncToStreamResponse handles the main-path sync-to-stream conversion.
// Reads streaming response, assembles non-streaming JSON, applies model replacement
// and cache TTL override, then writes to the client.
func (s *GatewayService) handleSyncToStreamResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel, mappedModel string,
) (*ClaudeUsage, error) {
	s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)

	finalResp, usage, err := s.collectAnthropicStreamEvents(resp)
	if err != nil {
		return nil, err
	}

	body, err := marshalAnthropicResponseWithCacheDetails(finalResp, usage)
	if err != nil {
		return nil, err
	}

	if overrideTarget, ok := s.resolveCacheTTLUsageOverrideTarget(ctx, account); ok {
		if applyCacheTTLOverride(usage, overrideTarget) {
			if newBody, err := sjson.SetBytes(body, "usage.cache_creation.ephemeral_5m_input_tokens", usage.CacheCreation5mTokens); err == nil {
				body = newBody
			}
			if newBody, err := sjson.SetBytes(body, "usage.cache_creation.ephemeral_1h_input_tokens", usage.CacheCreation1hTokens); err == nil {
				body = newBody
			}
		}
	}

	if originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	body = reverseToolNamesIfPresent(c, body)
	c.Data(http.StatusOK, "application/json", body)

	return usage, nil
}

// handleSyncToStreamPassthroughResponse handles the passthrough-path sync-to-stream
// conversion. Reads streaming response, assembles non-streaming JSON, then writes
// to the client with passthrough headers.
func (s *GatewayService) handleSyncToStreamPassthroughResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*ClaudeUsage, error) {
	if s.rateLimitService != nil {
		s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)
	}

	finalResp, usage, err := s.collectAnthropicStreamEvents(resp)
	if err != nil {
		return nil, err
	}

	body, err := marshalAnthropicResponseWithCacheDetails(finalResp, usage)
	if err != nil {
		return nil, err
	}

	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	body = reverseToolNamesIfPresent(c, body)
	c.Data(http.StatusOK, "application/json", body)

	return usage, nil
}
