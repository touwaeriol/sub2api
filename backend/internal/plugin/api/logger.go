package api

import (
	"log/slog"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// sdkLogger adapts *slog.Logger to the plugin.Logger contract. The adapter
// injects a stable "plugin=<id>" attribute on every call so host-side log
// aggregation can route entries per plugin.
type sdkLogger struct {
	inner *slog.Logger
}

// newLogger returns a plugin.Logger tagged with pluginID. The host-supplied
// slog.Logger is the base; the adapter adds the plugin attribute once via
// With, avoiding per-call allocations.
func newLogger(base *slog.Logger, pluginID string) plugin.Logger {
	if base == nil {
		base = slog.Default()
	}
	return &sdkLogger{inner: base.With("plugin", pluginID)}
}

// Debug forwards to slog.Debug with the plugin tag pre-attached.
func (l *sdkLogger) Debug(msg string, kv ...any) { l.inner.Debug(msg, kv...) }

// Info forwards to slog.Info.
func (l *sdkLogger) Info(msg string, kv ...any) { l.inner.Info(msg, kv...) }

// Warn forwards to slog.Warn.
func (l *sdkLogger) Warn(msg string, kv ...any) { l.inner.Warn(msg, kv...) }

// Error forwards to slog.Error.
func (l *sdkLogger) Error(msg string, kv ...any) { l.inner.Error(msg, kv...) }
