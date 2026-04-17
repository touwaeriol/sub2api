# TLS 指纹与指纹工具链 — 改动分析

## 1. 概述

该 fork 在 TLS 指纹域的核心改动是：**从手工编制的单一 Node.js 24.x 17-cipher 指纹升级为真实 Claude Code 2.1.109+ 的 52-cipher 完整捕获指纹，并增加了每账号 JA3 随机化能力**。改造目的是伪装成官方 Claude Code 客户端流量，规避上游 Anthropic 的指纹检测。新增的 `capture_fingerprint` 和 `verify_fingerprint` 工具链使得这个指纹可被持续验证。

关键创新：
1. ALPN 强制过滤掉 "h2"，解决 Go `http.Transport` 无法正确处理 HTTP/2 的根本问题
2. 新增 randomizer 对每账号生成独立 JA3，在保持整体 Node.js 轮廓的前提下防止被集合指纹识别
3. 建立 baseline 参考库和 capture/verify 工具，支持长期维护

## 2. 改动文件一览

| 文件 | 状态 | +行 | -行 | 改动要点 |
|---|---|---|---|---|
| `backend/internal/pkg/tlsfingerprint/dialer.go` | M | 184 | 40 | 52-cipher 默认指纹（vs 17），ALPN 过滤 h2，支持 MLKEM768，26 signature 算法 |
| `backend/internal/pkg/tlsfingerprint/randomizer.go` | A | 112 | 0 | 新增：JA3 随机化器，6-10 次 cipher 局部交换，30% GREASE，signature 分组洗牌 |
| `backend/internal/pkg/tlsfingerprint/dialer_capture_test.go` | M | 5 | 2 | 测试修正：ALPN 预期改为 `[http/1.1]`（去掉 h2） |
| `backend/internal/service/tls_fingerprint_profile_service.go` | M | 116 | ? | 新增 `RandomizeForAccount()`，注册 account repo，auto-generated profile 清理 |
| `backend/internal/handler/admin/tls_fingerprint_profile_handler.go` | M | 19 | 0 | 新增 handler：POST `/randomize-for-account/:accountID` |
| `frontend/src/api/admin/tlsFingerprintProfile.ts` | M | 15 | 0 | 新增 API 调用：`randomizeForAccount()` |
| `backend/tools/capture_fingerprint/main.go` | A | ~900 | 0 | 新工具：启 HTTPS+H2 capture 服务，嗅探 ClientHello，响应最小化 SSE |
| `backend/tools/capture_fingerprint/baselines/claude-code-2.1.111.json` | A | 163 | 0 | 真实 CC 2.1.111 指纹 baseline：52 cipher, 26 sig, 8 curves, JA3 hash |
| `backend/tools/verify_fingerprint/main.go` | A | 80 | 0 | 验证工具：用 dialer 的默认 profile 拨号至 capture server，对比 ClientHello |

## 3. 核心改动详解

### 3.1 dialer.go — ClientHello 对齐与 ALPN 过滤

**v0.1.114 做法**：
```go
defaultCipherSuites = []uint16{
    0x1301, 0x1302, 0x1303,                    // TLS 1.3
    0xc02b, 0xc02f, ..., 0x002f                // 14 others
}
defaultCurves = []utls.CurveID{utls.X25519, utls.CurveP256, utls.CurveP384}
```

17 个 cipher，3 个曲线，ALPN 默认 `["http/1.1"]` 但允许 profile 覆盖，无过滤。

