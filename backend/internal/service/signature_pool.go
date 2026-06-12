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

// defaultSignaturePoolKeywords 用户未配置关键词时的默认值（与前端 placeholder 保持一致）
const defaultSignaturePoolKeywords = "signature,Invalid signature,thinking block"

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

// isSignaturePoolError 判断 400 错误是否为签名池可处理的签名错误。
// 匹配规则完全来自渠道配置的关键词（未配置时使用默认关键词），不依赖内置模式。
func (s *GatewayService) isSignaturePoolError(ctx context.Context, account *Account, respBody []byte, groupID *int64) bool {
	cfg := s.getSignaturePoolConfig(ctx, account, groupID)
	if cfg == nil {
		return false
	}
	keywords := strings.TrimSpace(cfg.ErrorKeywords)
	if keywords == "" {
		keywords = defaultSignaturePoolKeywords
	}
	bodyLower := strings.ToLower(string(respBody))
	for _, kw := range strings.Split(keywords, ",") {
		kw = strings.TrimSpace(strings.ToLower(kw))
		if kw != "" && strings.Contains(bodyLower, kw) {
			return true
		}
	}
	return false
}

// signaturePoolRetryParams 签名池重试所需的请求上下文参数
type signaturePoolRetryParams struct {
	C                     *gin.Context
	Account               *Account
	Body                  []byte
	RespBody              []byte
	GroupID               *int64
	ReqStream             bool
	Token                 string
	TokenType             string
	MappedModel           string
	ShouldMimicClaudeCode bool
	ProxyURL              string
}

// trySignaturePoolRetry 尝试用签名池中的签名替换请求中的 signature 并重试一次。
// 成功返回 (resp, body, true)；池为空/替换无效/重试仍失败返回 (nil, nil, false)。
func (s *GatewayService) trySignaturePoolRetry(ctx context.Context, p signaturePoolRetryParams) (*http.Response, []byte, bool) {
	if s.signaturePoolCache == nil || !s.isSignaturePoolError(ctx, p.Account, p.RespBody, p.GroupID) {
		return nil, nil, false
	}
	poolSig, err := s.signaturePoolCache.RandomGet(ctx, p.Account.ID)
	if err != nil || poolSig == "" {
		return nil, nil, false
	}
	poolBody := ReplaceThinkingSignatures(p.Body, poolSig)
	if bytes.Equal(poolBody, p.Body) {
		return nil, nil, false
	}
	logger.LegacyPrintf("service.gateway", "[SignaturePool] Account %d: replacing signature from pool and retrying", p.Account.ID)
	upstreamCtx, releaseCtx := detachStreamUpstreamContext(ctx, p.ReqStream)
	poolReq, builtPoolBody, reqErr := s.buildUpstreamRequest(upstreamCtx, p.C, p.Account, poolBody, p.Token, p.TokenType, p.MappedModel, p.ReqStream, p.ShouldMimicClaudeCode)
	releaseCtx()
	if reqErr != nil {
		return nil, nil, false
	}
	// build 阶段可能按 beta 能力进一步清理 body，回传时以实际发送的版本为准
	if builtPoolBody != nil {
		poolBody = builtPoolBody
	}
	poolResp, doErr := s.httpUpstream.DoWithTLS(poolReq, p.ProxyURL, p.Account.ID, p.Account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(p.Account))
	if doErr == nil && poolResp != nil && poolResp.StatusCode < 400 {
		return poolResp, poolBody, true
	}
	if poolResp != nil && poolResp.Body != nil {
		_ = poolResp.Body.Close()
	}
	logger.LegacyPrintf("service.gateway", "[SignaturePool] Account %d: pool signature retry failed, falling through to rectifier", p.Account.ID)
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
