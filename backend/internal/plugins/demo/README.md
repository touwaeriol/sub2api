# Demo Plugin

End-to-end reference plugin for the sub2api plugin SDK. Used during Phase 0
to validate that schema, settings, HTTP routes, event subscribers and
cross-plugin exports all work together.

## Files

| File | Purpose |
|------|---------|
| `plugin.go` | `Plugin` struct + `init()` registration, `Meta`, lifecycle |
| `handlers.go` | HTTP handlers (`/hello`, `/notes`) |
| `subscribers.go` | `account.created` event subscriber |
| `exports.go` | `demoapi.Exports` implementation for peer plugins |
| `i18n.go` | `embed.FS` loader for locale bundles |
| `api/exports.go` | Public interface + DTOs for cross-plugin callers |
| `ent/schema/note.go` | ent schema for `plugin_demo_notes` |
| `ent/gen/` | Generated ent client (produced by `go generate`) |
| `locales/en.json`, `locales/zh.json` | i18n bundles |

## Regenerating the ent client

```
cd backend
go generate ./internal/plugins/demo/ent/...
```

## Endpoints

- `GET /api/v1/plugin/demo/hello` — public; returns the configured greeting.
- `GET /api/v1/plugin/demo/notes` — admin; returns the most recent 50 notes.

## Events

Subscribes to `account.created` (Notify). On each delivery the plugin
writes an audit note into `plugin_demo_notes` so integration tests can
observe the end-to-end flow (schema → subscriber → exports).

## Exports

Peer plugins retrieve the export surface with:

```go
api, err := plugin.PluginAs[demoapi.Exports](core, demo.PluginID)
```

See `api/exports.go` for the stable cross-plugin contract.
