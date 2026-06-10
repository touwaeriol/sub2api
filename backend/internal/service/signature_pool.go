package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
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


// trySignaturePoolRetry 尝试用签名池中的签名替换请求中的 signature 并重试一次。
// 成功返回 (resp, body, true)；池为空/替换无效/重试仍失败返回 (nil, nil, false)。
func (s *GatewayService) trySignaturePoolRetry(
	ctx context.Context, c *gin.Context, account *Account,
	body, respBody []byte, groupID *int64,
	reqStream bool, token, tokenType, mappedModel string,
	shouldMimicClaudeCode bool, proxyURL string,
) (*http.Response, []byte, bool) {
	if s.signaturePoolCache == nil || !s.isSignaturePoolError(ctx, account, respBody, groupID) {
		return nil, nil, false
	}
	poolSig, err := s.signaturePoolCache.RandomGet(ctx, account.ID)
	if err != nil || poolSig == "" {
		return nil, nil, false
	}
	poolBody := ReplaceThinkingSignatures(body, poolSig)
	if bytes.Equal(poolBody, body) {
		return nil, nil, false
	}
	logger.LegacyPrintf("service.gateway", "[SignaturePool] Account %d: replacing signature from pool and retrying", account.ID)
	upstreamCtx, releaseCtx := detachStreamUpstreamContext(ctx, reqStream)
	poolReq, reqErr := s.buildUpstreamRequest(upstreamCtx, c, account, poolBody, token, tokenType, mappedModel, reqStream, shouldMimicClaudeCode)
	releaseCtx()
	if reqErr != nil {
		return nil, nil, false
	}
	poolResp, doErr := s.httpUpstream.DoWithTLS(poolReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if doErr == nil && poolResp != nil && poolResp.StatusCode < 400 {
		return poolResp, poolBody, true
	}
	if poolResp != nil && poolResp.Body != nil {
		_ = poolResp.Body.Close()
	}
	logger.LegacyPrintf("service.gateway", "[SignaturePool] Account %d: pool signature retry failed, falling through to rectifier", account.ID)
	return nil, nil, false
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

// tryHarvestStreamSignature 从 SSE data 中提取签名并异步存入池
// 提取在调用方线程完成（纯内存操作），仅在有签名时才起 goroutine 写 Redis
func (s *GatewayService) tryHarvestStreamSignature(ctx context.Context, accountID int64, data string) {
	if s.signaturePoolCache == nil || data == "" || data == "[DONE]" {
		return
	}
	sig := ExtractSignatureFromSSEData(data)
	if sig == "" {
		return
	}
	go func() {
		addCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		groupID, _ := ctx.Value(ctxkey.SignaturePoolGroupID).(*int64)
		if s.getSignaturePoolConfig(addCtx, &Account{ID: accountID}, groupID) == nil {
			return
		}
		_ = s.signaturePoolCache.Add(addCtx, accountID, sig)
	}()
}

// tryHarvestResponseSignature 从非流式响应体中提取签名并异步存入池
func (s *GatewayService) tryHarvestResponseSignature(ctx context.Context, accountID int64, body []byte) {
	if s.signaturePoolCache == nil {
		return
	}
	sigs := extractSignaturesFromResponse(body)
	if len(sigs) == 0 {
		return
	}
	go func() {
		addCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		groupID, _ := ctx.Value(ctxkey.SignaturePoolGroupID).(*int64)
		if s.getSignaturePoolConfig(addCtx, &Account{ID: accountID}, groupID) == nil {
			return
		}
		for _, sig := range sigs {
			_ = s.signaturePoolCache.Add(addCtx, accountID, sig)
		}
	}()
}
