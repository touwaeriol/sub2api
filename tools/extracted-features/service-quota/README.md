# Service Quota Feature Patch

A standalone patch set extracting the **Service Quota** feature (multi-dimensional rate
limiting, quota management, and runtime monitoring) from `touwaeriol/sub2api` fork against
the clean `Wei-Shaw/sub2api` upstream tag **`v0.1.119`**.

## Baseline

| Item | Value |
| --- | --- |
| Upstream baseline | `Wei-Shaw/sub2api` tag `v0.1.119` (commit `a0b5e5bf`) |
| Source branch | `touwaeriol/sub2api` `release/custom-0.1.119` |
| Patch generated with | `git diff v0.1.119..HEAD -- <files>` |

> **All patches in this directory assume the working tree is at upstream `v0.1.119`**.
> Do **NOT** apply onto `upstream/main` (the migration numbers were chosen to fit the
> `v0.1.119` migration table; later upstream commits may add new files at colliding paths).

## Files

| File | Lines | What it is |
| --- | --- | --- |
| `backend.patch` | ~9 003 | **Drop-in patch.** Adds 47 service-quota Go files + 1 pkg dependency (`pkgerrors/validation.go`) + 8 SQL migrations. Each file is a pure addition - no existing upstream file is modified. Passes `git apply --check` cleanly on `v0.1.119`. |
| `frontend.patch` | ~7 464 | **Full drop-in patch.** Adds 38 service-quota / dependency files **and** patches 13 in-place files (`router/index.ts`, `AppSidebar.vue`, `stores/app.ts`, `types/index.ts`, `featureFlags.ts`, `format.ts`, `api/admin/settings.ts`, `views/admin/SettingsView.vue`, `views/admin/ops/components/OpsConcurrencyCard.vue`, `i18n/locales/{en,zh}.ts`, `vitest.config.ts`, `__tests__/setup.ts`). Passes `git apply --check` cleanly on `v0.1.119`. **The frontend is fully wired up after applying this patch alone** - no manual integration step needed (unlike the backend, which still needs `integration-backend.diff`). |
| `integration-backend.diff` | ~5 111 | **Reference only - DO NOT `git apply`.** Diff of the 28 backend files that the fork modifies in-place (wire bindings, route registration, settings DTO, `setting_service.GetBool` helper, gateway hook integration, billing cache ticket). The diff includes a lot of unrelated fork changes (rectifier, advisor, version bumps); maintainers should hand-pick the service-quota hunks. |
| `integration-frontend.diff` | ~2 680 | **Superseded by `frontend.patch`.** Original "manual hand-pick" diff against the same 13 frontend integration points. Kept for historical reference - the corresponding hunks have already been baked into `frontend.patch`. |

## What's in `backend.patch`

### Pure additions

- **47 service-quota Go files** under:
  - `backend/internal/handler/admin/service_quota_*.go` - admin CRUD + monitor endpoints
  - `backend/internal/handler/admin/ops_service_quota_metrics_handler.go` - metrics endpoint
  - `backend/internal/handler/user_service_quota_handler.go` - user-facing quota view
  - `backend/internal/repository/service_quota_*.go` - repo, cache, limiter (fixed-window / rolling-window / concurrency)
  - `backend/internal/service/service_quota_*.go` - pre-check / acquire / release pipeline, validation, monitor
  - `backend/internal/metrics/service_quota.go` - Prometheus counters
- **1 pkg dependency**: `backend/internal/pkg/errors/validation.go` (`FieldErrorCollector`,
  `ValidationFailed`) - shared by service-quota field-level validation. (`validation_test.go`
  also included.)
