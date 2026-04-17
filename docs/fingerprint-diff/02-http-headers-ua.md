# HTTP 头 / UA / anthropic-beta — 改动分析

## 1. 概述

该 fork 通过伪装官方 Claude CLI 客户端来规避 Anthropic 对流量的识别。本次对比分析了 `v0.1.114` (基线) 与 `hai/snapshot` 分支在 HTTP 头、User-Agent、anthropic-beta、Timeout、Retry 采样等方面的改动。**关键变更**：UA 版本号从 2.1.22 升至 2.1.112；anthropic-beta 新增 3 个 token 并移除过时 token；Timeout 从 600 秒降至 300 秒；Retry-Count 从硬编码改为概率采样。

## 2. 改动文件一览

| 文件 | 行数变化 | 主要改动 |
|------|--------|--------|
| `backend/internal/pkg/claude/constants.go` | +67/-15 | 新增 Beta 常量、`SampleStainlessRetryCount()`、更新 UA/Timeout |
| `backend/internal/service/gateway_service.go` | ~+72 | 集成 retry 采样、beta 常量应用、cache-control 自动注入、sessionHash 跟踪 |

## 3. 核心改动详解

### 3.1 UA 字符串变更

**v0.1.114 版本：**
```go
"User-Agent": "claude-cli/2.1.22 (external, cli)",
"X-Stainless-OS": "Linux",
"X-Stainless-Runtime-Version": "v24.13.0",
```

**hai/snapshot 版本：**
```go
"User-Agent": "claude-cli/2.1.112 (external, sdk-cli)",
"X-Stainless-OS": "MacOS",
"X-Stainless-Runtime-Version": "v24.14.1",
"X-Stainless-Package-Version": "0.81.0",
```

**分析：** UA 版本号更新至最新 npm 发行版 2.1.112，同时调整 UA 后缀从 `cli` → `sdk-cli`（对齐真实 Claude Code 2.1.11x 抓包）。OS 从 Linux → MacOS，Runtime 版本号同步最新环境（Node.js v24.14.1）。注释指出此次更新通过 `backend/tools/capture_fingerprint` 实时流量抓包验证。

### 3.2 anthropic-beta header 注入

**新增 Beta 常量（三个）：**
```go
BetaContextManagement20250627  = "context-management-2025-06-27"
BetaPromptCachingScope20260105 = "prompt-caching-scope-2026-01-05"
BetaAdvisorTool20260301        = "advisor-tool-2026-03-01"
```

**旧版问题：** v0.1.114 包含已过时的 `BetaFineGrainedToolStreaming`，注释明确说明该特性已被 Anthropic 合并入基线 API，保留它会留下 "UA 版本号与 beta token 不符" 的扫描痕迹。

**新策略 — `clientExtraBetas`：**
```go
const clientExtraBetas = "," + BetaContextManagement20250627 + "," + BetaPromptCachingScope20260105 + "," + BetaAdvisorTool20260301
```

所有 beta header 常量（`DefaultBetaHeader`、`MessageBetaHeaderNoTools`、`MessageBetaHeaderWithTools`、`CountTokensBetaHeader`）都通过 `+ clientExtraBetas` 追加这三个 token。

**API-Key 账号处理：** `APIKeyBetaHeader` 从原来的包含 `BetaFineGrainedToolStreaming` 改为包含 `clientExtraBetas`，精确匹配真实 Claude Code 2.1.111 在 `x-api-key` 认证下的行为。

**网关层应用（gateway_service.go）：** 非 Haiku 模型的 OAuth 请求强制注入这三个 token：
```go
requiredBetas = []string{
    claude.BetaClaudeCode,
    claude.BetaOAuth,
    claude.BetaInterleavedThinking,
    claude.BetaContextManagement20250627,      // 新增
    claude.BetaPromptCachingScope20260105,     // 新增
    claude.BetaAdvisorTool20260301,            // 新增
}
```

### 3.3 Timeout 调整

| 版本 | X-Stainless-Timeout |
|---|---|
| v0.1.114 | `600`（10 分钟）|
| hai/snapshot | `300`（5 分钟）|

注释指出 `@anthropic-ai/sdk` 最近将默认超时降至 ~5 分钟，与真实 Claude Code 2.1.111 的抓包一致（2.1.109 为 600 秒）。

### 3.4 Retry 权重采样

**v0.1.114：**
```go
"X-Stainless-Retry-Count": "0",  // 硬编码
```

