// Package admin plugin_handler.go exposes the lifecycle + observability
// HTTP endpoints for the plugin subsystem. Read-only data comes from the
// Loader/PluginRepository; mutating transitions delegate to the Loader.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/plugin/eventbus"
	"github.com/Wei-Shaw/sub2api/internal/plugin/loader"
	"github.com/Wei-Shaw/sub2api/internal/plugin/repository"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"github.com/gin-gonic/gin"
)

// pluginLifecycle is the subset of *loader.Loader consumed by the handler.
// Declared as an interface so unit tests can inject a stub.
type pluginLifecycle interface {
	Install(ctx context.Context, p plugin.Plugin) error
	Enable(ctx context.Context, id string) error
	Disable(ctx context.Context, id string) error
	Uninstall(ctx context.Context, id string, purge bool) error
	ListStates(ctx context.Context) ([]loader.PluginState, error)
	FindState(ctx context.Context, id string) (*loader.PluginState, error)
}

// deadLetterRetrier abstracts the dependency on *eventbus.Bus so tests can
// skip constructing a full bus. Returning nil signals "async not wired".
type deadLetterRetrier interface {
	Retrier() eventbus.DeadLetterRetrier
}

// busAdapter wraps *eventbus.Bus into deadLetterRetrier.
type busAdapter struct {
	bus *eventbus.Bus
}

// Retrier builds a DeadLetterRetrier backed by the async dispatcher.
func (a *busAdapter) Retrier() eventbus.DeadLetterRetrier {
	return buildRetrier(a.bus)
}

// PluginAdminHandler serves the /api/v1/admin/plugins endpoints.
//
// The loader is passed as a concrete *loader.Loader for ease of wiring;
// the handler itself uses a narrow interface so tests can stub it. Dead-
// letter operations use the eventbus.DeadLetterRepo and *eventbus.Bus.
type PluginAdminHandler struct {
	loader         pluginLifecycle
	pluginRepo     repository.PluginRepository
	deadLetterRepo eventbus.DeadLetterRepo
	busAdapter     deadLetterRetrier
	logger         *slog.Logger
}

// NewPluginAdminHandler wires the collaborators. A nil logger falls back
// to slog.Default() so tests don't need to construct one.
func NewPluginAdminHandler(
	l *loader.Loader,
	repo repository.PluginRepository,
	dlr eventbus.DeadLetterRepo,
	bus *eventbus.Bus,
	logger *slog.Logger,
) *PluginAdminHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PluginAdminHandler{
		loader:         l,
		pluginRepo:     repo,
		deadLetterRepo: dlr,
		busAdapter:     &busAdapter{bus: bus},
		logger:         logger,
	}
}

// -------------------------------------------------------------------------
// Response helpers
// -------------------------------------------------------------------------

// pluginErrorResponse is the on-the-wire shape mandated by CLAUDE.md for
// error responses: { code, message, details? }.
type pluginErrorResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// pluginDataResponse is the envelope shared by all success responses.
type pluginDataResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// writePluginError sends a structured error using the required format.
func writePluginError(c *gin.Context, status, code int, message string, details map[string]any) {
	c.JSON(status, pluginErrorResponse{Code: code, Message: message, Details: details})
}

// writePluginSuccess sends a successful response.
func writePluginSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, pluginDataResponse{Code: 0, Message: "success", Data: data})
}

// -------------------------------------------------------------------------
// List / Detail
// -------------------------------------------------------------------------

// ListPlugins handles GET /api/v1/admin/plugins.
func (h *PluginAdminHandler) ListPlugins(c *gin.Context) {
	states, err := h.loader.ListStates(c.Request.Context())
	if err != nil {
		h.logger.Error("list plugin states failed", "error", err)
		writePluginError(c, http.StatusInternalServerError, ErrCodePluginInternal,
			"failed to list plugin states", nil)
		return
	}
	registry := registeredByID()
	items := make([]dto.PluginResponse, 0, len(states)+len(registry))
	seen := map[string]struct{}{}
	for _, st := range states {
		seen[st.ID] = struct{}{}
		items = append(items, buildAdminListDTO(st, registry[st.ID]))
	}
	// Surface plugins compiled in but not yet installed so operators can
	// trigger Install.
	for id, p := range registry {
		if _, ok := seen[id]; ok {
			continue
		}
		items = append(items, buildAdminListDTO(loader.PluginState{
			ID:    id,
			State: dto.PluginStateUninstalled,
		}, p))
	}
	writePluginSuccess(c, gin.H{"plugins": items})
}

