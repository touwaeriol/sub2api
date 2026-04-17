# Sidecar 陪跑（usage / count_tokens / startup probe）— 改动分析

## 1. 概述

`hai/snapshot` 对比 `v0.1.114` 新增并完整实现了 **Sidecar 陪跑** 三大模块：
- **usage 轮询**（`ClaudeSidecarProbeService`）：每 5-15 分钟（随机）调一次 `/api/oauth/usage`
- **count_tokens 注入**（`gateway_count_tokens_inject.go`）：`/v1/messages` 请求前异步注入 `/v1/messages/count_tokens`
- **startup probe**（`claude_startup_probe.go`）：token 刷新成功后触发 `max_tokens=1` haiku 握手

三个功能独立开关，均为非阻塞、容错的"副作用"流量，不影响主请求路径。

## 2. 改动 / 新增文件一览

| 文件 | 类型 | 说明 |
|---|---|---|
| `backend/internal/service/claude_sidecar_probe_service.go` | 新增 | usage 轮询主服务，独立 goroutine 循环 |
| `backend/internal/service/claude_sidecar_probe_service_test.go` | 新增 | 单元测试：targets 筛选、jitter、生命周期 |
| `backend/internal/service/claude_startup_probe.go` | 新增 | startup probe 实现 + 异步包装器 |
| `backend/internal/service/gateway_count_tokens_inject.go` | 新增 | count_tokens 注入入口 |
| `backend/internal/service/token_refresh_service.go` | 已改 | +67 行：startup prober 注入、jitter、probe 触发 |
| `backend/internal/config/config.go` | 已改 | `SidecarProbeConfig` 结构 + 3 套子配置 + 默认值 |

## 3. 核心改动详解

### 3.1 Usage 轮询调度器

**文件**：`claude_sidecar_probe_service.go`（168 行）

独立 goroutine 循环，优雅关闭：
```go
type ClaudeSidecarProbeService struct {
    accountRepo  AccountRepository
    usageService *AccountUsageService
    minInterval  time.Duration
    maxInterval  time.Duration
    dryRun       bool
    stopCh       chan struct{}
    stopOnce     sync.Once
    wg           sync.WaitGroup
}

func (s *ClaudeSidecarProbeService) Start() {
    if s == nil || s.accountRepo == nil || s.usageService == nil { return }
    if s.minInterval <= 0 || s.maxInterval < s.minInterval { return }
    s.wg.Add(1)
    go s.loop()
}
```

**随机区间**：
```go
func (s *ClaudeSidecarProbeService) jitterInterval() time.Duration {
    if s.maxInterval <= s.minInterval { return s.minInterval }
    span := int64(s.maxInterval - s.minInterval)
    return s.minInterval + time.Duration(rand.Int64N(span))
}
```
闭开区间 `[minInterval, maxInterval)`，用 `math/rand/v2` 的 `Int64N()`。

**目标筛选**（`filterSidecarProbeTargets`）：
- 仅 Claude OAuth 账户（`AccountTypeOAuth`）
- 状态 = Active
- 有 access_token（排除仅 setup-token）
- 每轮随机选一个，避免同时轰炸上游

**默认配置**：
```yaml
sidecar_probe:
  usage_poll:
    enabled: true
    min_interval_seconds: 300    # 5 分钟
    max_interval_seconds: 900    # 15 分钟
    dry_run: false
```

### 3.2 count_tokens 注入

**文件**：`gateway_count_tokens_inject.go`（98 行）

调用点：`GatewayService.Forward()` 主路径：
```go
s.maybeInjectCountTokensSidecar(account, body, reqModel, token, tokenType,
    shouldMimicClaudeCode, isClaudeCode, proxyURL)
```

**四层门槛 + 异步执行**：
```go
func (s *GatewayService) maybeInjectCountTokensSidecar(...) {
    if !s.cfg.SidecarProbe.CountTokensInject.Enabled { return }   // 开关关闭
    if account == nil || account.Platform != PlatformAnthropic || !account.IsOAuth() { return } // 仅 Claude OAuth
    if isClaudeCode { return }                                    // 真实 CC 已自发，避免重复
    if len(body) == 0 { return }                                  // 空 body

    bodyCopy := append([]byte(nil), body...)                       // 防 retry 覆写
    timeoutMs := s.cfg.SidecarProbe.CountTokensInject.TimeoutMilliseconds
    if timeoutMs <= 0 { timeoutMs = 3000 }

    go func() {
        ctx, cancel := context.WithTimeout(context.Background(),
            time.Duration(timeoutMs)*time.Millisecond)
        defer cancel()
        req, err := s.buildCountTokensRequest(ctx, nil, account, bodyCopy,
            token, tokenType, reqModel, mimicClaudeCode)
        // ... 发送，失败仅日志
    }()
}
```

