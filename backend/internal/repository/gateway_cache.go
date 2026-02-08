package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"

// Gemini Trie Lua 脚本
const (
	// geminiTrieFindScript 查找最长前缀匹配的 Lua 脚本
	// KEYS[1] = trie key
	// ARGV[1] = digestChain (如 "u:a-m:b-u:c-m:d")
	// ARGV[2] = TTL seconds (用于刷新)
	// 返回: 最长匹配的 value (uuid:accountID) 或 nil
	// 查找成功时自动刷新 TTL，防止活跃会话意外过期
	// 从最长前缀（完整 chain）开始逐步缩短，第一次命中即返回
	geminiTrieFindScript = `
local chain = ARGV[1]
local ttl = tonumber(ARGV[2])

-- 先尝试完整 chain（最常见场景：同一对话的下一轮请求）
local val = redis.call('HGET', KEYS[1], chain)
if val and val ~= "" then
    redis.call('EXPIRE', KEYS[1], ttl)
    return val
end

-- 从最长前缀开始逐步缩短（去掉最后一个 "-xxx" 段）
local path = chain
while true do
    local i = string.find(path, "-[^-]*$")
    if not i or i <= 1 then
        break
    end
    path = string.sub(path, 1, i - 1)
    val = redis.call('HGET', KEYS[1], path)
    if val and val ~= "" then
        redis.call('EXPIRE', KEYS[1], ttl)
        return val
    end
end

return nil
`

	// geminiTrieSaveScript 保存会话到 Trie 的 Lua 脚本
	// KEYS[1] = trie key
	// ARGV[1] = digestChain
	// ARGV[2] = value (uuid:accountID)
	// ARGV[3] = TTL seconds
	geminiTrieSaveScript = `
local chain = ARGV[1]
local value = ARGV[2]
local ttl = tonumber(ARGV[3])
local path = ""

for part in string.gmatch(chain, "[^-]+") do
    path = path == "" and part or path .. "-" .. part
end
redis.call('HSET', KEYS[1], path, value)
redis.call('EXPIRE', KEYS[1], ttl)
return "OK"
`
)

// ============ Gemini 会话 Fallback 方法 (Trie 实现) ============

type gatewayCache struct {
	rdb *redis.Client
}

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdb}
}

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupID}:{sessionHash}
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
}

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
}

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
}

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// DeleteSessionAccountID 删除粘性会话与账号的绑定关系。
// 当检测到绑定的账号不可用（如状态错误、禁用、不可调度等）时调用，
// 以便下次请求能够重新选择可用账号。
//
// DeleteSessionAccountID removes the sticky session binding for the given session.
// Called when the bound account becomes unavailable (e.g., error status, disabled,
// or unschedulable), allowing subsequent requests to select a new available account.
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

// ============ Gemini 会话 Fallback 方法 (Trie 实现) ============

// FindGeminiSession 查找 Gemini 会话（使用 Trie + Lua 脚本实现 O(L) 查询）
// 返回最长匹配的会话信息，匹配成功时自动刷新 TTL
func (c *gatewayCache) FindGeminiSession(ctx context.Context, groupID int64, prefixHash, digestChain string) (uuid string, accountID int64, found bool) {
	if digestChain == "" {
		return "", 0, false
	}

	trieKey := service.BuildGeminiTrieKey(groupID, prefixHash)
	ttlSeconds := int(service.GeminiSessionTTL().Seconds())

	// 使用 Lua 脚本在 Redis 端执行 Trie 查找，O(L) 次 HGET，1 次网络往返
	// 查找成功时自动刷新 TTL，防止活跃会话意外过期
	result, err := c.rdb.Eval(ctx, geminiTrieFindScript, []string{trieKey}, digestChain, ttlSeconds).Result()
	if err != nil || result == nil {
		return "", 0, false
	}

	value, ok := result.(string)
	if !ok || value == "" {
		return "", 0, false
	}

	uuid, accountID, ok = service.ParseGeminiSessionValue(value)
	return uuid, accountID, ok
}

// SaveGeminiSession 保存 Gemini 会话（使用 Trie + Lua 脚本）
func (c *gatewayCache) SaveGeminiSession(ctx context.Context, groupID int64, prefixHash, digestChain, uuid string, accountID int64) error {
	if digestChain == "" {
		return nil
	}

	trieKey := service.BuildGeminiTrieKey(groupID, prefixHash)
	value := service.FormatGeminiSessionValue(uuid, accountID)
	ttlSeconds := int(service.GeminiSessionTTL().Seconds())

	return c.rdb.Eval(ctx, geminiTrieSaveScript, []string{trieKey}, digestChain, value, ttlSeconds).Err()
}

// ============ Anthropic 会话 Fallback 方法 (复用 Trie 实现) ============

// FindAnthropicSession 查找 Anthropic 会话（复用 Gemini Trie Lua 脚本）
func (c *gatewayCache) FindAnthropicSession(ctx context.Context, groupID int64, prefixHash, digestChain string) (uuid string, accountID int64, found bool) {
	if digestChain == "" {
		return "", 0, false
	}

	trieKey := service.BuildAnthropicTrieKey(groupID, prefixHash)
	ttlSeconds := int(service.AnthropicSessionTTL().Seconds())

	result, err := c.rdb.Eval(ctx, geminiTrieFindScript, []string{trieKey}, digestChain, ttlSeconds).Result()
	if err != nil || result == nil {
		return "", 0, false
	}

	value, ok := result.(string)
	if !ok || value == "" {
		return "", 0, false
	}

	uuid, accountID, ok = service.ParseGeminiSessionValue(value)
	return uuid, accountID, ok
}

// SaveAnthropicSession 保存 Anthropic 会话（复用 Gemini Trie Lua 脚本）
func (c *gatewayCache) SaveAnthropicSession(ctx context.Context, groupID int64, prefixHash, digestChain, uuid string, accountID int64) error {
	if digestChain == "" {
		return nil
	}

	trieKey := service.BuildAnthropicTrieKey(groupID, prefixHash)
	value := service.FormatGeminiSessionValue(uuid, accountID)
	ttlSeconds := int(service.AnthropicSessionTTL().Seconds())

	return c.rdb.Eval(ctx, geminiTrieSaveScript, []string{trieKey}, digestChain, value, ttlSeconds).Err()
}
