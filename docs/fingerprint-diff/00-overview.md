# sub2api-hai fork — 指纹伪装改造总览

> 分析基线：上游官方 **v0.1.114** tag 与 **hai/snapshot** 分支的 diff
> 改动范围：71 个文件、+6416 / -1247 行
> 仓库位置：本文档所在的 `sub2api-hai-wt` 是我们的 `sub2api` 仓库里的 `hai/snapshot` 分支 worktree

## 1. 一句话概括

这个 fork 的目的是 **让 sub2api 网关的出站流量在 TLS 层、HTTP 层、会话层、访问模式层都伪装成真实的 Claude Code CLI（2.1.11x）客户端**，规避 Anthropic 的"订阅滥用检测"对网关型账号的静默降 weekly-limit 行为。

触发动机见 `backend/internal/config/config.go` 中 `SidecarProbeConfig` 的注释原文：
> Anthropic 的订阅滥用检测会对只产生 /v1/messages 流量的账号静默降低 weekly limit（典型症状：实际用量不到 2% 就触发限流）。

## 2. 五大改造方向 — 跨域落地表

| # | 方向 | 主要落地文件 | 状态 | 详情文档 |
|---|---|---|---|---|
| **1** | **TLS 指纹**：对齐 CC 2.1.109 ClientHello + ALPN 过滤 h2 + 每账号 JA3 随机化 | `pkg/tlsfingerprint/{dialer,randomizer}.go`、`service/tls_fingerprint_profile_service.go`、`handler/admin/tls_fingerprint_profile_handler.go` | ✅ 完整 | [01-tls-fingerprint.md](./01-tls-fingerprint.md) |
| **2** | **HTTP 头 / UA**：UA 2.1.22→2.1.112、beta 三 token 补齐、Timeout 600→300、Retry-Count 97/2.5/0.5% 随机 | `pkg/claude/constants.go`、`service/gateway_service.go` | ✅ 完整 | [02-http-headers-ua.md](./02-http-headers-ua.md) |
| **3** | **Session 身份**：session_id 改 crypto/rand UUIDv4、30 min sticky 复用、tools 末尾补 `cache_control: ephemeral` | `service/identity_service.go`、`repository/identity_cache.go` | ✅ 完整 | [03-session-identity.md](./03-session-identity.md) |
| **4** | **Sidecar 陪跑**：usage 轮询 5-15 min 随机、count_tokens 注入、token 刷新后 startup probe；**三开关独立** | `service/claude_sidecar_probe_service.go`、`claude_startup_probe.go`、`gateway_count_tokens_inject.go`、`token_refresh_service.go` | ✅ 完整（count_tokens 默认关，Phase 2）| [04-sidecar-probe.md](./04-sidecar-probe.md) |
| **5** | **工具链**：`capture_fingerprint` 抓 ClientHello + HTTP/2、`verify_fingerprint` 对比、2.1.111 baseline | `tools/capture_fingerprint/main.go`、`tools/verify_fingerprint/main.go`、`tools/capture_fingerprint/baselines/claude-code-2.1.111.json` | ✅ 工具可用，verify 自动化是手动 `jq` 对比 | [01-tls-fingerprint.md §3.4-3.6](./01-tls-fingerprint.md) |
| **+** | **支撑基础设施**（配置 / DI / Settings API / 路由 / failover 状态机）| `config/config.go`、`service/wire.go`、`handler/admin/setting_handler.go`、`handler/failover_loop.go` | ✅ | [05-backend-infra.md](./05-backend-infra.md) |
| **+** | **品牌化与 UX**（首页重做 / DocsView / SettingsView / Claude 主题色 / 账号 Modal 扩展）| `frontend/src/views/*`、`tailwind.config.js`、`index.html` | ✅ | [06-frontend-misc.md](./06-frontend-misc.md) |

> 📌 表格中 5 大方向的每一项都已**完整落地**，没有"表格描述但代码缺失"的项。唯一的部分实现：`count_tokens_inject` 默认 `false`（Phase 2 灰度），但代码路径已就绪。

## 3. 架构分层视图