**特性**：
- **非阻塞**：fire-and-forget goroutine
- **独立超时**：3s，不影响主请求
- **错误不传播**：日志到 `service.sidecar_probe`
- **body 克隆**：防止 retry 循环对原 buffer 的破坏

**默认**：`enabled: false`（Phase 2 灰度）。

### 3.3 Startup Probe（token 刷新后触发）

**文件**：`claude_startup_probe.go`（136 行）

**触发链**：
```
TokenRefreshService.postRefreshActions()
  → runStartupProbeAsync(prober, account)
    → goroutine: ctx 10s timeout → prober.ProbeClaudeStartup(ctx, account)
```

**Probe body**（canonical）：
```go
func claudeStartupProbeBody() []byte {
    body := map[string]any{
        "model":      "claude-haiku-4-5",
        "max_tokens": 1,                             // 关键：极小
        "messages":   []map[string]any{{"role":"user","content":"hi"}},
    }
    b, _ := json.Marshal(body)
    return b
}
```

**完整请求（仿真真实 CC）**：
```go
setHeaderRaw(req.Header, "authorization", "Bearer "+token)
setHeaderRaw(req.Header, "content-type", "application/json")
setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")
setHeaderRaw(req.Header, "anthropic-beta", claude.HaikuBetaHeader)
applyClaudeOAuthHeaderDefaults(req)

// 应用账户 cached fingerprint
if s.identityService != nil {
    if fp, fpErr := s.identityService.GetOrCreateFingerprint(ctx, account.ID, http.Header{}); fpErr == nil && fp != nil {
        s.identityService.ApplyFingerprint(req, fp)
    }
}

// 代理 + TLS profile + 并发控制
tlsProfile := s.tlsFPProfileService.ResolveTLSProfile(account)
resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, tlsProfile)
```

**异步包装器**：
```go
const startupProbeDefaultTimeout = 10 * time.Second

func runStartupProbeAsync(prober ClaudeStartupProber, account *Account) {
    if prober == nil || account == nil { return }
    go func() {
        ctx, cancel := context.WithTimeout(context.Background(), startupProbeDefaultTimeout)
        defer cancel()
        if err := prober.ProbeClaudeStartup(ctx, account); err != nil {
            logger.LegacyPrintf("service.sidecar_probe",
                "claude startup probe failed account=%d: %v", account.ID, err)
        }
    }()
}
```

**默认**：`enabled: true`。

### 3.4 三个开关的配置入口

**`config.go`**：
```go
type SidecarProbeConfig struct {
    UsagePoll         SidecarProbeUsagePollConfig `mapstructure:"usage_poll"`
    CountTokensInject SidecarProbeCTInjectConfig  `mapstructure:"count_tokens_inject"`
    StartupProbe      SidecarProbeStartupConfig   `mapstructure:"startup_probe"`
}

type SidecarProbeUsagePollConfig struct {
    Enabled            bool `mapstructure:"enabled"`
    MinIntervalSeconds int  `mapstructure:"min_interval_seconds"`
    MaxIntervalSeconds int  `mapstructure:"max_interval_seconds"`
    DryRun             bool `mapstructure:"dry_run"`
}

type SidecarProbeCTInjectConfig struct {
    Enabled             bool `mapstructure:"enabled"`
    TimeoutMilliseconds int  `mapstructure:"timeout_ms"`
}

type SidecarProbeStartupConfig struct {
    Enabled bool `mapstructure:"enabled"`
}
```

**默认值**：
```go
viper.SetDefault("sidecar_probe.usage_poll.enabled", true)
viper.SetDefault("sidecar_probe.usage_poll.min_interval_seconds", 300)
viper.SetDefault("sidecar_probe.usage_poll.max_interval_seconds", 900)
viper.SetDefault("sidecar_probe.usage_poll.dry_run", false)
viper.SetDefault("sidecar_probe.count_tokens_inject.enabled", false)   // Phase 2
viper.SetDefault("sidecar_probe.count_tokens_inject.timeout_ms", 3000)
viper.SetDefault("sidecar_probe.startup_probe.enabled", true)
```

