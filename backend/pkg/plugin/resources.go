package plugin

import "github.com/gin-gonic/gin"

// RouteSpec declares an HTTP endpoint exposed by the plugin.
//
// The host mounts it under /api/v1/plugins/<plugin-id>/<Path>. The HTTP method,
// auth requirement and handler come from this spec; the host is responsible
// for applying the matching auth middleware before the handler runs.
type RouteSpec struct {
	// Method is the HTTP verb ("GET", "POST", ...).
	Method string
	// Path is relative to the plugin's mount prefix; leading slash is
	// required, e.g. "/webhook".
	Path string
	// Auth declares the authentication level the host must enforce.
	Auth AuthRequirement
	// Handler is the gin handler for the route. It runs after auth and
	// permission checks.
	Handler gin.HandlerFunc
	// Description is shown in auto-generated route documentation / admin UI.
	Description string
}

// MenuSpec declares a menu entry for the admin UI.
type MenuSpec struct {
	// ID is stable, e.g. "payment-alipay". Used for ordering/hiding.
	ID string
	// Label is the localized display text (plain string; i18n can be
	// resolved later via I18n.T).
	Label string
	// Icon is a reference to a frontend icon component (name or path).
	Icon string
	// Path is the Vue-Router path this entry navigates to.
	Path string
	// Parent is the parent menu ID for nesting; empty for top-level.
	Parent string
	// Order controls menu position (lower = earlier).
	Order int
	// RequiredRole limits visibility: "admin" or "user".
	RequiredRole string
}

// SettingSpec declares a configuration slot the admin UI should render via
// the schema-driven form (UI layer 1).
type SettingSpec struct {
	// Key is the namespaced setting key (actual storage key is
	// "plugin:<plugin-id>:<Key>"). Use [NamespacedSettings] to read/write.
	Key string
	// Label is the human-readable label.
	Label string
	// Description explains the setting to operators.
	Description string
	// Default is the default value serialised to JSON.
	Default any
	// Secret marks the value as sensitive (masked in UI, encrypted at rest).
	Secret bool
	// Schema drives form rendering and client-side validation.
	Schema UIFieldSchema
}

// UIFieldSchema is a minimal JSON-Schema-like descriptor consumed by the
// admin UI's generic form renderer. Keep it deliberately small; for anything
// more complex ship a custom Vue component via [FrontendSpec].
type UIFieldSchema struct {
	// Type is one of "string", "number", "boolean", "enum", "object",
	// "array", "textarea", "password".
	Type string
	// Enum lists allowed values when Type == "enum".
	Enum []string
	// Min/Max apply to Type == "number" (nil means unbounded).
	Min *float64
	Max *float64
	// MinLen/MaxLen apply to Type == "string" / "textarea".
	MinLen *int
	MaxLen *int
	// Required is an informational hint; server-side validation is still
	// the plugin's responsibility.
	Required bool
	// Pattern is an optional regex for string validation.
	Pattern string
	// Properties describes nested fields when Type == "object".
	Properties map[string]UIFieldSchema
	// Items describes the element schema when Type == "array".
	Items *UIFieldSchema
}

// CronSpec declares a scheduled job. The host's scheduler (wrapping
// robfig/cron) invokes Run at each trigger.
type CronSpec struct {
	// ID is stable and unique within the plugin; used for observability.
	ID string
	// Schedule is a standard cron expression (5 or 6 fields).
	Schedule string
	// Description documents the job for admin UIs.
	Description string
	// Run is the job body.
	Run func() error
}

// FrontendSpec locates the plugin's UI asset. Layer-1 (schema-driven) does
// not need this; layer-2/3 plugins set it.
type FrontendSpec struct {
	// BundleName is the compiled Vue package name, e.g.
	// "@myorg/plugin-alipay-ui". Used when the host embeds plugin UIs at
	// build time (layer 2).
	BundleName string
	// EntryURL is the runtime-loaded ESM entry (layer 3). When set, the
	// host serves it from /plugins/<id>/ui/. Phase 1 ignores this field.
	EntryURL string
}
