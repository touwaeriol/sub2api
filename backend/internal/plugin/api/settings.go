package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// settingsPrefix is prepended to every key persisted via NamespacedSettings
// so plugin settings never collide with the host's system settings table.
const settingsPrefix = "plugin:"

// settingsAPI is the read-only NamespacedSettings for Phase 0. Writes and
// OnChange reach into SettingService's update path, which requires callback
// wiring; those are TODO items and return ErrNotImplemented today.
type settingsAPI struct {
	guard       *guard
	pluginID    string
	settingRepo service.SettingRepository
}

// newSettings binds the repository to the wrapper.
func newSettings(c *coreAPIImpl) plugin.NamespacedSettings {
	if c.deps.SettingRepo == nil {
		return unimplementedSettings{}
	}
	return &settingsAPI{
		guard:       c.guard,
		pluginID:    c.pluginID,
		settingRepo: c.deps.SettingRepo,
	}
}

// key returns the full storage key for a bare plugin-supplied key.
func (s *settingsAPI) key(bare string) string {
	return settingsPrefix + s.pluginID + ":" + strings.TrimSpace(bare)
}

// Get returns the raw string value as `any`, matching the SDK contract.
// Missing keys return (nil, nil) so callers can distinguish "absent" from
// "present but empty".
func (s *settingsAPI) Get(ctx context.Context, key string) (any, error) {
	val, err := s.settingRepo.GetValue(ctx, s.key(key))
	if err != nil {
		if isSettingNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin settings get: %w", err)
	}
	return val, nil
}

// Set is not implemented in Phase 0 — the host SettingService has no
// namespace-aware validator yet, so writes are gated until the policy is
// defined.
func (s *settingsAPI) Set(context.Context, string, any) error {
	return fmt.Errorf("plugin settings set: %w", plugin.ErrNotImplemented)
}

// OnChange is a TODO: the host publishes TopicPluginSettingsChanged on the
// event bus but the local subscription path is wired in a later wave.
func (s *settingsAPI) OnChange(string, func(context.Context, any, any)) {}

// isSettingNotFound reports whether err is the repository's "not found"
// error. The repo wraps pg.ErrNoRows inside a formatted message; a
// substring check is good enough for Phase 0 until the repo returns a
// sentinel.
func isSettingNotFound(err error) bool {
	if err == nil {
		return false
	}
	var target interface{ Error() string }
	if errors.As(err, &target) {
		msg := strings.ToLower(target.Error())
		return strings.Contains(msg, "no rows") || strings.Contains(msg, "not found")
	}
	return false
}

// unimplementedSettings stands in when no SettingRepository is wired.
type unimplementedSettings struct{}

func (unimplementedSettings) Get(context.Context, string) (any, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedSettings) Set(context.Context, string, any) error {
	return plugin.ErrNotImplemented
}
func (unimplementedSettings) OnChange(string, func(context.Context, any, any)) {}
