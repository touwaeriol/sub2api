# 后端支撑基础设施 — 改动分析

## 1. 概述

这一域是 **支撑前 4 大指纹伪装方向的基础设施**：新增的 config 字段承载 TLS profile / sidecar 三开关 / startup probe 配置，Wire DI 连接新 service，管理后台 Settings API 暴露运行时可调开关，新增路由挂载 docs/admin/settings 相关接口，failover_loop 将错误处理从内联逻辑重构为独立状态机。`account.go` 则把原本散在各文件的 Account 结构收拢到一处，便于随 `Extra` map 挂更多指纹相关字段。

核心理念：**把所有动态功能开关都扔进 Settings 数据库**，通过管理后台热调，无需重启。

## 2. 改动 / 新增文件一览（按子域）

### Config & Wire
| 文件 | 类型 | 说明 |
|---|---|---|
| `backend/internal/config/config.go` | 改 +54 | 新增 `SidecarProbeConfig` 结构 + 3 套子配置 + 默认值 |
| `backend/cmd/server/wire.go` | 新 +5 | 顶层 wire build tag 入口 |
| `backend/cmd/server/wire_gen.go` | 改 +9/-3 | 注入新 service（sidecar / TLS profile / startup prober）|
| `backend/cmd/server/wire_gen_test.go` | 新 +2 | wire 生成测试兜底 |
| `backend/internal/service/wire.go` | 新 | Provider 集合：Claude sidecar / TLS fingerprint profile / token refresh startup prober |
| `backend/internal/service/domain_constants.go` | 新 | 把 domain 常量 re-export 到 service 包 |

### Settings API（admin）
| 文件 | 类型 | 说明 |
|---|---|---|
| `backend/internal/handler/admin/setting_handler.go` | 新 | admin 端 settings 端点（需 admin API key）|
| `backend/internal/handler/setting_handler.go` | 新 | 公开视图（登录页 / 首页品牌化读取）|
| `backend/internal/handler/dto/settings.go` | 新 | 请求/响应 DTO |
| `backend/internal/service/setting_service.go` | 新 | 业务逻辑 + 缓存 |
| `backend/internal/service/settings_view.go` | 新 | 序列化为前端视图（按登录态选择性暴露字段）|

### Routes / Handlers / 支撑
| 文件 | 类型 | 说明 |
|---|---|---|
| `backend/internal/server/router.go` | 新 +1 | 顶层路由装配 |
| `backend/internal/server/routes/admin.go` | 新 +1 | admin 挂 settings + tls-fingerprint-profile |
| `backend/internal/server/routes/docs.go` | 新 +1 | `/docs/config` + `/docs/:slug` |
| `backend/internal/handler/failover_loop.go` | 新 +19 | failover 状态机独立文件 |
| `backend/internal/web/embed_on.go` | 新 | 前端 HTML embed + CSP nonce 替换 |
| `backend/internal/service/account.go` | 新 | Account 结构集中定义 |
| `backend/internal/service/account_test_service.go` | 新 | 测试辅助 |
| `backend/internal/service/gateway_prompt_test.go` | 新 | gateway prompt 测试 |

## 3. 核心改动详解

### 3.1 新增 Config 字段 — `SidecarProbeConfig`

`Config` 结构新增一个字段：
```go
type Config struct {
    // ...
    TokenRefresh  TokenRefreshConfig  `mapstructure:"token_refresh"`
    SidecarProbe  SidecarProbeConfig  `mapstructure:"sidecar_probe"`   // ← 新增
    // ...
}
```

**完整定义**：
```go
// SidecarProbeConfig 控制 Claude OAuth 账号的 sidecar 流量注入。
//
// Anthropic 的订阅滥用检测会对只产生 /v1/messages 流量的账号静默降低
// weekly limit（典型症状：实际用量不到 2% 就触发限流）。真实 Claude
// Code CLI 会在 statusline 上周期性调用 /api/oauth/usage，也会在发送
// /v1/messages 之前发 count_tokens。此配置启用两个补丁让上游看到的
// endpoint 比例更像真人：
//   - UsagePoll:        周期性调 /api/oauth/usage
//   - CountTokensInject: 在热路径上为 /v1/messages 前置注入 count_tokens
type SidecarProbeConfig struct {
    UsagePoll         SidecarProbeUsagePollConfig `mapstructure:"usage_poll"`
    CountTokensInject SidecarProbeCTInjectConfig  `mapstructure:"count_tokens_inject"`
    StartupProbe      SidecarProbeStartupConfig   `mapstructure:"startup_probe"`
}

type SidecarProbeUsagePollConfig struct {
    Enabled            bool // 默认 true
    MinIntervalSeconds int  // 下限 60
    MaxIntervalSeconds int
    DryRun             bool // 灰度
}
type SidecarProbeCTInjectConfig struct {
    Enabled             bool // 默认 false（Phase 2）
    TimeoutMilliseconds int  // 默认 3000
}
type SidecarProbeStartupConfig struct {
    Enabled bool // 默认 true
}
```