**hai 做法**：
```go
defaultCipherSuites = []uint16{
    0x1302, 0x1303, 0x1301,                    // 注意 1302 在前！
    0xc02f, 0xc02b, 0xc030, 0xc02c,            // ECDHE AES-GCM
    0x009e,                                     // DHE_RSA
    // ... 47 more 包括 CCM, ARIA, legacy SHA1/DSA
    0x002f                                      // RSA AES-128-CBC-SHA（legacy）
}
defaultCurves = []utls.CurveID{
    utls.X25519MLKEM768,                        // 0x11ec（PQ hybrid）
    utls.X25519,
    utls.CurveP256, utls.CurveP384, utls.CurveP521,
}
defaultSignatureAlgorithms = []utls.SignatureScheme{
    0x0905, 0x0906, 0x0904,                    // experimental TLS 1.3
    0x0403, 0x0503, 0x0603,                    // ECDSA SHA256/384/512
    // ... 20 more 含 Brainpool, RSA-PSS, ed25519/ed448, legacy SHA1/DSA
}

// 防御性 ALPN 过滤（每次握手强制）
alpnProtocols = filterHTTP2FromALPN(alpnProtocols)

func filterHTTP2FromALPN(alpn []string) []string {
    out := make([]string, 0, len(alpn))
    for _, p := range alpn {
        if p == "h2" || p == "h2c" { continue }
        out = append(out, p)
    }
    if len(out) == 0 { return []string{"http/1.1"} }
    return out
}
```

**差异说明**：
- 52 cipher（3× 增长），完整反映 OpenSSL 默认链
- cipher 顺序极其关键：JA3 依赖顺序。`0x1302` 必须首位（v0.1.114 是 `0x1301` 首位）
- 新增 `X25519MLKEM768`（PQ hybrid），真实 CC 客户端的后量子混合 key share
- signature 从 9 扩展至 26，含 Brainpool/Experimental
- **关键创新**：`filterHTTP2FromALPN()` 在每次握手时强制移除 `h2`，即使 profile 指定了也无效。解决 Go `http.Transport` 无法处理 HTTP/2 SETTINGS 帧的架构问题（若 server 协商 h2，流量会 "malformed HTTP response"）

**对指纹伪装的作用**：
- JA3 hash 从 `44f88fca027f27bab4bb08d4af15f23e` → `d67b094811e5145139d7cea5f014309f`，精确对齐官方 CC 2.1.109+
- 脱离了 "Go client" 的显著特征（少量 cipher、无 PQ hybrid），融入真实 Node.js 客户端池
- ALPN-only-http/1.1 避免协议协商失败陷阱

### 3.2 randomizer.go — JA3 随机化（新增）

**设计动机**：
Anthropic 可能在检测层 "聚合" 所有 sub2api 出站流量指纹，发现它们都同一 JA3。新 randomizer 为每个账号独立生成一个微扰 Profile，52 个账号 → 52 个不同 JA3（都在 Node.js 轮廓内），打破聚合指纹检测。

**实现机制**：
```go
func GenerateRandomizedProfile() *Profile {
    // 1. Cipher 局部交换
    //    - PFS band [3:29]：6-10 次随机交换
    //    - RSA band [41:]：2-4 次交换（保留整体 legacy 分布）
    ciphers := copy(defaultCipherSuites)
    swapCountPFS := 6 + rand.IntN(5)  // 6..10
    for i := 0; i < swapCountPFS; i++ {
        a := 3 + rand.IntN(26)
        b := 3 + rand.IntN(26)
        ciphers[a], ciphers[b] = ciphers[b], ciphers[a]
    }

    // 2. Signature 分组洗牌：同强度组内
    //    - 保留 experimental TLS 1.3 (0..2) 和 ECDSA (3..5) 不变
    //    - 洗牌 RSA-PSS-PSS/RSAE/legacy-DSA 子组

    // 3. GREASE：30% 启用
    p.EnableGREASE = rand.IntN(10) < 3

    // 4. ALPN：固定 ["http/1.1"]
    p.ALPNProtocols = []string{"http/1.1"}

    // 5. Key share groups：固定 [MLKEM768, X25519]
    //    若 server HRR 至 MLKEM768，utls 无法重生，故总在初始 key_share 中
    p.KeyShareGroups = []uint16{0x11ec, 29}

    return p
}
```

**关键设计约束**：
- **局部交换而非全 shuffle**：保留 "FS-preferred" 整体形状，不变成陌生指纹
- **不扰动的轴**：curve、point_formats、supported_versions、psk_modes、extensions 留空（让 dialer 用 baseline）
- **MLKEM768 约束**：必须总在初始 key_share 中，否则 server HRR 会让 utls 崩溃

