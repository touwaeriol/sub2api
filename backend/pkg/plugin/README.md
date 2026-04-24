# sub2api plugin SDK

`github.com/Wei-Shaw/sub2api/pkg/plugin` is the import boundary between sub2api
plugins and the host binary. Plugins depend only on this package (and the
Go standard library / explicitly re-exported libraries like ent and gin).
The host is free to evolve its internals without breaking plugins as long as
this package's exported surface stays within its APIVersion contract.

## Minimal plugin

```go
package helloplugin

import (
    "context"

    "github.com/Wei-Shaw/sub2api/pkg/plugin"
)

func init() { plugin.Register(&helloPlugin{}) }

type helloPlugin struct{ core plugin.CoreAPI }

func (p *helloPlugin) Meta() plugin.Meta {
    return plugin.Meta{
        ID:          "hello",
        Name:        "Hello Plugin",
        Version:     "0.1.0",
        APIVersion:  plugin.SDKVersion,
        Permissions: []plugin.Permission{plugin.PermAccountRead},
    }
}

func (p *helloPlugin) Init(c plugin.CoreAPI) error { p.core = c; return nil }
func (p *helloPlugin) Start(ctx context.Context) error    { return nil }
func (p *helloPlugin) Shutdown(ctx context.Context) error { return nil }
```

Linking it is a blank import from the host's main package:

```go
import _ "github.com/myorg/hello-plugin"
```

## File layout (this package)

| file | purpose |
|------|---------|
| `doc.go` | package-level godoc, lifecycle overview |
| `version.go` | `SDKVersion`, `CheckAPIVersion` |
| `errors.go` | sentinel errors (`ErrPermissionDenied`, …) |
| `plugin.go` | `Plugin`, `HealthChecker`, `ConfigChangeListener` |
| `meta.go` | `Meta`, `Permission` constants, `Dep`, `AuthRequirement` |
| `resources.go` | `RouteSpec`, `MenuSpec`, `SettingSpec`, `UIFieldSchema`, `CronSpec`, `FrontendSpec` |
| `events.go` | `EventKind`, `EventSchema`, `EventSubscription`, `EventBus`, topic constants |
| `schema.go` | `SchemaProvider`, `TableName`, `AssertTableName`, `NewEntSchemaProvider` |
| `migration.go` | `Migration`, `MigrationStep`, `MigrationsFromFS` |
| `types.go` | plugin-facing DTOs (`Account`, `User`, `Order`, `ForwardRequest`, …) |
| `extensions.go` | `GatewayPlugin`, `AccountTypePlugin`, `RateLimitParser`, `PaymentProvider` |
| `core.go` | `CoreAPI` + all sub-API interfaces |
| `register.go` | `Register`, `Registered`, `Lookup` |
| `exports.go` | `PluginAs[T]` generic helper |

## Data layer contract

- Plugins **declare** tables via `Meta.Tables` + a `SchemaProvider`; the host
  **executes** DDL. Every table name must be prefixed with `plugin_<id>_` —
  use `TableName(pluginID, local)` and the host verifies with
  `AssertTableName`.
- Idempotent "create table / add column / add index" goes through
  `SchemaProvider.CreateOrUpgrade`. Build one from an ent-generated schema
  with `NewEntSchemaProvider(client.Schema)`.
- Destructive or data-transforming changes live in `Meta.Migrations`, with
  one entry per change, keyed by a lexical `ID`
  (`YYYYMMDDHHMMSS_description`). `MigrationsFromFS(embedFS, dir)` reads a
  folder of `*.sql` files into the list.

## Event semantics

Every topic has one of three `EventKind`s, enforced at subscribe time:

- `EventKindSyncHook` — in-tx, handler error aborts publish (veto).
- `EventKindAsyncHook` — enqueued, retried, dead-lettered.
- `EventKindNotify` — fire-and-forget, best-effort.

Built-in topics live as `TopicAccountBeforeDelete`, `TopicOrderPaid`, etc.

## What's NOT in this package (yet)

This SDK is interfaces and helpers only. The following are the host's
responsibility and will be built in subsequent phases:

- **Loader**: reads `Registered()`, resolves `Meta.Dependencies` topologically,
  validates `CheckAPIVersion`, creates the `CoreAPI` implementation per
  plugin, drives the lifecycle state machine in the `plugins` table.
- **PermissionGuard**: a wrapper around every CoreAPI sub-API that rejects
  calls whose required `Permission` is not in `Meta.Permissions`, returning
  `ErrPermissionDenied`.
- **EventBus implementation**: a registry of topic schemas plus three
  dispatchers (sync, async queue, notify fan-out). Validates
  `EventSubscription.Kind` against `EventSchema.Kind` at registration time.
- **MigrationRunner**: walks `Meta.Migrations`, compares checksums against
  `plugin_migrations`, applies new migrations inside transactions.
- **CoreAPI adapters**: each sub-API (AccountAPI, UserAPI, …) implemented by
  wrapping the existing `internal/service` code. `BillingAPI` routes to
  `UsageLogService`; `HTTPUpstream` wraps `internal/pkg/httputil`; `Crypto`
  wraps the existing payment encryption key; and so on.
- **Admin UI**: plugin lifecycle controls, schema-driven settings forms,
  health/usage dashboards.
- **Frontend loader**: layer 2 (compiled Vue packages) and eventually layer 3
  (runtime ESM via `FrontendSpec.EntryURL`).
