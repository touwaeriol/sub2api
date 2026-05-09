# Extracted Features

从 fork `touwaeriol/sub2api` 提取的可移植功能 patch，基线为上游 **`Wei-Shaw/sub2api` tag `v0.1.119`** (commit `a0b5e5bf`)。

## 产物

| 目录 | 说明 |
|---|---|
| `rectifier/` | 请求整流器（signature 增强 + thinking-budget + advisor-tool） |
| `service-quota/` | 多维度服务级限流（backend + frontend） |

## 推荐应用顺序

```bash
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api
git fetch --tags
git checkout v0.1.119

# 1. Rectifier（自包含，独立）
git apply /path/to/rectifier/rectifier.patch

# 2. Service Quota backend
git apply /path/to/service-quota/backend.patch

# 3. Service Quota frontend —— 必须用 --3way，因为 i18n 文件与 rectifier 同区域修改
git apply --3way /path/to/service-quota/frontend.patch
```

第三步不用 `--3way` 会在 `frontend/src/i18n/locales/{en,zh}.ts` 冲突失败。`--3way` 基于 blob SHA 做 merge，rectifier 和 quota 的 i18n key 互不重叠，能自动合并。

## 验证状态（v0.1.119 干净 worktree）

| Patch | Apply 结果 | 文件数 |
|---|---|---|
| rectifier | ✅ 干净 apply | 13 |
| service-quota/backend | ✅ 干净 apply | 101 |
| service-quota/frontend | ✅ `--3way` apply，其余干净 | 34 |
| 三者叠加（含 --3way） | ✅ 成功 | 112 个文件、无冲突标记 |

## 已知依赖

- `service-quota/backend.patch` 内已包含 `pkg/errors/validation.go` 的 `FieldError` 框架
- `service-quota/frontend.patch` 依赖 `service-quota/backend.patch`（API DTO 同步）
- `rectifier.patch` 完全自包含，可独立 apply

## 测试

```bash
# Backend
cd backend && go build ./... && go test -tags unit ./internal/service/...

# Frontend
cd frontend && pnpm install && pnpm build && pnpm test
```

参见各子目录的 `README.md` 获取详细文件清单、i18n key、迁移说明、已知风险。
