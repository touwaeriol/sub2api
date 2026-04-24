package plugin

import "context"

// Plugin is the root contract that every sub2api plugin implements.
//
// The lifecycle the host drives:
//
//  1. At process start, each plugin's init() calls [Register] to hand the
//     loader a *Plugin value.
//  2. The loader reads [Plugin.Meta], validates APIVersion / permissions /
//     dependencies, applies schema and migrations, and calls [Plugin.Init]
//     with a CoreAPI scoped to the plugin's id.
//  3. When the plugin's state in the `plugins` table is `enabled`, the loader
//     calls [Plugin.Start]; on disable or shutdown it calls [Plugin.Shutdown].
//
// Start MUST NOT block — long-running work should be spawned on a goroutine
// that honours ctx cancellation. Shutdown MUST release all resources acquired
// in Start; the host may call Init/Start again on a later enable transition.
type Plugin interface {
	// Meta returns the plugin's declarative descriptor. The host calls this
	// repeatedly and expects a pure, side-effect-free return.
	Meta() Meta

	// Init is called once per process lifetime, after dependencies are ready
	// and before Start. The plugin should store the CoreAPI handle and
	// prepare internal services. Init MUST NOT call CoreAPI methods that
	// require the plugin to be "enabled".
	Init(core CoreAPI) error

	// Start activates the plugin. It is called each time the plugin
	// transitions to enabled. Return quickly; use goroutines for background
	// work.
	Start(ctx context.Context) error

	// Shutdown deactivates the plugin. It is called on disable or on host
	// shutdown. Implementations must be idempotent and bounded — respect
	// ctx deadlines.
	Shutdown(ctx context.Context) error
}

// HealthChecker is an optional capability: plugins implementing it expose a
// health signal that the host surfaces through /api/v1/admin/plugins/health.
type HealthChecker interface {
	// Health returns the current readiness snapshot. Implementations should
	// not block — run any probe in the background and return cached state.
	Health(ctx context.Context) HealthStatus
}

// ConfigChangeListener is an optional capability: plugins implementing it are
// notified when their namespaced settings change at runtime. The host invokes
// this synchronously after persisting the new value; errors cause the change
// to be rolled back.
type ConfigChangeListener interface {
	OnConfigChange(ctx context.Context, key string, oldValue, newValue any) error
}
