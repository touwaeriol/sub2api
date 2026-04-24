// Package demo is the end-to-end reference plugin used to validate the
// sub2api plugin SDK. It intentionally touches every moving part: schema
// provider, settings, HTTP routes (admin + public), event subscribers, a
// cross-plugin export surface, and an embedded i18n bundle.
//
// Importing this package from cmd/server/main.go triggers init() which
// registers the plugin with the SDK global registry. The host loader then
// walks the rest of the lifecycle.
package demo

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect"
	gensql "entgo.io/ent/dialect/sql/schema"

	demoapi "github.com/Wei-Shaw/sub2api/internal/plugins/demo/api"
	gen "github.com/Wei-Shaw/sub2api/internal/plugins/demo/ent/gen"
	"github.com/Wei-Shaw/sub2api/internal/plugins/demo/ent/gen/migrate"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// PluginID is the stable identifier for this plugin. Table names and
// settings keys derive from it.
const PluginID = "demo"

// apiVersion is the SDK version the demo plugin was built against.
const apiVersion = plugin.SDKVersion

// settingKeyGreeting is the single namespaced setting the demo reads at
// runtime. Kept as a const so the handler and the tests share one source
// of truth.
const settingKeyGreeting = "greeting"

// defaultGreeting is emitted by the public /hello endpoint when the
// greeting setting has not been written yet.
const defaultGreeting = "hello"

// noteTable is the fully-qualified table name the demo owns.
var noteTable = plugin.TableName(PluginID, "notes")

// Plugin is the demo implementation of plugin.Plugin. It holds the SDK-
// provided CoreAPI handle and the plugin-scoped ent client (created in
// Init once a driver becomes available).
type Plugin struct {
	core    plugin.CoreAPI
	client  *gen.Client
	exports *demoExports
}

// init registers the plugin with the SDK global registry. The host's
// loader reads that registry at boot.
func init() { plugin.Register(&Plugin{}) }

// Meta returns the declarative descriptor consumed by the loader, the
// permission guard, the router, the migration runner and the admin UI.
func (p *Plugin) Meta() plugin.Meta {
	return plugin.Meta{
		ID:          PluginID,
		Name:        "Demo Plugin",
		Description: "Hello-world plugin used to validate the sub2api plugin SDK end to end.",
		Version:     "0.1.0",
		APIVersion:  apiVersion,

		Permissions: []plugin.Permission{
			plugin.PermAccountRead,
		},

		Tables: []string{noteTable},
		Schema: &demoSchemaProvider{},

		Settings: []plugin.SettingSpec{
			{
				Key:         settingKeyGreeting,
				Label:       "Greeting",
				Description: "Text returned by the public /hello endpoint.",
				Default:     defaultGreeting,
				Schema:      plugin.UIFieldSchema{Type: "string"},
			},
		},

		Routes: []plugin.RouteSpec{
			{
				Method:      "GET",
				Path:        "/api/v1/plugin/demo/hello",
				Auth:        plugin.AuthNone,
				Handler:     p.hello,
				Description: "Public hello-world endpoint echoing the configured greeting.",
			},
			{
				Method:      "GET",
				Path:        "/api/v1/plugin/demo/notes",
				Auth:        plugin.AuthAdmin,
				Handler:     p.listNotes,
				Description: "Admin-only listing of audit notes written by the demo plugin.",
			},
		},

		Menus: []plugin.MenuSpec{
			{
				ID:           "demo-notes",
				Label:        "Demo Notes",
				Path:         "/admin/plugins/demo",
				RequiredRole: "admin",
			},
		},

		Subscribes: []plugin.EventSubscription{
			{
				Topic:         plugin.TopicAccountCreated,
				Kind:          plugin.EventKindNotify,
				SubscriberTag: PluginID + "/notes-init",
				Handler:       p.onAccountCreated,
			},
		},

		Exports: p.exportsHandle(),
	}
}

// exportsHandle returns the embedded demoExports pointer so peer plugins
// can type-assert against demoapi.Exports via plugin.PluginAs. The pointer
// is stable across Meta() calls because it is lazily allocated once.
func (p *Plugin) exportsHandle() demoapi.Exports {
	if p.exports == nil {
		p.exports = &demoExports{owner: p}
	}
	return p.exports
}

// Init is called once after the host has applied schema and migrations.
// We stash the CoreAPI handle and materialise the plugin's ent client.
func (p *Plugin) Init(core plugin.CoreAPI) error {
	p.core = core
	drv := core.EntDriver()
	if drv == nil {
		// Phase 0 boot without a wired driver — the plugin still loads so
		// pure-metadata features (settings, i18n, route mounting) work,
		// but data access is disabled and queries return an error.
		core.Logger().Warn("demo: ent driver unavailable; data features disabled")
		return nil
	}
	p.client = gen.NewClient(gen.Driver(drv))
	if p.exports != nil {
		p.exports.client = p.client
	}
	return nil
}

// Start activates the plugin. The demo has no background loops so it just
// returns nil.
func (p *Plugin) Start(_ context.Context) error { return nil }

// Shutdown deactivates the plugin. Closing the ent client is a no-op for
// shared-driver clients; we still null it out so a later Enable rebuilds.
func (p *Plugin) Shutdown(_ context.Context) error {
	p.client = nil
	if p.exports != nil {
		p.exports.client = nil
	}
	return nil
}

// demoSchemaProvider implements plugin.SchemaProvider. It defers the ent
// Schema construction to CreateOrUpgrade so it can use the driver passed
// by the migrator instead of one captured at Meta() time (which would be
// impossible — the driver is a host-owned runtime resource).
type demoSchemaProvider struct{}

// CreateOrUpgrade builds a migrate.Schema from the supplied driver and
// delegates to ent's idempotent Create step.
func (demoSchemaProvider) CreateOrUpgrade(ctx context.Context, drv dialect.Driver) error {
	if drv == nil {
		return fmt.Errorf("demo: schema upgrade requires a driver")
	}
	schema := migrate.NewSchema(drv)
	return schema.Create(ctx, gensql.WithGlobalUniqueID(true))
}
