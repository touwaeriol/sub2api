package eventbus

import "log/slog"

// slogErrorf is a tiny helper so callers don't need to import slog for a
// one-off Error log. Kept package-local and unexported.
func slogErrorf(msg string, kv ...any) {
	slog.Error(msg, kv...)
}