```
┌─────────────────── 请求入口 ───────────────────┐
│  前端（Claude 品牌化 UI）                      │
│    ├─ SettingsView（admin 运行时调参）         │
│    ├─ CreateAccountModal / EditAccountModal    │
│    │     └─ "随机化 TLS" 按钮                  │
│    └─ DocsView（/docs/*，用户自助文档）        │
└────────────────────┬───────────────────────────┘
                     │
┌────────────────────▼───────────────────────────┐
│ Gateway Service（service/gateway_service.go）  │
│  ├─ Forward()                                  │
│  │   ├─ GenerateSessionHash → WithSessionHash  │ ◄── 方向 3
│  │   ├─ ensureToolsCacheControl()              │ ◄── 方向 3
│  │   ├─ applyClaudeCodeMimicHeaders()          │ ◄── 方向 2
│  │   │     └─ SampleStainlessRetryCount 97/2.5/0.5%
│  │   ├─ RewriteUserIDWithMasking() 30min sticky│ ◄── 方向 3
│  │   └─ maybeInjectCountTokensSidecar()        │ ◄── 方向 4
│  └─ httpUpstream.DoWithTLS()                   │
│        └─ tlsfingerprint.Dialer                │ ◄── 方向 1
│              ├─ 52 cipher / 26 sig / 5 curves  │
│              ├─ filterHTTP2FromALPN (强制 h1)  │
│              └─ per-account randomized profile │
└────────────────────┬───────────────────────────┘
                     │
┌────────────────────▼───────────────────────────┐
│ 后台陪跑（独立 goroutine）                     │
│  ├─ ClaudeSidecarProbeService（usage 轮询）    │ ◄── 方向 4
│  │     └─ 5-15 min 随机 jitterInterval         │
│  └─ TokenRefreshService                        │
│        ├─ ±25% jitter（反指纹刷新模式）        │
│        └─ postRefreshActions                   │
│              └─ runStartupProbeAsync           │ ◄── 方向 4
│                    └─ ProbeClaudeStartup       │
│                          max_tokens=1, haiku   │
└─────────────────────────────────────────────────┘

┌─────────────── 工具链（离线）─────────────────┐
│  capture_fingerprint                           │ ◄── 方向 5
│     ├─ 本地 HTTPS+H2 server                    │
│     ├─ peekConn 抓 ClientHello                 │
│     ├─ utls.Fingerprinter 解析                 │
│     └─ JA3 + HTTP/2 帧序列 → JSON              │
│        baselines/claude-code-2.1.111.json      │
│  verify_fingerprint                            │
│     └─ 用 dialer 的 default profile 拨号       │
└─────────────────────────────────────────────────┘
```

## 4. 改动量 & 文件类型分布

**Diff 总览**（v0.1.114 → hai/snapshot）：
- 71 个文件变更
- +6416 行 / -1247 行
- 43 个"纯新增内容"文件、3 个"纯删除内容"文件、25 个修改

**前十大改动文件**（按 +行数）：
| 文件 | +行 | 域 |
|---|---|---|
| `frontend/src/views/HomeView.vue` | 1361 | 06（品牌化首页）|
| `tools/check_pnpm_audit_exceptions.py` | 492 | 06（重构）|
| `frontend/src/components/layout/AuthLayout.vue` | 335 | 06 |
| `backend/internal/pkg/tlsfingerprint/dialer.go` | 184 | 01 |
| `frontend/src/i18n/locales/en.ts` | 218 | 06 |
| `frontend/src/i18n/locales/zh.ts` | 215 | 06 |
| `backend/internal/service/identity_service.go` | 186 | 03 |
| `backend/internal/service/tls_fingerprint_profile_service.go` | 116 | 01 |
| `backend/internal/pkg/claude/constants.go` | 67 | 02 |
| `backend/internal/service/token_refresh_service.go` | 77 | 04 |

前端 i18n + UI 体量最大（品牌化），后端单文件冠军是 TLS `dialer.go`。

## 5. 跨域设计亮点

### 5.1 分层防御：单点失效不会全崩
- 即使 TLS 指纹被上游识破（聚合 JA3 检测），HTTP 头层的 beta token + UA 变化还能起作用
- count_tokens 注入失败不会影响主请求（fire-and-forget + 独立 timeout）
- Redis 故障时 sticky session 降级为"每次随机"，session_id 一致性丢失但不影响功能

### 5.2 "注释即文档"
- `SidecarProbeConfig` 的 godoc 直接说明了 Anthropic 的检测机制（静默降 weekly-limit）
- `RewriteUserIDWithMasking` 测试注释记录了 2026-04-15 去除 masking 的原因
- `dialer.go` 的 cipher 常量注释引用 baseline JSON 文件证明数据来源

### 5.3 设计保守性
- cipher 只做**局部交换**而非全 shuffle — 保留 Node.js 轮廓
- ALPN **强制过滤 h2**（不是开关可选）— 因为 Go `http.Transport` 架构限制
- 60s 最小间隔 floor — 防止配错后 hot spin
- count_tokens inject **默认关**（Phase 2）— 先观察 usage poll 效果再放开