- **8 SQL migrations** (renumbered to avoid colliding with `v0.1.119`'s migrations 125-133):

  | Original (in fork) | Renumbered for patch |
  | --- | --- |
  | `128_add_service_quota_rules.sql` | `200_add_service_quota_rules.sql` |
  | `130_refactor_service_quota_target_mode.sql` | `201_refactor_service_quota_target_mode.sql` |
  | `131_add_service_quota_batch_id.sql` | `202_add_service_quota_batch_id.sql` |
  | `132_add_service_quota_channel_id.sql` | `203_add_service_quota_channel_id.sql` |
  | `133_refactor_service_quota_hierarchical.sql` | `204_refactor_service_quota_hierarchical.sql` |
  | `135_restore_service_quota_paths.sql` | `205_restore_service_quota_paths.sql` |
  | `136_service_quota_token_components.sql` | `206_service_quota_token_components.sql` |
  | `137_service_quota_rpm_count_on_arrival.sql` | `207_service_quota_rpm_count_on_arrival.sql` |

  > Renumbering rationale: `v0.1.119` already ships migrations `125..133`
  > (`125_add_channel_monitors`, `126_add_channel_monitor_aggregation`, ...,
  > `133_affiliate_rebate_freeze`). The fork numbers `128/130/131/132/133` collide with
  > upstream content. Renumbering to `200..207` puts the new SQL safely above the upstream
  > range without changing any of the SQL content. **There are no cross-references between
  > the migration files**, so renumbering is purely cosmetic.

### What's NOT in `backend.patch`

The patch is intentionally limited to **net-new files**. It does **not** modify any
upstream file. Consequently it will **not compile on its own** - you need the integration
hunks below.

## What's in `frontend.patch`

51 files (38 net-new + 13 in-place edits), 6 470 insertions / 270 deletions.

### Net-new files (38)

- `frontend/src/api/serviceQuota.ts` and `frontend/src/api/admin/serviceQuota.ts`
- `frontend/src/components/serviceQuota/*` - shared `PathChevron`, `QuotaMonitorTable`,
  composables (`useQuotaMonitorFormat`, `useQuotaMonitorRows`), `pathRender`, `entityNames`,
  plus specs
- `frontend/src/views/admin/serviceQuota/*` - `ConfigView`, `MonitorView`, components
  (`CollapsibleSection`, `EnabledToggleCell`, `FilterBar`, `RuleEditDialog`), composables
  (`useRuleDeleteDialog`, `useServiceQuotaDisplay`, `useServiceQuotaFilters`), plus specs
- `frontend/src/views/user/QuotaMonitorView.{vue,spec.ts}` and `useUserQuotaFilters` (+ spec)
- `frontend/src/utils/validateServiceQuota.ts`, `loadIndicator.ts`, `limiterColors.ts`,
  plus specs and `formatDailyUsd.spec.ts`
- `frontend/src/components/admin/{LimiterEditor,PathEditor}.vue` - shared editors only
  consumed by the quota config view
- `frontend/src/components/common/{EntitySearchSelect,NumericInput,PlatformPicker,RequiredLabel,UserMultiSelect}.vue`
  plus `NumericInput.spec.ts` - generic UI primitives whose only callers (in this fork) are
  the service-quota editors and dialog. Listed under "common" because they have no
  service-quota-specific logic and could be reused elsewhere if the upstream maintainers
  wish.

### In-place edits (13)

Wiring + UX touch-ups so the feature is reachable from the UI:

- `router/index.ts` - register `/admin/service-quotas/{monitor,config}` and `/quota-monitor`
- `components/layout/AppSidebar.vue` - add admin nav group ("Service Quota" with two
  children) and user nav item ("My Quotas")
- `views/admin/SettingsView.vue` - add the **Service Quota Enabled** master toggle in the
  feature-flags panel; call new `appStore.patchPublicSettings({ service_quota_enabled })`
  to hot-update the sidebar after save
- `stores/app.ts` - add `patchPublicSettings()` action (lets `SettingsView` patch a single
  flag without round-tripping `/api/v1/public/settings`)
- `types/index.ts`, `api/admin/settings.ts`, `utils/featureFlags.ts` - surface the new
  `service_quota_enabled` boolean on `PublicSettings` / `SystemSettings` / feature flag
  registry
- `utils/format.ts` - add `formatThousands()` (en-US thousands, used in limiter chips) and
  `formatDailyUsd()` (en-US fixed-locale USD with up-to-6-decimal precision, used in daily
  spend chips). Both are additive — `formatCurrency` is unchanged.
- `views/admin/ops/components/OpsConcurrencyCard.vue` - extract the inline
  `getLoadBarClass / getLoadBarStyle / getLoadTextClass` helpers into the shared
  `utils/loadIndicator.ts` so the new `QuotaMonitorTable` reuses the exact same colour
  ramp. Behaviour-preserving refactor.
- `i18n/locales/{en,zh}.ts` - add the `serviceQuota.*`, `userQuotaMonitor.*`,
  `nav.{serviceQuota,serviceQuotaMonitor,serviceQuotaConfig,myQuota}`,
  `admin.settings.features.serviceQuota.*`, plus reusable `common.{userSearch,entitySearch,tagInput}`
  string namespaces, plus a few error code keys consumed by the field-error parser.
  > **Caveat:** the `en.ts` / `zh.ts` files are copied wholesale from
  > `release/custom-0.1.119`. They include a small number of **unrelated** i18n key
  > additions (e.g. account / common error codes) that the fork added in the same period.
  > These are **additive only** (no key renames or deletions on existing upstream keys), so
  > they do not break upstream behaviour, but reviewers may want to drop the keys they don't
  > recognise during review.
- `vitest.config.ts`, `__tests__/setup.ts` - register the global vitest setup file (loaded
  by the new specs) and add a `window.matchMedia` mock (jsdom does not implement it, and
  `QuotaMonitorView` uses a `useDesktopViewport` composable that calls `matchMedia`).

After applying `frontend.patch`, the only remaining frontend gap is whatever depends on
the **backend** wiring (the new `/api/v1/admin/service-quota/...` endpoints). The backend
side still requires the manual hunks listed under "Required integration changes" below.

## Required integration changes (manual)

After applying `backend.patch` and `frontend.patch`, the build will fail until you wire up
the feature. Use `integration-backend.diff` and `integration-frontend.diff` as reference
(they include the full fork diff for these files, including unrelated rectifier/advisor
changes - pick out the service-quota hunks).

### Backend integration points (28 files)

- **Wire bindings** - register the new providers:
  - `backend/internal/repository/wire.go` - add `ProvideServiceQuotaRepo`,
    `ProvideServiceQuotaLimiter`, `ProvideServiceQuotaCache`
  - `backend/internal/service/wire.go` - add `ProvideServiceQuotaService`,
    `ProvideServiceQuotaMonitorService`
  - `backend/internal/handler/wire.go` - add admin `ServiceQuotaHandler`,
    `ServiceQuotaMonitorHandler`, `OpsServiceQuotaMetricsHandler`,
    `UserServiceQuotaHandler`
  - `backend/cmd/server/wire_gen.go` - regenerate via `go generate ./...` (or hand-merge the
    new `provide` lines)

- **Route registration**:
  - `backend/internal/server/routes/admin.go` - register
    `/api/v1/admin/service-quota/...` and `/api/v1/admin/ops/service-quota-metrics`
  - `backend/internal/server/routes/user.go` - register `/api/v1/quota` (user view)

- **Settings**:
  - `backend/internal/service/setting_service.go` - add constants
    `SettingKeyServiceQuotaEnabled`, `service_quota_enabled` field on
    `Settings`/`PublicSettings` structs, default `"false"`, plus the new helper method
    `func (s *SettingService) GetBool(ctx, key) (bool, error)` (used by service-quota to
    read the feature switch). The `GetBool` method is a **new shared utility** that the
    quota service depends on.
  - `backend/internal/service/settings_view.go` - surface `ServiceQuotaEnabled` in
    `PublicSettingsView`
  - `backend/internal/handler/dto/settings.go` - add `service_quota_enabled` JSON tag
  - `backend/internal/handler/setting_handler.go` and
    `backend/internal/handler/admin/setting_handler.go` - accept/render the new key

- **Gateway hook integration** (the actual quota enforcement at request time):
  - `backend/internal/service/billing_cache_service.go` and
    `billing_cache_service_ticket.go` - extend `BillingTicket` with
    `Acquire / Consume / Close` to wrap the quota lease lifecycle
  - `backend/internal/service/gateway_service.go` - call
    `ServiceQuotaService.PreCheck` / `Acquire` / `Record` in pre-flight and post-flight
  - `backend/internal/handler/openai_gateway_handler.go`,
    `gateway_handler*.go`, `gemini_v1beta_handler.go`,
    `openai_chat_completions.go`, `openai_images.go` - protocol adapters that call the
    pipeline. Most of the actual logic lives in `gateway_service.go`; the handlers only
    forward.

- **Repo helpers**:
  - `backend/internal/repository/user_repo.go` - quota uses a couple of new helpers (e.g.
    bound-users lookup for monitor view)

- **Admin handlers** - small additions to support cleanup of residual counter keys when
  channels / groups / users / accounts are deleted:
  - `account_handler.go`, `channel_handler.go`, `group_handler.go`, `user_handler.go`,
    `common.go`

### Frontend integration points

**Already baked into `frontend.patch`** - no manual step needed. See "What's in
`frontend.patch` - In-place edits (13)" above for the list. The legacy
`integration-frontend.diff` is kept as a reference copy of those same hunks.

## How to apply (clean `v0.1.119` checkout)

```bash
git fetch upstream --tags
git checkout v0.1.119
git apply tools/extracted-features/service-quota/frontend.patch
cd frontend && pnpm install && pnpm build && pnpm test
```

Backend (separate, still requires the manual integration step):

```bash
git apply tools/extracted-features/service-quota/backend.patch

# Now hand-merge the integration hunks (see "Required integration changes" above),
# using integration-backend.diff as the reference.

# Build check
cd backend && go build ./...
go test -tags unit ./internal/service/...
```

## Verification

To re-verify the patches apply cleanly on a fresh checkout:

```bash
git stash
git checkout v0.1.119
git apply --check tools/extracted-features/service-quota/backend.patch
git apply --check tools/extracted-features/service-quota/frontend.patch
git checkout -
git stash pop
```

Status as of generation (baseline `v0.1.119`):

- `git apply --check tools/extracted-features/service-quota/backend.patch` -> **clean**
- `git apply --check tools/extracted-features/service-quota/frontend.patch` -> **clean**
- Standalone backend build (without `integration-backend.diff` hunks) -> **fails as expected**
  (undefined references to `SettingService.GetBool` and the new wire providers)
- Standalone frontend build (after applying just `frontend.patch`) -> **succeeds**
  (frontend integration is complete; the only thing missing is the backend API surface)

## Source commits

The patch corresponds to ~195 commits between `v0.1.119` and the fork
`release/custom-0.1.119` HEAD that touch service-quota. Notable seed commits:

- `ebb91557` - `feat: add service quota management` (initial implementation, predates
  v0.1.119; included in the fork's pre-existing tree)
- `b356d73c` - `feat(pkgerrors): add ValidationFailed with FieldErrorCollector`
- `b0ef5606` - `refactor(service_quota): extract composables from ConfigView and QuotaMonitorTable`
- `aca1d6bf` - `feat(service_quota): clean residual counter keys on channel/group/account delete`
- `eac96113` - `perf(service_quota): pipeline deferred limiter reads in PreCheckAcquire`

For the full list:

```bash
git log v0.1.119..HEAD --oneline --grep='quota\|ServiceQuota\|service_quota' -i
```
