# Request Rectifier System (整流器)

This patch extracts the **request-rectifier** system from the
`touwaeriol/sub2api` fork (release `v0.1.119`) onto the upstream
tag `Wei-Shaw/sub2api` `v0.1.119` (commit `a0b5e5bf`).

> The patch is generated with `git diff v0.1.119..HEAD -- <files>`,
> so applying it on a clean checkout of upstream tag `v0.1.119`
> avoids any drift introduced by post-tag commits on `upstream/main`.

## What is "Request Rectifier"?

When an upstream Anthropic-compatible provider returns a deterministic
4xx error caused by a *single* known-bad field in the request body or
header, the gateway transparently rewrites that field and replays the
request. The user never sees the upstream error.

There are 3 rectifiers:

| # | Rectifier | Status on upstream v0.1.119 | What this patch adds |
|---|-----------|-------------------------|----------------------|
| 1 | **Signature** — strip `thinking` blocks / `clear_thinking_*` ctx-mgmt when API Key signature breaks | already present (`FilterThinkingBlocksForRetry` / `FilterSignatureSensitiveBlocksForRetry` / `shouldRectifySignatureError`) | nothing — fork shares the same signature path |
| 2 | **Thinking-budget** — auto-inject `thinking.type=enabled` + `budget=32000` + `max_tokens>=32001` when upstream complains `budget_tokens >= 1024` | already present (`isThinkingBudgetConstraintError` / `RectifyThinkingBudget`) | refactors the retry to **not share retry budget** with signature rectifier (advisor/budget = single deterministic retry; signature = retry pool) |
| 3 | **Advisor-tool** — strip the `advisor-tool-2026-03-01` token from `anthropic-beta` and remove `tools[type=advisor_*]` when upstream rejects it | **not present** | full implementation: detection (`IsAdvisorToolUnsupportedError`) + body rewrite (`RectifyAdvisorTool`) + header strip (`stripBetaTokenIgnoreCase`) + admin UI toggle |

## File list

```
backend/internal/service/gateway_request.go              + advisor consts, IsAdvisorToolUnsupportedError, RectifyAdvisorTool
backend/internal/service/gateway_service.go              + Forward branch for advisor + budget retry refactor + helpers
backend/internal/service/header_util.go                  + delHeaderRaw (canonical/wire-casing/raw triple delete)
backend/internal/service/setting_service.go              + AdvisorToolPatterns default-injection on legacy JSON
backend/internal/service/settings_view.go                + AdvisorToolEnabled / AdvisorToolPatterns fields
backend/internal/handler/dto/settings.go                 + AdvisorTool* DTO fields
backend/internal/service/gateway_request_advisor_test.go (new) advisor detection + RectifyAdvisorTool tests
backend/internal/service/header_util_test.go             (new) delHeaderRaw + stripBetaTokenIgnoreCase tests
backend/internal/service/setting_service_advisor_test.go (new) legacy JSON upgrade + default injection tests
frontend/src/api/admin/settings.ts                       + advisor_tool_* TS fields
frontend/src/views/admin/SettingsView.vue                + Advisor Tool toggle + custom-pattern editor + 3× passthroughNotice
frontend/src/i18n/locales/zh.ts                          + 6 keys under admin.settings.rectifier.*
frontend/src/i18n/locales/en.ts                          + 6 keys under admin.settings.rectifier.*
```

## i18n additions

Namespace: `admin.settings.rectifier.*` (zh + en, 6 keys × 2 = 12 entries).

| Key | Purpose |
|-----|---------|
| `passthroughNotice` | Banner shown on stream-timeout, rectifier, beta-policy cards warning that "API Key passthrough" bypasses these features |
| `advisorTool` | Toggle label |
| `advisorToolHint` | Toggle help text |
| `advisorToolPatterns` | Custom-pattern section label |
| `advisorToolPatternsHint` | Custom-pattern help text |
| `advisorToolPatternPlaceholder` | Input placeholder |

## How to apply

```bash
# from a clean checkout of upstream tag v0.1.119
git fetch upstream --tags
git checkout v0.1.119
git apply --check tools/extracted-features/rectifier/rectifier.patch
git apply tools/extracted-features/rectifier/rectifier.patch
cd backend && go build ./... && go test -tags unit -run 'Rectif|Advisor|Budget|Signature|DelHeaderRaw' ./internal/service/
```

Verified clean apply against upstream tag `v0.1.119` (commit
`a0b5e5bf`) - `git apply --check` exits 0 and `go build ./...`
in the patched worktree succeeds.

If you want to re-verify without disturbing the working tree,
use a worktree:

```bash
git worktree add /tmp/sub2api-v0119 v0.1.119
cd /tmp/sub2api-v0119
git apply --check /path/to/rectifier.patch
```

## Known risks / conflict surface

> All line numbers below refer to upstream tag `v0.1.119`.

1. **`backend/internal/service/gateway_service.go`** is the most fragile
   file — the patch modifies the inner retry loop of `Forward()` (one
   12k-line god-function). If upstream refactors that retry block, the
   patch will need manual merging. The 3 hunks are at upstream lines
   `4432`, `4469`, `6186`, `6349`. Look for landmarks
   `IsBudgetRectifierEnabled`, `containsBetaToken`,
   `isSignatureErrorPattern`.
2. **`backend/internal/service/header_util.go`** — upstream already has
   `getHeaderRaw` / `setHeaderRaw` / `addHeaderRaw`. We only **add**
   `delHeaderRaw` immediately after `addHeaderRaw`. Low risk.
3. **`frontend/src/views/admin/SettingsView.vue`** — touches 7 hunks
   spread across template + setup script. If upstream renumbers cards
   or restructures the rectifier setup script, manual port required.
   Look for landmarks `rectifierForm.advisor_tool_enabled`,
   `passthroughNotice`, `loadRectifierSettings`,
   `saveRectifierSettings`.
4. **i18n files** — fork has its own large set of i18n changes;
   this patch only touches the `admin.settings.rectifier.*` block, so
   conflicts with other rectifier i18n changes upstream are possible
   but no other content is involved.

The patch is **self-contained** — no dependency on the
`service-quota` / `field-error-codes` / `cache-hit` patches in
`tools/extracted-features/`. It can be applied first or last in any
order with respect to those.

## Testing

```bash
# Unit tests for the rectifier:
cd backend
go test -tags unit -run 'Rectif|Advisor|Budget|Signature|DelHeaderRaw' ./internal/service/
```

Existing fork-side test count: **22** new test cases across the 3 new
test files (header_util / advisor / setting_service legacy upgrade).

## Manual verification

After applying, in the admin UI:

1. Settings → Rectifier card now has a 4th toggle "Advisor Tool 整流"
2. Toggle on → custom-pattern editor appears with placeholder
3. Save → backend returns the saved settings including
   `advisor_tool_enabled` + `advisor_tool_patterns`
4. Trigger an upstream `400` with body matching
   `Unexpected value(s) `advisor-tool-2026-03-01` for the
   `anthropic-beta` header.` → gateway should log
   `detected advisor-tool unsupported, retrying without
   advisor-tool-2026-03-01 beta+tool` and the request should succeed.