### 5.4 可观测性 + 可运维性
- 所有动态开关落 Settings 数据库 → 管理后台热调，无需重启
- 账号级 TLS profile 随机化幂等（自动清理旧 `__auto__:` profile）
- Dry-run 模式（至少 usage_poll 有）方便灰度
- capture/verify 工具支持持续对齐 CC 版本更新

## 6. 风险清单（跨域汇总）

| # | 风险 | 所在域 | 严重度 | 建议 |
|---|---|---|---|---|
| R1 | **JA3 聚合检测**：即便 per-account 随机，仍在 Node.js 轮廓内，Anthropic 若改为"检测来自 Node.js 但不是真实 CC"，措施可能失效 | 01 | 高 | 持续用 `verify_fingerprint` 对齐最新 CC 版本 |
| R2 | **baseline 版本绑定**：`claude-code-2.1.111.json` 硬编文件名；CC 升级需新增文件 + 改 dialer 默认值，无自动化流程 | 01 / 05 | 中 | 加 CI 任务定期 capture + diff 报警 |
| R3 | **30 min sticky TTL 硬编码**：不可配置，无法按账号/场景调 | 03 | 低 | 后续如有需求提升为配置项 |
| R4 | **Redis 故障降级**：sticky session 丢失 → 同账号短时间 session_id 高频变化，反而可能暴露 | 03 | 中 | 加 Redis 健康探活告警 |
| R5 | **Settings 多实例一致性**：本地缓存 + 多实例部署可能有 60s 级不一致窗口 | 05 | 中 | 关注 setting_service 缓存 TTL / 广播设计 |
| R6 | **`Account.Extra` 非强类型 map**：TLS profile ID / randomize flag / dry-run 全塞 `map[string]any`，容易 typo | 05 | 低-中 | 抽公用常量 key 或结构体 |
| R7 | **Retry 采样随机可预测模式**：`math/rand/v2` 短时间窗口内仍有统计可预测性 | 02 | 低 | 必要时结合时间戳 hash 去规律 |
| R8 | **cache_control 4 块上限冲突**：`ensureToolsCacheControl` + system 缓存 + 其他 block 理论上可能超 4 | 03 | 低 | `enforceCacheControlLimit` 已兜底 |
| R9 | **brand.name 硬编 i18n**：`anthropic.mom` 写死在翻译文件，改名需改代码 | 06 | 低 | 纳入 SystemSettings + i18n fallback |
| R10 | **embed_on.go CSP nonce 占位符依赖前端构建输出**：前端 webpack chunk hash 变化可能破坏替换 | 05 | 低 | 加构建后自检 |

## 7. 文档索引

| 文档 | 内容 |
|---|---|
| [01-tls-fingerprint.md](./01-tls-fingerprint.md) | TLS 指纹 + capture/verify 工具链 |
| [02-http-headers-ua.md](./02-http-headers-ua.md) | HTTP 头 / UA / anthropic-beta / Retry 采样 |
| [03-session-identity.md](./03-session-identity.md) | session_id 随机化 + sticky 复用 + cache_control |
| [04-sidecar-probe.md](./04-sidecar-probe.md) | usage 轮询 + count_tokens 注入 + startup probe |
| [05-backend-infra.md](./05-backend-infra.md) | config / wire / Settings API / failover / account |
| [06-frontend-misc.md](./06-frontend-misc.md) | 前端品牌化 + 账号 Modal + docs-example + go.mod 精简 |

## 8. 如何复现这份 diff

```bash
# 1. 在 sub2api 仓库里已有 hai/snapshot 分支（orphan commit）
cd C:\Users\16790\GolandProjects\sub2api

# 2. 宏观对比
git diff v0.1.114 hai/snapshot --shortstat
# 71 files changed, 6416 insertions(+), 1247 deletions(-)

# 3. 单文件精确 diff（示例）
git diff v0.1.114 hai/snapshot -- backend/internal/pkg/tlsfingerprint/dialer.go

# 4. 查看 hai 版完整文件
git show hai/snapshot:backend/internal/service/claude_sidecar_probe_service.go

# 5. 查看 v0.1.114 原版
git show v0.1.114:backend/internal/pkg/claude/constants.go
```

---

**文档生成信息**
- 生成时间：2026-04-18
- 分析工具：6 个并行 `Explore` agent，按域划分
- 对比基线：upstream `v0.1.114` tag（含 `Wei-Shaw/sub2api` 的官方 v0.1.114 tree）
- 快照分支：`hai/snapshot`（orphan commit `58193ae7`，无父节点）
