# Session 身份 / sticky session / cache_control — 改动分析

## 1. 概述

hai/snapshot 分支针对 Session 身份的改动围绕 **反指纹识别** 与 **会话复用** 两个核心问题：

1. **v0.1.114 的问题**：
   - `session_id` 由确定性哈希生成（`SHA256(accountID::originalSessionID)`），同一账户的 session_id 呈现有限循环模式，易被指纹识别
   - 多个并发请求会共享相同的 15 分钟缓存 session_id，不符合真实 Claude Code CLI 行为

2. **hai/snapshot 的改进**：
   - 使用 `crypto/rand` 的 UUIDv4 替代确定性哈希，每次新会话生成真随机 UUID
   - 引入 **sticky session** 机制：30 分钟窗口内按 `sessionHash` 复用同一 session_id
   - tools 数组末尾自动注入 `cache_control: {"type":"ephemeral"}`，对齐官方 CLI 的 prompt caching 行为

## 2. 改动文件一览

| 文件 | 行数变化 | 核心改动 |
|---|---|---|
| `backend/internal/service/identity_service.go` | +186 | session_id 生成逻辑、sticky session 上下文、`RewriteUserIDWithMasking` 重构 |
| `backend/internal/repository/identity_cache.go` | +35/-4 | 新增 `stickySessionUUIDTTL`、`GetStickySessionUUID`、`SetStickySessionUUID` |
| `backend/internal/service/identity_service_order_test.go` | +74 | sticky session 单元测试、缓存存根 |
| `backend/internal/service/gateway_service.go` | N/A | 调用链支持、`cache_control` 注入 |

## 3. 核心改动详解

### 3.1 session_id 生成：crypto/rand UUIDv4（从确定性到随机）

**v0.1.114（已删除）：**
```go
func generateUUIDFromSeed(seed string) string {
    hash := sha256.Sum256([]byte(seed))
    bytes := hash[:16]
    return fmt.Sprintf("%x-%x-%x-%x-%x", ...)
}

seed := fmt.Sprintf("%d::%s", accountID, sessionTail)
newSessionHash := generateUUIDFromSeed(seed)  // 确定性！
```

**问题**：同一账户的 session_id 永远相同，对手可轻易识别。

**hai/snapshot（新增）：**
```go
func generateRandomUUID() string {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil {
        h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
        b = h[:16]
    }
    b[6] = (b[6] & 0x0f) | 0x40  // UUID v4 版本位
    b[8] = (b[8] & 0x3f) | 0x80  // UUID 变体位
    return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

newSessionHash := generateRandomUUID()  // 密码学强随机
```

### 3.2 sticky session 复用机制（30 分钟窗口）

这是本次改动的**核心创新**，解决"随机化过度→每次都换 session_id 不像真实 CLI"的矛盾。

#### 3.2.1 上下文传递：`WithSessionHash`

Gateway 入口注入 sessionHash：
```go
// gateway_service.go Forward()
ctx = WithSessionHash(ctx, s.GenerateSessionHash(parsed))
```

```go
type sessionHashCtxKeyType struct{}
var sessionHashCtxKey sessionHashCtxKeyType

func WithSessionHash(ctx context.Context, sessionHash string) context.Context {
    if sessionHash == "" {
        return ctx
    }
    return context.WithValue(ctx, sessionHashCtxKey, sessionHash)
}

func SessionHashFromContext(ctx context.Context) string {
    if ctx == nil {
        return ""
    }
    v, _ := ctx.Value(sessionHashCtxKey).(string)
    return v
}
```

**`GenerateSessionHash()` 优先级：**
1. 从请求 `metadata.user_id` 提取 session_id 字段（如果客户端已声明）
2. 从带 `cache_control: ephemeral` 的内容生成哈希（对齐 prompt caching 内容）
3. 综合 ClientIP、UA、APIKeyID、system、messages 生成内容摘要哈希

#### 3.2.2 Redis 缓存：30 分钟 TTL

```go
const (
    stickySessionUUIDKeyPrefix = "sticky_session_uuid:"
    stickySessionUUIDTTL       = 30 * time.Minute
    // 30 分钟 TTL 贴近真实 Claude Code CLI 一次会话的典型生命周期
)

func stickySessionUUIDKey(accountID int64, sessionHash string) string {
    return fmt.Sprintf("%s%d:%s", stickySessionUUIDKeyPrefix, accountID, sessionHash)
}
// 示例：sticky_session_uuid:42:7578cf37-aaca-46e4-a45c-71285d9dbb83
```