**默认值**：
```go
viper.SetDefault("sidecar_probe.usage_poll.enabled", true)
viper.SetDefault("sidecar_probe.usage_poll.min_interval_seconds", 300) // 5 min
viper.SetDefault("sidecar_probe.usage_poll.max_interval_seconds", 900) // 15 min
viper.SetDefault("sidecar_probe.usage_poll.dry_run", false)
viper.SetDefault("sidecar_probe.count_tokens_inject.enabled", false)   // Phase 2
viper.SetDefault("sidecar_probe.count_tokens_inject.timeout_ms", 3000)
viper.SetDefault("sidecar_probe.startup_probe.enabled", true)
```

**设计要点**：
- **评论即设计文档**：`SidecarProbeConfig` 的注释直接记录了 Anthropic 的"静默降 weekly limit"行为 —— 这是该 fork 全部设计的出发点
- **三开关独立**：可分别灰度
- **60s floor**：wire 中把 `min_interval_seconds < 60` 兜底提升到 60（防 hot spin）

### 3.2 Wire DI 新连线

**`service/wire.go` 是本次改造的 DI 中心**。关键 provider：

#### `ProvideClaudeSidecarProbeService`
```go
// sidecar_probe.usage_poll.enabled 为 false 时，service 仍实例化但 Start 变 no-op
func ProvideClaudeSidecarProbeService(
    cfg *config.Config,
    accountRepo AccountRepository,
    usageService *AccountUsageService,
) *ClaudeSidecarProbeService {
    probe := cfg.SidecarProbe.UsagePoll
    if !probe.Enabled {
        return NewClaudeSidecarProbeService(accountRepo, usageService, 0, 0, false)
    }
    minInterval := time.Duration(probe.MinIntervalSeconds) * time.Second
    maxInterval := time.Duration(probe.MaxIntervalSeconds) * time.Second
    // 60s floor 防 hot spin
    if minInterval < 60*time.Second { minInterval = 60 * time.Second }
    if maxInterval < minInterval   { maxInterval = minInterval }
    // ...
    svc.Start()
    return svc
}
```

#### `ProvideTokenRefreshService` 注入 startup prober
```go
func ProvideTokenRefreshService(
    // ... 已有依赖
    gatewayService *GatewayService, // ← 新增
) *TokenRefreshService {
    svc := NewTokenRefreshService(...)
    svc.SetPrivacyDeps(privacyClientFactory, proxyRepo)
    svc.SetRefreshAPI(refreshAPI)
    svc.SetRefreshPolicy(DefaultBackgroundRefreshPolicy())

    // 注入 Claude 启动探测器：token 刷新成功后模拟 Claude Code 握手请求。
    // gatewayService 在某些测试 wire 路径下为 nil，SetStartupProber 接受 nil。
    if cfg != nil && cfg.SidecarProbe.StartupProbe.Enabled && gatewayService != nil {
        svc.SetStartupProber(gatewayService)
    }
    svc.Start()
    return svc
}
```

**Wire 集合 provider**（节选）：
```go
wire.NewSet(
    ProvideClaudeSidecarProbeService,
    NewTLSFingerprintProfileService,
    // ... 原有 provider
)
```

### 3.3 Settings API（admin）

新增 6 文件共同组成读写链：
- `handler/admin/setting_handler.go` — admin 入口
- `handler/setting_handler.go` — 公开视图
- `handler/dto/settings.go` — DTO
- `service/setting_service.go` — 业务逻辑 + 缓存
- `service/settings_view.go` — 按登录态选择性暴露字段