**Wire 注入**（`service/wire.go`）：
```go
func ProvideClaudeSidecarProbeService(cfg, accountRepo, usageService) *ClaudeSidecarProbeService {
    probe := cfg.SidecarProbe.UsagePoll
    if !probe.Enabled {
        return NewClaudeSidecarProbeService(accountRepo, usageService, 0, 0, false)
    }
    svc := NewClaudeSidecarProbeService(accountRepo, usageService,
        time.Duration(probe.MinIntervalSeconds)*time.Second,
        time.Duration(probe.MaxIntervalSeconds)*time.Second,
        probe.DryRun)
    svc.Start()
    return svc
}

// token_refresh_service 注入 prober
if cfg != nil && cfg.SidecarProbe.StartupProbe.Enabled && gatewayService != nil {
    svc.SetStartupProber(gatewayService)
}
```

### 3.5 Token Refresh Service 反指纹修改（+67 行）

1. **新增 startup prober 字段**：
```go
// nil 表示禁用
startupProber ClaudeStartupProber

func (s *TokenRefreshService) SetStartupProber(p ClaudeStartupProber) {
    s.startupProber = p
}
```

2. **jitter 算法（避免规律性刷新）**：
```go
func jitteredInterval(base time.Duration, pct float64) time.Duration {
    // [base*(1-pct), base*(1+pct)]
}

timer := time.NewTimer(jitteredInterval(baseInterval, 0.25))  // ±25%
for {
    select {
    case <-timer.C:
        s.processRefresh()
        timer.Reset(jitteredInterval(baseInterval, 0.25))
    }
}
```

3. **per-account 刷新窗口 jitter**：
```go
jitterFactor := 0.6 + rand.Float64()*0.8     // [0.6, 1.4]
perAccountWindow := time.Duration(float64(refreshWindow) * jitterFactor)
if !refresher.NeedsRefresh(account, perAccountWindow) { ... }
```

4. **启动随机延迟**：
```go
startupDelay := time.Duration(rand.Int64N(int64(30 * time.Second)))
startupTimer := time.NewTimer(startupDelay)
select {
case <-startupTimer.C:
case <-s.stopCh:
    return
}
s.processRefresh()
```

5. **Token 刷新后触发 probe**（`postRefreshActions`）：
```go
// Fire-and-forget：探测失败不影响刷新
if account.Platform == PlatformAnthropic && account.IsOAuth() && s.startupProber != nil {
    runStartupProbeAsync(s.startupProber, account)
}
```

## 4. 关键常量与超时

| 项目 | 值 | 含义 |
|---|---|---|
| Usage 轮询最小间隔 | 300s (5 min) | 配置下限 |
| Usage 轮询最大间隔 | 900s (15 min) | 上限，jitter 范围 |
| count_tokens 超时 | 3000ms | 独立超时 |
| startup probe 超时 | 10s | 握手超时 |
| token 刷新间隔 jitter | ±25% | 避免整点刷新 |
| per-account refresh jitter | [0.6, 1.4] | 避免多账号齐发 |
| 启动延迟 | 0-30s 随机 | 避免多实例同步 |

## 5. 对 5 大设计表的对照

| 表格项 | 实现位置 | 状态 |
|---|---|---|
| Usage 轮询 5-15min 随机晃号 | `ClaudeSidecarProbeService.jitterInterval()` | ✅ |
| count_tokens 注入 | `GatewayService.maybeInjectCountTokensSidecar()` | ✅（默认关，Phase 2）|
| startup probe max_tokens=1（token 刷新后触发）| `claude_startup_probe.go` + `TokenRefreshService.postRefreshActions()` | ✅ |
| 三开关独立 | `config.go` + `wire.go` 三个独立 `enabled` | ✅ |
| 反指纹 jitter | `token_refresh_service.go` 的 `jitteredInterval` + per-account 抖动 | ✅（超出表格，额外收益）|

## 6. 潜在风险与观察

1. **count_tokens 默认关闭**：Phase 2 灰度，尚未生效。需先观测 usage poll 效果再启用。
2. **错误完全容错**：三功能都 fire-and-forget，失败只记日志。对访问模式伪装有利，但 sidecar 流量本身问题难排查。
3. **Startup probe 依赖 `gatewayService`**：wire 中判 `!= nil`，某些测试路径可能跳过。
4. **DryRun 仅 usage poll**：其他两个无灰度机制，只能全开/全关。
5. **指纹复用**：startup probe 用账户 cached fingerprint 保持与真实流量一致；fingerprint service 故障会退化为无指纹请求。
6. **多实例启动同步**：`startupDelay` 0-30s，高并发时仍可能有微弱时间相关性，但已比固定 Ticker 显著改善。

---

**总结**：完整实现 Sidecar 陪跑三大需求；均独立可配、关键路径容错。token refresh 的 jitter 优化是"超出表格"的额外收益。目前 count_tokens 灰度中，其余两个已就绪。
