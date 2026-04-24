package admin

import (
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/plugin/eventbus"
	"github.com/Wei-Shaw/sub2api/internal/plugin/loader"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// unixMilli converts a time to Unix milliseconds; zero in, zero out.
func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// registeredByID indexes plugin.Registered() by meta id for O(1) lookups.
func registeredByID() map[string]plugin.Plugin {
	regs := plugin.Registered()
	out := make(map[string]plugin.Plugin, len(regs))
	for _, p := range regs {
		if p == nil {
			continue
		}
		out[p.Meta().ID] = p
	}
	return out
}

// buildAdminListDTO projects a plugin into the list endpoint response. The
// plugin pointer may be nil when the row refers to a plugin no longer
// compiled in (e.g. uninstalled but row kept for audit).
func buildAdminListDTO(state loader.PluginState, p plugin.Plugin) dto.PluginResponse {
	out := dto.PluginResponse{
		ID:             state.ID,
		Version:        state.Version,
		APIVersion:     state.APIVersion,
		State:          defaultString(state.State, dto.PluginStateUninstalled),
		InstalledAt:    unixMilli(state.InstalledAt),
		DeclaredTables: append([]string(nil), state.DeclaredTables...),
	}
	if !state.LastEnabledAt.IsZero() {
		ms := unixMilli(state.LastEnabledAt)
		out.LastEnabledAt = &ms
	}
	if p != nil {
		meta := p.Meta()
		out.Name = meta.Name
		if out.Version == "" {
			out.Version = meta.Version
		}
		if out.APIVersion == "" {
			out.APIVersion = meta.APIVersion
		}
		out.Dependencies = mapDependencies(meta.Dependencies)
		out.Permissions = mapPermissions(meta.Permissions)
		if len(out.DeclaredTables) == 0 {
			out.DeclaredTables = append([]string(nil), meta.Tables...)
		}
	}
	return out
}

// buildAdminDetailDTO extends the list projection with the full Meta, minus
// code-only fields (Schema, Migrations, Exports, Gateway, ...).
func buildAdminDetailDTO(state loader.PluginState, p plugin.Plugin) dto.PluginResponse {
	base := buildAdminListDTO(state, p)
	if p == nil {
		return base
	}
	meta := p.Meta()
	base.Description = meta.Description
	base.Menus = mapMenus(meta.Menus)
	base.Settings = mapSettings(meta.Settings, true)
	base.Frontend = mapFrontend(meta.Frontend)
	return base
}

// buildPublicDTO is the minimal projection returned by GET /api/v1/plugins.
// Secret settings are stripped of their default value.
func buildPublicDTO(p plugin.Plugin) dto.PluginPublicDTO {
	meta := p.Meta()
	return dto.PluginPublicDTO{
		ID:          meta.ID,
		Name:        meta.Name,
		Version:     meta.Version,
		Description: meta.Description,
		Menus:       mapMenus(meta.Menus),
		Settings:    mapSettings(meta.Settings, false),
		Frontend:    mapFrontend(meta.Frontend),
	}
}

// mapDependencies projects plugin.Dep slices onto the wire DTO.
func mapDependencies(deps []plugin.Dep) []dto.PluginDepDTO {
	if len(deps) == 0 {
		return nil
	}
	out := make([]dto.PluginDepDTO, 0, len(deps))
	for _, d := range deps {
		out = append(out, dto.PluginDepDTO{
			ID:           d.ID,
			VersionRange: d.VersionRange,
			Optional:     d.Optional,
		})
	}
	return out
}

// mapPermissions renders permission constants as their string form.
func mapPermissions(perms []plugin.Permission) []string {
	if len(perms) == 0 {
		return nil
	}
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		out = append(out, string(p))
	}
	return out
}

// mapMenus copies MenuSpec records; handlers/gin functions are dropped.
func mapMenus(menus []plugin.MenuSpec) []dto.PluginMenuDTO {
	if len(menus) == 0 {
		return nil
	}
	out := make([]dto.PluginMenuDTO, 0, len(menus))
	for _, m := range menus {
		out = append(out, dto.PluginMenuDTO{
			ID:           m.ID,
			Label:        m.Label,
			Icon:         m.Icon,
			Path:         m.Path,
			Parent:       m.Parent,
			Order:        m.Order,
			RequiredRole: m.RequiredRole,
		})
	}
	return out
}

// mapSettings projects plugin.SettingSpec onto the wire DTO. When
// includeSecretDefaults is false, secret fields have their Default value
// masked ("" or nil depending on the schema type). Even admin callers only
// see defaults, never the actual stored value — that is resolved separately
// via the settings API.
func mapSettings(settings []plugin.SettingSpec, includeSecretDefaults bool) []dto.PluginSettingDTO {
	if len(settings) == 0 {
		return nil
	}
	out := make([]dto.PluginSettingDTO, 0, len(settings))
	for _, s := range settings {
		item := dto.PluginSettingDTO{
			Key:         s.Key,
			Label:       s.Label,
			Description: s.Description,
			Secret:      s.Secret,
			Schema:      mapUISchema(s.Schema),
		}
		if !s.Secret || includeSecretDefaults {
			item.Default = s.Default
		}
		out = append(out, item)
	}
	return out
}

// mapUISchema walks UIFieldSchema recursively.
func mapUISchema(in plugin.UIFieldSchema) dto.PluginUIFieldSchemaDTO {
	out := dto.PluginUIFieldSchemaDTO{
		Type:     in.Type,
		Enum:     append([]string(nil), in.Enum...),
		Min:      in.Min,
		Max:      in.Max,
		MinLen:   in.MinLen,
		MaxLen:   in.MaxLen,
		Required: in.Required,
		Pattern:  in.Pattern,
	}
	if len(in.Properties) > 0 {
		out.Properties = make(map[string]dto.PluginUIFieldSchemaDTO, len(in.Properties))
		for k, v := range in.Properties {
			out.Properties[k] = mapUISchema(v)
		}
	}
	if in.Items != nil {
		nested := mapUISchema(*in.Items)
		out.Items = &nested
	}
	return out
}

// mapFrontend projects FrontendSpec; nil in, nil out.
func mapFrontend(in *plugin.FrontendSpec) *dto.PluginFrontendDTO {
	if in == nil {
		return nil
	}
	return &dto.PluginFrontendDTO{
		BundleName: in.BundleName,
		EntryURL:   in.EntryURL,
	}
}

// mapDeadLetterToDTO projects eventbus.DeadLetterEntry into the admin DTO.
// The payload is stringified (JSON) so the admin UI can render it directly.
func mapDeadLetterToDTO(e eventbus.DeadLetterEntry) dto.DeadLetterDTO {
	return dto.DeadLetterDTO{
		ID:            e.ID,
		Topic:         e.Topic,
		SubscriberTag: e.SubscriberTag,
		CorrelationID: e.CorrelationID,
		Payload:       payloadToJSONString(e.Payload),
		FirstFailedAt: unixMilli(e.FirstFailedAt),
		LastAttemptAt: unixMilli(e.LastAttemptAt),
		AttemptCount:  e.AttemptCount,
		LastError:     e.LastError,
	}
}

// payloadToJSONString returns the payload as a pretty JSON string when
// possible, falling back to the raw bytes.
func payloadToJSONString(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	// Re-encode without indentation to keep wire traffic small; the admin
	// UI can pretty-print client-side.
	out, err := json.Marshal(decoded)
	if err != nil {
		return string(raw)
	}
	return string(out)
}

// defaultString returns fallback when v is empty.
func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