**核心端点**（基于前端 SettingsView 反推）：
- `GET/PUT /api/v1/admin/settings` — 100+ 项统一读写
- `GET/PUT /api/v1/admin/settings/{overload-cooldown,stream-timeout,rectifier,beta-policy,...}` — 子资源
- `GET /api/v1/admin/settings/admin-api-key` — 掩码视图
- `POST /api/v1/admin/settings/test-smtp` / `send-test-email`

**本质**：所有动态开关落库，运行时热调整，无需重启。

### 3.4 新增路由

- **`server/routes/admin.go`** 新增组：
  - `/admin/settings` + admin API key 中间件
  - `/admin/tls-fingerprint-profiles`（含 `POST /randomize-for-account/:accountID`）
- **`server/routes/docs.go`** 新增：
  - `GET /docs/config` — 文档结构
  - `GET /docs/:slug` — Markdown 内容

### 3.5 `failover_loop.go`（独立状态机）

核心类型：
```go
type FailoverAction int
const (
    FailoverContinue FailoverAction = iota  // 同账号重试或切账号
    FailoverExhausted                        // 切换次数耗尽
    FailoverCanceled                         // context 取消
)

const (
    maxSameAccountRetries     = 3                      // RetryableOnSameAccount 上限
    sameAccountRetryDelay     = 500 * time.Millisecond
    singleAccountBackoffDelay = 2 * time.Second        // 单账号分组 503 退避
)

type FailoverState struct {
    SwitchCount           int
    MaxSwitches           int
    FailedAccountIDs      map[int64]struct{}
    SameAccountRetryCount map[int64]int
    LastFailoverErr       *service.UpstreamFailoverError
    ForceCacheBilling     bool
    hasBoundSession       bool
}

type TempUnscheduler interface {
    TempUnscheduleRetryableError(ctx context.Context, accountID int64, failoverErr *service.UpstreamFailoverError)
}
```

**反指纹逻辑**：真实 Claude Code 遇 429 会停、遇 500/503 会短暂重试、遇 network error 会换连接。把这组典型模式封装为状态机，让上游看到的"错误后行为"也像真人。

**`TempUnscheduler`**：同账号重试耗尽后临时封禁，从调度池移除一段时间，避免反复打到同一个已限流账号上徒增错误率。

### 3.6 `account.go`（新增集中定义）

```go
type Account struct {
    ID, Name, Notes, Platform, Type
    Credentials map[string]any
    Extra       map[string]any   // ← TLS profile ID、tls_fingerprint_randomized、sidecar 相关全塞这
    ProxyID, Concurrency, Priority
    RateMultiplier  *float64     // 指针兼容 Redis 旧版本缓存缺字段
    LoadFactor      *int
    Status, ErrorMessage
    LastUsedAt, ExpiresAt, AutoPauseOnExpired
    CreatedAt, UpdatedAt
    Schedulable
    RateLimitedAt, RateLimitResetAt, OverloadUntil
    TempUnschedulableUntil, TempUnschedulableReason
    SessionWindowStart, SessionWindowEnd, SessionWindowStatus
    Proxy, AccountGroups, GroupIDs, Groups
    // model_mapping 热路径缓存（非持久化）
}
```

**关键字段**：
- `Extra map[string]any` → TLS profile 绑定 / 随机化标记 / 最后探测时间全在此
- `RateMultiplier *float64` → 指针兼容 Redis 旧缓存
- `TempUnschedulableUntil/Reason` → failover_loop 临时封禁用

### 3.7 `domain_constants.go` — 消除魔法字符串

把 `domain` 包常量 re-export 到 `service` 包：
```go
const (
    StatusActive   = domain.StatusActive
    StatusDisabled = domain.StatusDisabled
    StatusError    = domain.StatusError
    // ...
)
const (
    PlatformAnthropic   = domain.PlatformAnthropic
    PlatformOpenAI      = domain.PlatformOpenAI
    PlatformGemini      = domain.PlatformGemini
    PlatformAntigravity = domain.PlatformAntigravity
)
const (
    AccountTypeOAuth      = domain.AccountTypeOAuth      // full scope
    AccountTypeSetupToken = domain.AccountTypeSetupToken // inference only
    AccountTypeAPIKey     = domain.AccountTypeAPIKey
    AccountTypeUpstream   = domain.AccountTypeUpstream
    AccountTypeBedrock    = domain.AccountTypeBedrock
)
```