**接口方法：**
```go
type IdentityCache interface {
    GetStickySessionUUID(ctx context.Context, accountID int64, sessionHash string) (string, error)
    SetStickySessionUUID(ctx context.Context, accountID int64, sessionHash string, sessionUUID string) error
}
```

**Redis 实现：**
```go
func (c *identityCache) GetStickySessionUUID(ctx, accountID, sessionHash) (string, error) {
    if sessionHash == "" { return "", nil }
    val, err := c.rdb.Get(ctx, stickySessionUUIDKey(accountID, sessionHash)).Result()
    if err == redis.Nil { return "", nil }  // 未命中不算错误
    return val, err
}

func (c *identityCache) SetStickySessionUUID(ctx, accountID, sessionHash, sessionUUID) error {
    if sessionHash == "" || sessionUUID == "" { return nil }
    return c.rdb.Set(ctx, stickySessionUUIDKey(accountID, sessionHash), sessionUUID, stickySessionUUIDTTL).Err()
}
```

#### 3.2.3 `RewriteUserIDWithMasking` 的新逻辑

**重要变化**：函数不再依赖账户 `IsSessionIDMaskingEnabled()` 标志，改为根据上下文的 `sessionHash` 决定是否走 sticky。

```go
func (s *IdentityService) RewriteUserIDWithMasking(ctx, body, account, accountUUID, cachedClientID, fingerprintUA) ([]byte, error) {
    if account == nil { return body, nil }
    sessionHash := SessionHashFromContext(ctx)

    // 无 sessionHash 或无缓存 → 降级到随机每次
    if sessionHash == "" || s.cache == nil {
        return s.RewriteUserID(body, account.ID, accountUUID, cachedClientID, fingerprintUA)
    }

    sessionUUID, err := s.cache.GetStickySessionUUID(ctx, account.ID, sessionHash)
    if err != nil {
        logger.LegacyPrintf("service.identity", "sticky session uuid lookup failed... falling back to random")
        sessionUUID = ""
    }
    if sessionUUID == "" {
        sessionUUID = generateRandomUUID()
        if setErr := s.cache.SetStickySessionUUID(ctx, account.ID, sessionHash, sessionUUID); setErr != nil {
            logger.LegacyPrintf("service.identity", "sticky session uuid persist failed...")
        }
    }

    version := ExtractCLIVersion(fingerprintUA)
    newUserID := FormatMetadataUserID(cachedClientID, accountUUID, sessionUUID, version)
    // ... sjson 写回
}
```

**调用流程：**
```
Forward()
  → GenerateSessionHash(parsed) → sessionHash
  → ctx = WithSessionHash(ctx, sessionHash)
  → RewriteUserIDWithMasking(ctx, ...)
    → SessionHashFromContext(ctx)
    → cache.GetStickySessionUUID(accountID, sessionHash)
       ├ 命中 → 复用缓存 UUID（30min 内所有同 hash 请求相同）
       └ 未命中 → generateRandomUUID() → Set 缓存
    → FormatMetadataUserID(..., sessionUUID, ...)
```

### 3.3 tools cache_control 注入

真实 Claude Code CLI 在 tools 数组末尾标记 `cache_control: {"type": "ephemeral"}` 启用 prompt caching。Cherry Studio、Cursor、OpenAI SDK wrapper 等客户端通常不带 → 缓存命中率 0 → 易被指纹识别。

```go
func ensureToolsCacheControl(body []byte) []byte {
    if len(body) == 0 { return body }
    tools := gjson.GetBytes(body, "tools")
    if !tools.IsArray() { return body }
    arr := tools.Array()
    if len(arr) == 0 { return body }

    // 已有任何 cache_control 则尊重客户端，不改
    for _, t := range arr {
        if t.Get("cache_control").Exists() { return body }
    }

    // 只在最后一项注入
    lastIdx := len(arr) - 1
    path := fmt.Sprintf("tools.%d.cache_control", lastIdx)
    out, err := sjson.SetBytes(body, path, map[string]string{"type": "ephemeral"})
    if err != nil { return body }
    return out
}
```