// GetPlugin handles GET /api/v1/admin/plugins/:id.
func (h *PluginAdminHandler) GetPlugin(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		writePluginError(c, http.StatusBadRequest, ErrCodePluginBadRequest,
			"plugin id is required", nil)
		return
	}
	state, err := h.loader.FindState(c.Request.Context(), id)
	registryEntry, registered := plugin.Lookup(id)
	if err != nil && errors.Is(err, repository.ErrPluginNotFound) && !registered {
		writePluginError(c, http.StatusNotFound, ErrCodePluginNotFound,
			"plugin not found", map[string]any{ErrDetailPluginID: id})
		return
	}
	if err != nil && !errors.Is(err, repository.ErrPluginNotFound) {
		h.logger.Error("find plugin state failed", "plugin", id, "error", err)
		writePluginError(c, http.StatusInternalServerError, ErrCodePluginInternal,
			"failed to find plugin state", map[string]any{ErrDetailPluginID: id})
		return
	}
	if state == nil {
		state = &loader.PluginState{ID: id, State: dto.PluginStateUninstalled}
	}
	writePluginSuccess(c, buildAdminDetailDTO(*state, registryEntry))
}

// -------------------------------------------------------------------------
// Lifecycle transitions
// -------------------------------------------------------------------------

// InstallPlugin handles POST /api/v1/admin/plugins/:id/install.
func (h *PluginAdminHandler) InstallPlugin(c *gin.Context) {
	id := c.Param("id")
	p, ok := plugin.Lookup(id)
	if !ok {
		writePluginError(c, http.StatusNotFound, ErrCodePluginNotFound,
			"plugin not registered", map[string]any{ErrDetailPluginID: id})
		return
	}
	if err := h.loader.Install(c.Request.Context(), p); err != nil {
		h.handleLifecycleError(c, id, "install", err)
		return
	}
	writePluginSuccess(c, gin.H{"plugin_id": id, "state": dto.PluginStateInstalled})
}

// EnablePlugin handles POST /api/v1/admin/plugins/:id/enable.
func (h *PluginAdminHandler) EnablePlugin(c *gin.Context) {
	id := c.Param("id")
	if err := h.loader.Enable(c.Request.Context(), id); err != nil {
		h.handleLifecycleError(c, id, "enable", err)
		return
	}
	writePluginSuccess(c, gin.H{"plugin_id": id, "state": dto.PluginStateEnabled})
}

// DisablePlugin handles POST /api/v1/admin/plugins/:id/disable.
func (h *PluginAdminHandler) DisablePlugin(c *gin.Context) {
	id := c.Param("id")
	if err := h.loader.Disable(c.Request.Context(), id); err != nil {
		h.handleLifecycleError(c, id, "disable", err)
		return
	}
	writePluginSuccess(c, gin.H{"plugin_id": id, "state": dto.PluginStateDisabled})
}

// UninstallRequest is the optional body for uninstall.
type UninstallRequest struct {
	Purge bool `json:"purge"`
}

// UninstallPlugin handles POST /api/v1/admin/plugins/:id/uninstall.
func (h *PluginAdminHandler) UninstallPlugin(c *gin.Context) {
	id := c.Param("id")
	var req UninstallRequest
	// Body is optional — empty body means purge=false.
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			writePluginError(c, http.StatusBadRequest, ErrCodePluginBadRequest,
				"invalid uninstall request body", map[string]any{ErrDetailPluginID: id})
			return
		}
	}
	if err := h.loader.Uninstall(c.Request.Context(), id, req.Purge); err != nil {
		h.handleLifecycleError(c, id, "uninstall", err)
		return
	}
	target := dto.PluginStateUninstalled
	if req.Purge {
		target = "purged"
	}
	writePluginSuccess(c, gin.H{"plugin_id": id, "state": target})
}

// handleLifecycleError maps loader errors to a stable HTTP response.
func (h *PluginAdminHandler) handleLifecycleError(c *gin.Context, id, op string, err error) {
	h.logger.Error("plugin lifecycle operation failed", "op", op, "plugin", id, "error", err)
	switch {
	case errors.Is(err, plugin.ErrPluginNotFound), errors.Is(err, repository.ErrPluginNotFound):
		writePluginError(c, http.StatusNotFound, ErrCodePluginNotFound,
			"plugin not found", map[string]any{ErrDetailPluginID: id})
	case errors.Is(err, loader.ErrInvalidState):
		writePluginError(c, http.StatusConflict, ErrCodePluginInvalidState,
			err.Error(), map[string]any{ErrDetailPluginID: id})
	default:
		writePluginError(c, http.StatusInternalServerError, ErrCodePluginLifecycle,
			err.Error(), map[string]any{ErrDetailPluginID: id})
	}
}

// -------------------------------------------------------------------------
// Dead letters
// -------------------------------------------------------------------------

// ListDeadLetters handles GET /api/v1/admin/plugins/dead-letters.
func (h *PluginAdminHandler) ListDeadLetters(c *gin.Context) {
	filter, err := parseDeadLetterFilter(c)
	if err != nil {
		writePluginError(c, http.StatusBadRequest, ErrCodePluginBadRequest,
			err.Error(), nil)
		return
	}
	entries, err := h.deadLetterRepo.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("list dead letters failed", "error", err)
		writePluginError(c, http.StatusInternalServerError, ErrCodeDeadLetterListFailed,
			"failed to list dead letters", nil)
		return
	}
	items := make([]dto.DeadLetterDTO, 0, len(entries))
	for _, e := range entries {
		items = append(items, mapDeadLetterToDTO(e))
	}
	writePluginSuccess(c, gin.H{
		"items":     items,
		"page":      pageFromContext(c),
		"page_size": filter.Limit,
	})
}