**作用**：
- 避免 sidecar / failover / gateway 多处写 `"active"`、`"anthropic"` 字符串
- 集中更名点
- 为 `filterSidecarProbeTargets()` 这类筛选代码提供强类型入口

### 3.8 `embed_on.go` — 前端 HTML 嵌入

设计特点：
1. **HTML 缓存 + Settings JSON 注入**：`SettingsService` 序列化结果通过 `window.__APP_CONFIG__` 注入
2. **CSP Nonce 动态替换**：`__CSP_NONCE_VALUE__` 占位符 → 每请求随机 nonce
3. **favicon / title 动态替换**：从 settings 读 `site_logo` / `site_name`
4. **文件服务优先级**：本地覆盖 > 嵌入 dist > 404

一份 dist 镜像可以承担多站点（正式 / beta / star）的不同品牌化，无需重编前端。

## 4. 关键常量清单

| 常量 | 值 | 位置 | 用途 |
|---|---|---|---|
| `maxSameAccountRetries` | 3 | failover_loop.go | 同账号重试上限 |
| `sameAccountRetryDelay` | 500ms | failover_loop.go | 同账号重试间隔 |
| `singleAccountBackoffDelay` | 2s | failover_loop.go | 单账号 503 退避 |
| sidecar min interval floor | 60s | wire.go | probe 周期下限兜底 |
| usage_poll.min default | 300s | config.go | 轮询下限（5 min）|
| usage_poll.max default | 900s | config.go | 轮询上限（15 min）|
| count_tokens.timeout default | 3000ms | config.go | 独立超时 |
| startup_probe.enabled default | **true** | config.go | 默认开 |
| count_tokens_inject.enabled default | **false** | config.go | Phase 2 灰度 |

## 5. 与前 4 个方向的关联

| 方向 | 本域支撑 |
|---|---|
| **1. TLS 指纹** | `wire.go:NewTLSFingerprintProfileService` 注入；`routes/admin.go` 挂 `/tls-fingerprint-profiles/randomize-for-account/:accountID`；Account `Extra` 承载 profile 绑定 + randomized 标记 |
| **2. HTTP 头/UA** | 改动在 `gateway_service.go`（见 02 文档）；beta 开关通过 `SystemSettings`（Settings API）运行时调整 |
| **3. Session 身份** | 无新增 config 字段，通过已有 Redis 基础设施的 `cache` 注入（见 03 文档）|
| **4. Sidecar 陪跑** | **本域的核心**：`SidecarProbeConfig` 三字段 + `ProvideClaudeSidecarProbeService` + `TokenRefreshService` 注入 startup prober + 60s floor 兜底 |

## 6. 潜在风险与观察

1. **Settings 缓存一致性**：settings 改动需所有 goroutine 观察到。本地缓存 + 多实例部署下可能有 60s 级不一致窗口。
2. **failover 状态机复杂度**：`FailoverState` 持有 5+ 字段，`maxSameAccountRetries=3` 与 `MaxSwitches` 叠加后最多可能 3×N 次上游请求，需监控总耗时。
3. **startup probe 依赖 `gatewayService != nil`**：生产环境不影响，但后续 wire 图改动需注意不要无意切断。
4. **`Account.Extra` 是 `map[string]any`**：TLS profile ID、sidecar dry-run flag 等都塞在这，没有强类型校验。容易出现 key typo（`tls_fingerprint_randomized` vs `tls_fingerprints_randomized`），建议抽公用常量或结构体。
5. **`domain_constants.go` re-export 模式**：`StatusActive = domain.StatusActive` 这类冗余写法在 domain 常量增减时需手动同步。可考虑直接让 service 引用 domain 包。
6. **`embed_on.go` CSP nonce 机制**：nonce 替换要求 HTML 模板中占位符位置固定。前端重编后 webpack chunk hash 变了，可能需要同步更新模板。

---

**总结**：本域是前 4 方向的"接插板"——配置一条龙、wire 一条龙、路由一条龙、运行时开关通过 `SystemSettings` 统一热调。关键创新在 `SidecarProbeConfig` 的完整设计（含灰度 / dry_run / Phase 2）、wire 中对 startup prober 的可选注入，以及 `failover_loop.go` 把分散重试逻辑收拢成可测试状态机。风险集中在 settings 多实例一致性和 `Account.Extra` 的非强类型。
