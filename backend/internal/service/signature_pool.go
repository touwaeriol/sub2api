package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const featureKeySignaturePool = "signature_pool"

// SignaturePoolConfig 签名池渠道级配置
type SignaturePoolConfig struct {
	Enabled       bool   `json:"enabled"`
	ErrorKeywords string `json:"error_keywords"`
}

// GetSignaturePoolConfig 从渠道 features_config 中读取签名池配置
func (c *Channel) GetSignaturePoolConfig() *SignaturePoolConfig {
	if c == nil || c.FeaturesConfig == nil {
		return nil
	}
	raw, ok := c.FeaturesConfig[featureKeySignaturePool]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	cfg := &SignaturePoolConfig{}
	if enabled, ok := m["enabled"].(bool); ok {
		cfg.Enabled = enabled
	}
	if kw, ok := m["error_keywords"].(string); ok {
		cfg.ErrorKeywords = kw
	}
	return cfg
}

// SignaturePoolCache 签名池的 Redis 存储接口
type SignaturePoolCache interface {
	Add(ctx context.Context, accountID int64, signature string) error
	RandomGet(ctx context.Context, accountID int64) (string, error)
	Size(ctx context.Context, accountID int64) (int64, error)
}

// getSignaturePoolConfig 检查渠道是否启用了签名池，返回配置（未启用返回 nil）
func (s *GatewayService) getSignaturePoolConfig(ctx context.Context, account *Account, groupID *int64) *SignaturePoolConfig {
	if groupID == nil || s.channelService == nil {
		return nil
	}
	ch, err := s.channelService.GetChannelForGroup(ctx, *groupID)
	if err != nil || ch == nil {
		return nil
	}
	cfg := ch.GetSignaturePoolConfig()
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	return cfg
}

// isSignaturePoolError 判断 400 错误是否为签名池可处理的签名错误
func (s *GatewayService) isSignaturePoolError(ctx context.Context, account *Account, respBody []byte, groupID *int64) bool {
	cfg := s.getSignaturePoolConfig(ctx, account, groupID)
	if cfg == nil {
		return false
	}
	if s.isThinkingBlockSignatureError(respBody) {
		return true
	}
	if cfg.ErrorKeywords != "" {
		bodyLower := strings.ToLower(string(respBody))
		for _, kw := range strings.Split(cfg.ErrorKeywords, ",") {
			kw = strings.TrimSpace(strings.ToLower(kw))
			if kw != "" && strings.Contains(bodyLower, kw) {
				return true
			}
		}
	}
	return false
}


// ReplaceThinkingSignatures 用池中签名替换请求体中所有 thinking block 的 signature
func ReplaceThinkingSignatures(body []byte, poolSignature string) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}
	result := body
	var err error
	for mi, msg := range messages.Array() {
		content := msg.Get("content")
		if !content.Exists() || !content.IsArray() {
			continue
		}
		for bi, block := range content.Array() {
			if block.Get("type").String() == "thinking" && block.Get("signature").Exists() {
				path := fmt.Sprintf("messages.%d.content.%d.signature", mi, bi)
				if result, err = sjson.SetBytes(result, path, poolSignature); err != nil {
					return body
				}
			}
		}
	}
	return result
}

func extractSignaturesFromResponse(body []byte) []string {
	var sigs []string
	content := gjson.GetBytes(body, "content")
	if !content.Exists() || !content.IsArray() {
		return nil
	}
	for _, block := range content.Array() {
		if block.Get("type").String() == "thinking" {
			if sig := strings.TrimSpace(block.Get("signature").String()); sig != "" {
				sigs = append(sigs, sig)
			}
		}
	}
	return sigs
}

// ExtractSignatureFromSSEData 从流式 content_block_delta 事件中提取 signature
func ExtractSignatureFromSSEData(data string) string {
	if gjson.Get(data, "type").String() != "content_block_delta" {
		return ""
	}
	delta := gjson.Get(data, "delta")
	if delta.Get("type").String() != "signature_delta" {
		return ""
	}
	return strings.TrimSpace(delta.Get("signature").String())
}

// tryHarvestStreamSignature 异步从 SSE data 中提取签名存入池（全部异步，零阻塞）
func (s *GatewayService) tryHarvestStreamSignature(ctx context.Context, accountID int64, data string) {
	if s.signaturePoolCache == nil || data == "" || data == "[DONE]" {
		return
	}
	go func() {
		sig := ExtractSignatureFromSSEData(data)
		if sig == "" {
			return
		}
		groupID, _ := ctx.Value(ctxkey.SignaturePoolGroupID).(*int64)
		if s.getSignaturePoolConfig(context.Background(), &Account{ID: accountID}, groupID) == nil {
			return
		}
		_ = s.signaturePoolCache.Add(context.Background(), accountID, sig)
	}()
}

// tryHarvestResponseSignature 异步从非流式响应体中提取签名存入池（全部异步，零阻塞）
func (s *GatewayService) tryHarvestResponseSignature(ctx context.Context, accountID int64, body []byte) {
	if s.signaturePoolCache == nil {
		return
	}
	go func() {
		groupID, _ := ctx.Value(ctxkey.SignaturePoolGroupID).(*int64)
		if s.getSignaturePoolConfig(context.Background(), &Account{ID: accountID}, groupID) == nil {
			return
		}
		for _, sig := range extractSignaturesFromResponse(body) {
			_ = s.signaturePoolCache.Add(context.Background(), accountID, sig)
		}
	}()
}