// RetryDeadLetter handles POST /api/v1/admin/plugins/dead-letters/:id/retry.
func (h *PluginAdminHandler) RetryDeadLetter(c *gin.Context) {
	id, err := parseDeadLetterID(c)
	if err != nil {
		writePluginError(c, http.StatusBadRequest, ErrCodePluginBadRequest,
			err.Error(), nil)
		return
	}
	var retrier eventbus.DeadLetterRetrier
	if h.busAdapter != nil {
		retrier = h.busAdapter.Retrier()
	}
	if retrier == nil {
		writePluginError(c, http.StatusServiceUnavailable, ErrCodeDeadLetterRetryFailed,
			"event bus async dispatcher is not configured", nil)
		return
	}
	if err := h.deadLetterRepo.Retry(c.Request.Context(), id, retrier); err != nil {
		if errors.Is(err, eventbus.ErrDeadLetterNotFound) {
			writePluginError(c, http.StatusNotFound, ErrCodeDeadLetterNotFound,
				"dead letter not found", map[string]any{ErrDetailDeadLetterID: id})
			return
		}
		h.logger.Error("retry dead letter failed", "dead_letter_id", id, "error", err)
		writePluginError(c, http.StatusInternalServerError, ErrCodeDeadLetterRetryFailed,
			err.Error(), map[string]any{ErrDetailDeadLetterID: id})
		return
	}
	writePluginSuccess(c, gin.H{ErrDetailDeadLetterID: id, "retried": true})
}

// DeleteDeadLetter handles DELETE /api/v1/admin/plugins/dead-letters/:id.
func (h *PluginAdminHandler) DeleteDeadLetter(c *gin.Context) {
	id, err := parseDeadLetterID(c)
	if err != nil {
		writePluginError(c, http.StatusBadRequest, ErrCodePluginBadRequest,
			err.Error(), nil)
		return
	}
	if err := h.deadLetterRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, eventbus.ErrDeadLetterNotFound) {
			writePluginError(c, http.StatusNotFound, ErrCodeDeadLetterNotFound,
				"dead letter not found", map[string]any{ErrDetailDeadLetterID: id})
			return
		}
		h.logger.Error("delete dead letter failed", "dead_letter_id", id, "error", err)
		writePluginError(c, http.StatusInternalServerError, ErrCodeDeadLetterDeleteFailed,
			err.Error(), map[string]any{ErrDetailDeadLetterID: id})
		return
	}
	writePluginSuccess(c, gin.H{ErrDetailDeadLetterID: id, "deleted": true})
}

// parseDeadLetterID extracts ":id" as int64.
func parseDeadLetterID(c *gin.Context) (int64, error) {
	raw := c.Param("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid dead letter id")
	}
	return id, nil
}

// parseDeadLetterFilter reads page/page_size/topic/subscriber_tag query
// params and normalises them into a DeadLetterFilter.
func parseDeadLetterFilter(c *gin.Context) (eventbus.DeadLetterFilter, error) {
	page := defaultDeadLetterPage
	pageSize := defaultDeadLetterPageSize
	if raw := c.Query("page"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return eventbus.DeadLetterFilter{}, errors.New("invalid page parameter")
		}
		page = v
	}
	if raw := c.Query("page_size"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > maxDeadLetterPageSize {
			return eventbus.DeadLetterFilter{}, errors.New("invalid page_size parameter")
		}
		pageSize = v
	}
	topic := strings.TrimSpace(c.Query("topic"))
	subscriberTag := strings.TrimSpace(c.Query("subscriber_tag"))
	if subscriberTag == "" {
		// `plugin_id` is the friendlier alias; map it to subscriber_tag.
		subscriberTag = strings.TrimSpace(c.Query("plugin_id"))
	}
	return eventbus.DeadLetterFilter{
		Topic:         topic,
		SubscriberTag: subscriberTag,
		Limit:         pageSize,
		Offset:        (page - 1) * pageSize,
	}, nil
}

// pageFromContext echoes the page param back, defaulting to 1.
func pageFromContext(c *gin.Context) int {
	if raw := c.Query("page"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	return defaultDeadLetterPage
}

// buildRetrier converts the async dispatcher into the DeadLetterRetrier
// signature the repo expects. Returns nil when the bus has no async
// dispatcher wired (tests / sync-only deployments).
func buildRetrier(bus *eventbus.Bus) eventbus.DeadLetterRetrier {
	if bus == nil || bus.AsyncDispatcher() == nil {
		return nil
	}
	dispatcher := bus.AsyncDispatcher()
	return func(ctx context.Context, topic string, payload []byte) error {
		// Payload is already JSON-encoded; round-trip through an untyped
		// decode so the dispatcher's marshal produces a payload satisfying
		// the registered schema.
		var decoded any
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return err
		}
		return dispatcher.Publish(ctx, topic, decoded)
	}
}