**对指纹伪装的作用**：
- 52 账号 → 52 独立 JA3（概率极低重复），防聚合模式识别
- 变化范围仍在 Node.js 客户端轮廓内，不会被识别为"随机虚假指纹"
- 30% GREASE 随机化还能模拟 Chrome 在 Node 之上的伪装行为

### 3.3 TLS Profile 管理链（service + handler + 前端）

**hai 新增流程**：
```go
func (s *TLSFingerprintProfileService) RandomizeForAccount(ctx, accountID) (*Profile, error) {
    // 1. 获取账号，校验 IsAnthropicOAuthOrSetupToken
    account := s.accountRepo.GetByID(ctx, accountID)

    // 2. 检查旧 auto-generated profile（待删）
    if existingID > 0 && IsAutoGeneratedProfileName(existing.Name) {
        oldAutoProfileID = existingID
    }

    // 3. 调用 randomizer 生成 Profile
    generated := tlsfingerprint.GenerateRandomizedProfile()

    // 4. 命名规范：__auto__:acc-{accountID}-{unixTimestamp}
    newProfile := &model.TLSFingerprintProfile{
        Name: fmt.Sprintf("__auto__:acc-%d-%d", accountID, time.Now().Unix()),
        // ...
    }

    // 5. 更新账号 extra
    extraUpdates := {
        "enable_tls_fingerprint": true,
        "tls_fingerprint_profile_id": newProfile.ID,
        "tls_fingerprint_randomized": true,
        "tls_fingerprint_randomized_at": RFC3339 now,
    }
    s.accountRepo.UpdateExtra(ctx, accountID, extraUpdates)

    // 6. 清理旧 auto profile
    if oldAutoProfileID > 0 {
        s.Delete(ctx, oldAutoProfileID)
    }

    return newProfile, nil
}
```

**Handler + 前端 API**：
- `POST /admin/tls-fingerprint-profiles/randomize-for-account/:accountID`
- `frontend/src/api/admin/tlsFingerprintProfile.ts` 暴露 `randomizeForAccount()`

**设计特点**：
- **幂等**：重复调用自动清旧 auto profile
- **手动 profile 保护**：仅删 `__auto__:` 前缀的，手工绑定的只覆盖不删
- **状态追踪**：账号 extra 记录 `tls_fingerprint_randomized_at` 时间戳

### 3.4 capture_fingerprint 工具

**用途**：启动本地 HTTPS+H2 服务器，让真实 Claude Code CLI 指向该服务器，嗅探并保存其 ClientHello 和 HTTP/2 帧序列。

**工作流**：
```bash
# 终端 1
go run ./tools/capture_fingerprint -addr 127.0.0.1:8443 -out fp.json

# 终端 2
NODE_TLS_REJECT_UNAUTHORIZED=0 \
ANTHROPIC_BASE_URL=https://127.0.0.1:8443 \
ANTHROPIC_API_KEY=sk-ant-capture-dummy \
claude -p --model claude-sonnet-4-5 'hi'
```

**关键机制**：

1. **peekConn 缓冲**：拦截原始 TCP，先读取并记录前 5+2 字节（TLS record header），缓存到 buffer，后续 crypto/tls 从 buffer 回放 → 在 TLS 库消费 ClientHello 前就有完整明文
2. **ClientHello 解析**：用 `utls.Fingerprinter` 提取 52 cipher、13 extension、8 curves、26 sig algos、ALPN、PSK modes；计算 **JA3**（MD5）和 **JA4**
3. **HTTP/2 服务**：读 H2 preface，启 Framer，捕获客户端 SETTINGS、窗口更新、每个请求的 pseudo-header 顺序、body 前 512 字节；对 `/count_tokens`, `/v1/messages`, `/api/oauth/usage`, `/v1/models` 返回 stub
4. **Capture 结构**：
```go
type Capture struct {
    CapturedAt        string
    ClientHelloRaw    string              // hex
    CipherSuites      []string
    Extensions        []string
    Curves            []string
    SignatureAlgos    []string
    SupportedVersions []string
    KeyShareGroups    []string
    ALPNProtos        []string
    PSKModes          []string
    JA3String         string
    JA3Hash           string
    JA4               string
    HTTP2             *H2Capture
    HTTP1             *H1Capture
}
```