**注入规则：**
- 仅注入 tools 数组**末尾**一项
- 仅在所有 tool 都**没有** `cache_control` 时触发
- 失败静默降级（返回原 body）
- 与 `enforceCacheControlLimit` 4 块上限无冲突（只加 1 块）

### 3.4 测试用例揭示的行为

#### 3.4.1 `RewriteUserIDWithMasking_PreservesTopLevelFieldOrder`（已修改）

注释说明：
> 2026-04-15: session_id masking was removed for anti-fingerprinting reasons — it forced every concurrent request on one account to share the same 15-minute-cached session UUID, which is trivially detectable as non-human.

关键断言：
```go
require.NotContains(t, resultStr, "11111111-2222-4333-8444-555555555555",
    "masked session id must no longer leak through")
// 新增：两次调用必须产生不同 session_id
require.NotEqual(t, resultStr, string(result2), "session id must differ across calls")
```

#### 3.4.2 `RewriteUserIDWithMasking_StickyPerSessionHash`（新增）

核心验证：
```go
// 相同 sessionHash → 相同 session_id
ctxA := WithSessionHash(context.Background(), "conversation-A")
first,  _ := svc.RewriteUserIDWithMasking(ctxA, body, account, ...)
second, _ := svc.RewriteUserIDWithMasking(ctxA, body, account, ...)
require.Equal(t, string(first), string(second), "same session hash must yield sticky session id")

// 不同 sessionHash → 不同 session_id
ctxB := WithSessionHash(context.Background(), "conversation-B")
other, _ := svc.RewriteUserIDWithMasking(ctxB, body, account, ...)
require.NotEqual(t, string(first), string(other))
```

## 4. 关键常量与 TTL

| 常量 | 值 | 备注 |
|---|---|---|
| `stickySessionUUIDKeyPrefix` | `"sticky_session_uuid:"` | Redis key 前缀 |
| `stickySessionUUIDTTL` | `30 * time.Minute` | **硬编码**，不可配置 |
| `maskedSessionTTL` | `15 * time.Minute` | 旧机制保留（未再使用）|
| `fingerprintTTL` | `7 * 24 * time.Hour` | 指纹缓存 7 天 |

## 5. 对 5 大设计表的对照

| 表格项 | 实现位置 | 状态 |
|---|---|---|
| session_id 改 crypto/rand UUIDv4 去确定性 | `identity_service.go:generateRandomUUID()` | ✅ |
| 同会话 30min sticky 复用 | `identity_cache.go:stickySessionUUIDTTL` + `GetStickySessionUUID` | ✅ |
| tools 末尾补 cache_control: ephemeral | `gateway_service.go:ensureToolsCacheControl()` | ✅ |

## 6. 潜在风险与观察

### 6.1 Redis 故障降级

Redis 宕机时自动降级到"每次随机"，30 分钟窗口的 session 一致性会被破坏。可接受但会暴露"某账号在 Redis 故障期间 session_id 突然高频变化"的异常模式，可能被检测系统捕获。

### 6.2 sessionHash 生成策略的多层 fallback

`GenerateSessionHash` 三级优先级：
- P1：客户端请求中的 `metadata.user_id` sessionID
- P2：`cache_control: ephemeral` 内容哈希
- P3：ClientIP + UA + APIKeyID + system + messages 摘要

同一会话在不同客户端语义下可能产生不同 hash，导致 sticky 段被意外切分。

### 6.3 30 分钟 TTL 硬编码

`stickySessionUUIDTTL = 30 * time.Minute` 是硬编码常量，不受配置影响。若后续想按账号/用户/场景调整窗口大小，需改代码。

### 6.4 cache_control 注入时机

在 Forward() 较晚阶段注入（system 重写之后）。对 4 块上限留有余量，但如果后续流程再注入其他 cache_control（如 system 缓存），可能触达 4 块上限，需要 `enforceCacheControlLimit` 兜底。

### 6.5 旧方法保留但未使用

`GetMaskedSessionID` / `SetMaskedSessionID` 方法仍存在于 `IdentityCache` 接口中，但新代码路径已不调用。存在以下风险：
- 接口方法冗余，可能被意外引用复活旧行为
- 旧的 15 分钟 masked_session 缓存会自然过期，短期内 Redis 里可能同时存两种 key

建议在稳定后清理旧方法。

---

**文档生成时间**：2026-04-18
**对比基线**：v0.1.114 tag vs hai/snapshot branch