**hai/snapshot（新增函数）：**
```go
func SampleStainlessRetryCount() string {
    r := rand.Float64()
    switch {
    case r < 0.005:    // 0.5% → "2"
        return "2"
    case r < 0.03:     // 2.5% → "1"
        return "1"
    default:           // 97.0% → "0"
        return "0"
    }
}
```

**应用点：**
1. `applyClaudeOAuthHeaderDefaults()` 每次请求调用采样
2. `applyClaudeCodeMimicHeaders()` 强制覆盖下游客户端的 retry count

**精确性：** 表格要求的 97/2.5/0.5% 完全匹配。

### 3.5 其他 Header 改动

1. **DefaultHeaders 中移除 Retry-Count 硬编码**，改由 `SampleStainlessRetryCount()` 动态生成，避免重复请求的 retry count 完全相同的识别特征。
2. **`applyClaudeCodeMimicHeaders()` 强制行为**：先调用 `applyClaudeOAuthHeaderDefaults()` 填充缺失 header，再遍历 `DefaultHeaders` 强制覆盖所有键值，确保伪装模式下 header 绝对一致。
3. **`ensureToolsCacheControl()`**：在 tools 数组末尾自动注入 `cache_control: {"type":"ephemeral"}`，模拟真实 Claude Code 缓存工具定义的行为。
4. **SessionHash 跟踪**：`Forward()` 和 `ForwardCountTokens()` 中新增上下文记录，用于会话层 metadata 重写时复用同一 UUID。

## 4. 常量与硬编码值清单

| 常量 | v0.1.114 | hai/snapshot |
|---|---|---|
| User-Agent | claude-cli/2.1.22 | claude-cli/2.1.112 |
| X-Stainless-Package-Version | 0.70.0 | 0.81.0 |
| X-Stainless-OS | Linux | MacOS |
| X-Stainless-Runtime-Version | v24.13.0 | v24.14.1 |
| X-Stainless-Timeout | 600 | 300 |
| X-Stainless-Retry-Count | "0"（常数）| 采样函数 |
| UA 后缀 | (external, cli) | (external, sdk-cli) |

## 5. 对 5 大设计表的对照

| 表格项 | 实现位置 | v0.1.114 | hai/snapshot |
|---|---|---|---|
| UA 对齐 claude-cli/2.1.112 | constants.go | ❌ (2.1.22) | ✅ (2.1.112) |
| beta: context-management | constants.go | ❌ | ✅ `BetaContextManagement20250627` |
| beta: prompt-caching-scope | constants.go | ❌ | ✅ `BetaPromptCachingScope20260105` |
| beta: advisor-tool | constants.go | ❌ | ✅ `BetaAdvisorTool20260301` |
| ~~beta: fine-grained-tool-streaming~~ | constants.go | ✅（过时）| ❌ 已移除 |
| Timeout 600→300 | constants.go | ❌ (600) | ✅ (300) |
| Retry-Count 97/2.5/0.5% 随机 | constants.go | ❌（硬编码 "0"）| ✅ `SampleStainlessRetryCount` |

## 6. 潜在风险与观察

1. **Retry 采样随机性**：`math/rand/v2` 的随机源虽然按概率分布正确，但同一进程内短时间窗口内多次采样仍可能呈现某种可预测模式。若下游做滑动窗口统计检测，建议后续结合时间戳 hash 进一步去规律。
2. **cache_control 注入时机**：`ensureToolsCacheControl()` 在系统 message 改写后应用，可能被后续 `enforceCacheControlLimit()` 的 4 块上限约束。需验证与上游 4 块限制的交互。
3. **beta token 顺序敏感性**：`clientExtraBetas` 追加顺序固定，若上游按顺序校验或按位置规则匹配，顺序任何变动都会触发检测。建议记录真实抓包中的完整 beta 顺序。
4. **APIKeyBetaHeader 精确性**：新版注释声称与 2.1.111 抓包精确匹配，但 `BetaFineGrainedToolStreaming` 常量本身未删除（只是不再被拼接），存在被误引用的风险。
5. **Mac/Linux OS 标识切换**：从 Linux → MacOS 可能影响依赖 OS 标识的下游风控规则，上线后应监控是否引发新的告警。

---

**文档生成时间**：2026-04-18
**对比基线**：v0.1.114 tag vs hai/snapshot branch
**验证工具**：`backend/tools/capture_fingerprint`（实时抓包）
