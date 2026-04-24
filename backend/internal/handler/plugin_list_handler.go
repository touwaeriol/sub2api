package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/plugin/loader"
	"github.com/Wei-Shaw/sub2api/internal/plugin/repository"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"github.com/gin-gonic/gin"
)

// pluginStateLister is the minimal loader surface required by the public
// plugin list endpoint. Kept as an interface so tests can inject a stub
// without constructing a real loader.
type pluginStateLister interface {
	ListStates(ctx context.Context) ([]loader.PluginState, error)
}

// PluginListHandler serves GET /api/v1/plugins — the public bootstrap feed
// consumed by the frontend to discover enabled plugins (menus, form schemas,
// UI bundles). Only plugins currently in state=enabled are returned; secret
// setting defaults are never included in this feed.
type PluginListHandler struct {
	loader pluginStateLister
	logger *slog.Logger
}

// NewPluginListHandler wires the loader. nil logger → slog.Default().
func NewPluginListHandler(l *loader.Loader, logger *slog.Logger) *PluginListHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PluginListHandler{loader: l, logger: logger}
}

// ListEnabledPlugins returns the enabled plugins in the host. The response
// envelope mirrors the codebase convention: `{ code, data }`.
func (h *PluginListHandler) ListEnabledPlugins(c *gin.Context) {
	items, err := h.buildEnabledList(c.Request.Context())
	if err != nil {
		h.logger.Error("list enabled plugins failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "failed to list plugins",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{"plugins": items},
	})
}

// buildEnabledList queries the loader and matches each "enabled" row with
// the compiled-in plugin. Rows whose plugin is no longer registered are
// skipped: we can't publish menus/settings without the Meta anyway.
func (h *PluginListHandler) buildEnabledList(ctx context.Context) ([]dto.PluginPublicDTO, error) {
	states, err := h.loader.ListStates(ctx)
	if err != nil {
		if errors.Is(err, repository.ErrPluginNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]dto.PluginPublicDTO, 0, len(states))
	for _, st := range states {
		if st.State != dto.PluginStateEnabled {
			continue
		}
		p, ok := plugin.Lookup(st.ID)
		if !ok {
			continue
		}
		out = append(out, buildPublicDTOFromMeta(p))
	}
	return out, nil
}

// buildPublicDTOFromMeta is a thin projection wrapper; kept here so this
// file does not reach into the admin package for a shared mapper. The
// implementation mirrors admin.buildPublicDTO exactly.
func buildPublicDTOFromMeta(p plugin.Plugin) dto.PluginPublicDTO {
	meta := p.Meta()
	return dto.PluginPublicDTO{
		ID:          meta.ID,
		Name:        meta.Name,
		Version:     meta.Version,
		Description: meta.Description,
		Menus:       publicMenus(meta.Menus),
		Settings:    publicSettings(meta.Settings),
		Frontend:    publicFrontend(meta.Frontend),
	}
}

// publicMenus mirrors admin.mapMenus; duplicated locally to avoid an
// import cycle between handler and handler/admin.
func publicMenus(menus []plugin.MenuSpec) []dto.PluginMenuDTO {
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

// publicSettings masks secret defaults before returning the schema.
func publicSettings(settings []plugin.SettingSpec) []dto.PluginSettingDTO {
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
			Schema:      publicSchema(s.Schema),
		}
		if !s.Secret {
			item.Default = s.Default
		}
		out = append(out, item)
	}
	return out
}

// publicSchema walks UIFieldSchema recursively.
func publicSchema(in plugin.UIFieldSchema) dto.PluginUIFieldSchemaDTO {
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
			out.Properties[k] = publicSchema(v)
		}
	}
	if in.Items != nil {
		nested := publicSchema(*in.Items)
		out.Items = &nested
	}
	return out
}

// publicFrontend copies FrontendSpec; nil in, nil out.
func publicFrontend(in *plugin.FrontendSpec) *dto.PluginFrontendDTO {
	if in == nil {
		return nil
	}
	return &dto.PluginFrontendDTO{
		BundleName: in.BundleName,
		EntryURL:   in.EntryURL,
	}
}