### 3.5 verify_fingerprint 工具

**用途**：验证 sub2api dialer 是否与真实 CC 指纹一致。用默认 profile 拨号至 capture server，对比生成的 ClientHello。

```go
func main() {
    profile := &tlsfingerprint.Profile{
        Name: "default",
        ALPNProtocols: []string{"http/1.1"},
    }
    dialer := tlsfingerprint.NewDialer(profile, nil)
    transport := &http.Transport{
        DialTLSContext:    dialer.DialTLSContext,
        ForceAttemptHTTP2: false,
    }
    client := &http.Client{Transport: transport, Timeout: 15*time.Second}
    req, _ := http.NewRequestWithContext(ctx, "POST", captureURL+"/v1/messages", ...)
    resp, err := client.Do(req)  // 自签证书→error, capture server 已记录
    fmt.Printf("check capture server log for ClientHello details\n")
}
```

验证需人工用 `jq` 对比两份 JSON 的 `ja3_hash` / `cipher_suites`。

### 3.6 baseline 文件 claude-code-2.1.111.json

2026-04-16 从真实 CC 2.1.111 捕获的 ground truth。用于：
- **初始化 dialer 默认值**：52 cipher 顺序、5 curves、26 sig 都来自此
- **回归验证**：CC 新版本发布时重捕获对比 JA3
- **文档**：证明硬编码值来自真实客户端

**关键字段**：
```json
{
  "captured_at": "2026-04-16T16:41:55Z",
  "tls_version": "TLS 1.3",
  "negotiated_proto": "http/1.1",
  "client_hello_raw": "010006380303...",      // 1216 bytes hex
  "cipher_suites": ["0x1302","0x1303","0x1301", ...52 项],
  "extensions": [
    "renegotiation_info (0xff01)",             // 必须第一
    "server_name (0)",
    "ec_point_formats (11)",
    "supported_groups (10)",
    "session_ticket (35)",
    "application_layer_protocol_negotiation (16)",
    "generic (0x0016)",
    "extended_master_secret (23)",
    "signature_algorithms (13)",
    "supported_versions (43)",
    "psk_key_exchange_modes (45)",
    "key_share (51)"
  ],
  "curves": ["0x11ec","0x001d","0x0017","0x001e","0x0018","0x0019","0x0100","0x0101"],
  "key_share_groups": ["0x11ec", "0x001d"],    // 1216B MLKEM + 32B X25519
  "alpn_protos": ["http/1.1"],                  // ONLY
  "signature_algorithms": ["0x0905","0x0906","0x0904","0x0403", ...26 项],
  "supported_versions": ["0x0303", "0x0304"],   // TLS1.2, TLS1.3
  "ja3_hash": "d67b094811e5145139d7cea5f014309f",
  "ja3_string": "771,4866-4867-...",
  "http2": {
    "client_settings": { "HEADER_TABLE_SIZE":4096, "ENABLE_PUSH":1, "MAX_CONCURRENT_STREAMS":100, ... },
    "first_headers_pseudo_order": [":method",":authority",":path",":scheme"],
    "requests": [
      {"stream_id":1,"method":"POST","path":"/v1/messages", ...},
      {"stream_id":3,"method":"POST","path":"/api/oauth/usage", ...}
    ]
  }
}
```

## 4. 关键常量、阈值、配置

