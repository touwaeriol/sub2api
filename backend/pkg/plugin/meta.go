package plugin

// Permission names a capability on the CoreAPI that a plugin must declare in
// Meta.Permissions before the host will grant access. The permission guard
// (implemented alongside the loader) rejects CoreAPI calls whose required
// permission is not in the caller's declared set.
type Permission string

// Built-in permission constants. Add new entries sparingly — granularity
// should match audit/observability needs, not every individual method.
const (
	// PermAccountRead allows read-only access via AccountAPI.
	PermAccountRead Permission = "account.read"
	// PermAccountWrite allows mutating account credentials/extra via AccountAPI.
	PermAccountWrite Permission = "account.write"
	// PermUserRead allows read-only access via UserAPI.
	PermUserRead Permission = "user.read"
	// PermOrderRead allows read-only access via OrderAPI / SubscriptionAPI.
	PermOrderRead Permission = "order.read"
	// PermOrderWrite allows mutating orders via OrderAPI.
	PermOrderWrite Permission = "order.write"
	// PermBillingWrite allows writing usage records via BillingAPI.
	PermBillingWrite Permission = "billing.write"
	// PermSchedulerRead exposes scheduler snapshots via SchedulerAPI.
	PermSchedulerRead Permission = "scheduler.read"
	// PermHTTPOutbound allows outbound HTTP via HTTPUpstream.
	PermHTTPOutbound Permission = "http.outbound"
	// PermCrypto grants access to the Crypto helper.
	PermCrypto Permission = "crypto"
	// PermSettingsWrite allows writing system settings via NamespacedSettings.
	PermSettingsWrite Permission = "settings.write"
)

// AuthRequirement describes how the host should authenticate HTTP requests
// declared by the plugin via RouteSpec.
type AuthRequirement int

// AuthRequirement values.
const (
	// AuthNone — endpoint is public (no auth).
	AuthNone AuthRequirement = iota
	// AuthUser — requires a valid user session (JWT or API key).
	AuthUser
	// AuthAdmin — requires an admin API key or admin-role session.
	AuthAdmin
)

// Dep is a declarative dependency on another plugin. The loader resolves Deps
// topologically and rejects cycles with [ErrCircularDependency]. A plugin
// becomes Enabled only once all of its Deps are Enabled.
type Dep struct {
	// ID is the dependency plugin's Meta.ID.
	ID string
	// VersionRange is a semver range (e.g. "^1.2.0", ">=1.0 <2.0"). An empty
	// string accepts any installed version.
	VersionRange string
	// Optional marks the dependency as "use if present". Missing optional
	// deps do not block the plugin from loading.
	Optional bool
}

// Meta is the declarative descriptor of a plugin. Returned by Plugin.Meta and
// consumed by the loader, permission guard, router, migration runner, and
// admin UI.
//
// Fields are grouped by purpose; leave zero-values for features you do not
// use.
type Meta struct {
	// Identity ---------------------------------------------------------

	// ID is the unique, stable plugin id (lowercase, kebab-case). It is
	// used as the prefix for declared tables and for lookups.
	ID string
	// Name is the human-readable display name for admin UIs.
	Name string
	// Description is a one-paragraph summary for admin UIs.
	Description string
	// Version is the plugin's own semver (independent of APIVersion).
	Version string
	// APIVersion is the SDK version this plugin was built against. The
	// host validates it with [CheckAPIVersion] at load time.
	APIVersion string

	// Contract ---------------------------------------------------------

	// Permissions declares every Permission the plugin intends to use.
	// Requests through CoreAPI are gated against this set.
	Permissions []Permission
	// Dependencies are other plugins this plugin requires.
	Dependencies []Dep

	// Schema -----------------------------------------------------------

	// Tables enumerates the fully-qualified table names the plugin owns.
	// All names MUST start with the "plugin_<id>_" prefix; use [TableName]
	// to generate them. The host uses this list on uninstall --purge.
	Tables []string
	// Schema, if non-nil, produces the ent-style DDL for the plugin's
	// tables on install/upgrade.
	Schema SchemaProvider
	// Migrations is an ordered list of explicit migrations (DDL that
	// SchemaProvider cannot express idempotently: drops, type changes,
	// data transforms). Executed in ascending ID order.
	Migrations []Migration

	// Extension points -------------------------------------------------
	//
	// At most ONE plugin may be assigned to each extension slot for a
	// given scope; multi-instance support is handled by the plugin itself.

	// Gateway, if set, registers the plugin as the forwarder for a
	// particular upstream platform.
	Gateway GatewayPlugin
	// AccountType, if set, teaches the host how to create/validate/refresh
	// a custom account type.
	AccountType AccountTypePlugin
	// RateLimit, if set, replaces the host's default rate-limit response
	// parser for this plugin's upstream.
	RateLimit RateLimitParser
	// Payment, if set, registers a payment provider (see PaymentProvider).
	Payment PaymentProvider

	// Resources --------------------------------------------------------

	// Routes declares HTTP endpoints exposed by the plugin under
	// /api/v1/plugins/<id>/.
	Routes []RouteSpec
	// Menus declares admin navigation entries pointing at frontend views.
	Menus []MenuSpec
	// Settings declares schema-driven configuration slots exposed in the
	// admin settings page.
	Settings []SettingSpec
	// Crons declares background jobs the host's scheduler should run for
	// this plugin.
	Crons []CronSpec
	// Frontend, if non-nil, locates the compiled or runtime-ESM UI.
	Frontend *FrontendSpec

	// Events -----------------------------------------------------------

	// Publishes declares the topics this plugin emits, along with their
	// [EventKind]. Other plugins and the host use this manifest to verify
	// subscriptions at registration time.
	Publishes []EventDecl
	// Subscribes declares the topics this plugin listens on.
	Subscribes []EventSubscription

	// Cross-plugin API -------------------------------------------------

	// Exports is an arbitrary value exposing methods for other plugins to
	// call, retrieved via [PluginAs]. Keep the exported type stable —
	// breaking changes are equivalent to a major version bump.
	Exports any
}
