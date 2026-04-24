package dto

// PluginStateValues is the set of lifecycle states returned on the wire.
// Mirrors repository.PluginState* so the HTTP layer stays decoupled from
// the ent enum.
const (
	PluginStateInstalled   = "installed"
	PluginStateDisabled    = "disabled"
	PluginStateEnabled     = "enabled"
	PluginStateUninstalled = "uninstalled"
)

// PluginDepDTO represents a declared plugin dependency on the wire.
type PluginDepDTO struct {
	ID           string `json:"id"`
	VersionRange string `json:"version_range,omitempty"`
	Optional     bool   `json:"optional,omitempty"`
}

// PluginMenuDTO mirrors plugin.MenuSpec minus the Go handlers.
type PluginMenuDTO struct {
	ID           string `json:"id"`
	Label        string `json:"label,omitempty"`
	Icon         string `json:"icon,omitempty"`
	Path         string `json:"path,omitempty"`
	Parent       string `json:"parent,omitempty"`
	Order        int    `json:"order,omitempty"`
	RequiredRole string `json:"required_role,omitempty"`
}

// PluginFrontendDTO mirrors plugin.FrontendSpec on the wire.
type PluginFrontendDTO struct {
	BundleName string `json:"bundle_name,omitempty"`
	EntryURL   string `json:"entry_url,omitempty"`
}

// PluginUIFieldSchemaDTO is the JSON projection of plugin.UIFieldSchema.
// Only fields useful to the admin form renderer are included; regex
// `Pattern` is forwarded verbatim for client-side validation.
type PluginUIFieldSchemaDTO struct {
	Type       string                            `json:"type,omitempty"`
	Enum       []string                          `json:"enum,omitempty"`
	Min        *float64                          `json:"min,omitempty"`
	Max        *float64                          `json:"max,omitempty"`
	MinLen     *int                              `json:"min_len,omitempty"`
	MaxLen     *int                              `json:"max_len,omitempty"`
	Required   bool                              `json:"required,omitempty"`
	Pattern    string                            `json:"pattern,omitempty"`
	Properties map[string]PluginUIFieldSchemaDTO `json:"properties,omitempty"`
	Items      *PluginUIFieldSchemaDTO           `json:"items,omitempty"`
}

// PluginSettingDTO mirrors plugin.SettingSpec. Secret values are never
// serialised — only the schema and metadata are exposed so the admin UI
// can render an empty password field.
type PluginSettingDTO struct {
	Key         string                 `json:"key"`
	Label       string                 `json:"label,omitempty"`
	Description string                 `json:"description,omitempty"`
	Secret      bool                   `json:"secret,omitempty"`
	Default     any                    `json:"default,omitempty"`
	Schema      PluginUIFieldSchemaDTO `json:"schema"`
}

// PluginPublicDTO is the slim projection returned by `GET /api/v1/plugins`.
// Its purpose is to bootstrap the frontend (menus, forms, UI bundle) for
// currently enabled plugins. Secret setting values are masked before
// conversion.
type PluginPublicDTO struct {
	ID          string             `json:"id"`
	Name        string             `json:"name,omitempty"`
	Version     string             `json:"version,omitempty"`
	Description string             `json:"description,omitempty"`
	Menus       []PluginMenuDTO    `json:"menus,omitempty"`
	Settings    []PluginSettingDTO `json:"settings,omitempty"`
	Frontend    *PluginFrontendDTO `json:"frontend,omitempty"`
}

// PluginResponse is the admin-facing list/detail projection. State data
// comes from the loader (ListStates/FindState); declarative data from
// plugin.Meta.
type PluginResponse struct {
	ID             string         `json:"id"`
	Name           string         `json:"name,omitempty"`
	Version        string         `json:"version,omitempty"`
	APIVersion     string         `json:"api_version,omitempty"`
	State          string         `json:"state"`
	InstalledAt    int64          `json:"installed_at,omitempty"`    // Unix ms
	LastEnabledAt  *int64         `json:"last_enabled_at,omitempty"` // Unix ms
	DeclaredTables []string       `json:"declared_tables,omitempty"`
	Dependencies   []PluginDepDTO `json:"dependencies,omitempty"`
	Permissions    []string       `json:"permissions,omitempty"`

	// Populated only on detail endpoint.
	Description string             `json:"description,omitempty"`
	Author      string             `json:"author,omitempty"`
	Menus       []PluginMenuDTO    `json:"menus,omitempty"`
	Settings    []PluginSettingDTO `json:"settings,omitempty"`
	Frontend    *PluginFrontendDTO `json:"frontend,omitempty"`
}

// DeadLetterDTO is the admin-list projection of eventbus.DeadLetterEntry.
type DeadLetterDTO struct {
	ID            int64  `json:"id"`
	Topic         string `json:"topic"`
	SubscriberTag string `json:"subscriber_tag,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Payload       string `json:"payload,omitempty"` // JSON string
	FirstFailedAt int64  `json:"first_failed_at,omitempty"`
	LastAttemptAt int64  `json:"last_attempt_at,omitempty"`
	AttemptCount  int    `json:"attempt_count,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}