| 参数 | 值 | 说明 |
|---|---|---|
| cipher suites count | 52 | Node.js 24.14.1 OpenSSL 完整列表 |
| curves count | 5 | MLKEM768, X25519, P256, P384, P521 |
| signature algos count | 26 | 含 experimental TLS 1.3, Brainpool, RSA-PSS, legacy SHA1/DSA |
| supported versions | [TLS1.3, TLS1.2] | 固定 |
| ALPN | `["http/1.1"]` | **强制**，移除任何 h2/h2c |
| key_share_groups | [X25519MLKEM768, X25519] | 双份额：PQ 1216B + EC 32B |
| GREASE enabled | 30% on / 70% off | `rand.IntN(10) < 3` |
| cipher swap (PFS) | [3:29], 6-10 次 | randomizer 局部扰动 |
| cipher swap (RSA) | [41:end], 2-4 次 | 保留整体 legacy 分布 |
| auto-profile prefix | `__auto__:acc-` | 识别自动化分配 |
| randomizer seed | `math/rand/v2` 全局 | runtime 初始化 |
| baseline capture date | 2026-04-16 | CC 2.1.111 |

## 5. 对 5 大设计表的对照

| 表格项 | 实现位置 | 状态 |
|---|---|---|
| 对齐 CC 2.1.109+ ClientHello | `dialer.go` (52 cipher + 26 sig + 5 curves) | ✅ JA3 `44f88fc` → `d67b0948` |
| 删除 utls 不支持曲线 | `dialer.go:165-177` & `randomizer.go:69` | ✅ 仅保留 5 个曲线（4 可 HRR 重生，MLKEM768 总在初始 key_share 中） |
| ALPN 过滤 h2 | `dialer.go:357-369` `filterHTTP2FromALPN()` | ✅ 握手前强制移除 h2，randomizer 锁定 http/1.1 |
| 每账号 JA3 随机化（手动 + 新建自动）| `randomizer.go` + `service.go:RandomizeForAccount` | ✅ 手动：admin 建 profile 后绑定；自动：POST `/randomize-for-account/:accountID` |
| capture_fingerprint 抓流量 | `tools/capture_fingerprint/main.go` | ✅ HTTPS+H2 capture，ClientHello sniff，H2 request 序列 |
| verify_fingerprint 对比 baseline | `tools/verify_fingerprint/main.go` | ✅ 骨架工具，依赖人工 jq 对比 JSON（非自动化） |
| 存 2.1.111 ground truth | `baselines/claude-code-2.1.111.json` | ✅ 163 行完整 JSON，JA3 `d67b0948...` |

## 6. 潜在风险与观察

### 代码质量
- ✅ 注释详尽（MLKEM768 约束、为何局部交换）
- ⚠️ **硬编码 52 cipher**：`dialer.go:63-138` 超长常量定义，可考虑从 JSON 加载（但目前性能无碍）
- ✅ 测试适配：`dialer_capture_test.go` ALPN 预期已改为 `[http/1.1]`

### 并发安全
- ✅ randomizer 用 `math/rand/v2`，每次独立生成无竞态
- ✅ service 中 account repo/profile repo 更新原子（依赖底层实现）

### 可观测性
- ✅ capture 有详尽日志
- ✅ verify 简洁输出
- ⚠️ **randomizer 无日志**，无法观察每次随机化结果（仅可通过 HTTP 查询生成的 profile）

### 维护性
- ✅ baseline 更新靠 capture 工具自动生成，无需手工编辑
- ⚠️ **版本绑定**：baseline 文件名硬编 `2.1.111`，CC 升级需新增文件 + 更新 dialer 默认值，无自动化流程
- ✅ auto-profile 清理避免库污染

### 伪装持久性
- ⚠️ **JA3 聚合风险**：即便每账号随机，仍在 Node.js 轮廓内。若 Anthropic 改为检测"来自 Node.js 但不是真实 CC"，本措施可能失效
- ✅ HTTP/2 陷阱规避：ALPN h2 过滤彻底解决 Go `http.Transport` 兼容性
- ⚠️ **仅覆盖 TLS 层**：HTTP 请求顺序、UA、Cookie 等应用层指纹需其他域单独处理（见 02/03/04 文档）

---

**总结**：fork 在 TLS 域实现了"手工编制 → 真实捕获 → 每账号随机化"的完整升级，配套工具链支持持续验证。关键创新：ALPN 强制过滤（架构障碍）+ randomizer 保守设计（保 Node.js 轮廓）。风险集中在版本维护和应用层指纹互补防护。
