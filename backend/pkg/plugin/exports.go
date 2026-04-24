package plugin

import "fmt"

// PluginAs looks up another plugin by id and asserts its Meta.Exports to T.
// It is the canonical way for plugins to reach into peer plugins for typed
// cross-plugin API access:
//
//	type AlipayExports interface{ ReissueQR(id string) error }
//	api, err := plugin.PluginAs[AlipayExports](core, "alipay")
//
// The core argument is only used to locate the peer registry (so host tests
// can inject a mock CoreAPI); production implementations return the process-
// wide registry returned by [Registered]/[Lookup].
//
// Errors:
//   - [ErrPluginNotFound] when id is not registered.
//   - [ErrExportsTypeMismatch] when the peer's Meta.Exports cannot be
//     asserted to T.
func PluginAs[T any](core CoreAPI, id string) (T, error) {
	var zero T

	var p Plugin
	var ok bool
	if core != nil {
		p, ok = core.Plugins().Lookup(id)
	}
	if !ok {
		p, ok = Lookup(id)
	}
	if !ok {
		return zero, fmt.Errorf("%w: %q", ErrPluginNotFound, id)
	}

	exp := p.Meta().Exports
	if exp == nil {
		return zero, fmt.Errorf("%w: %q has no exports", ErrExportsTypeMismatch, id)
	}
	value, ok := exp.(T)
	if !ok {
		return zero, fmt.Errorf("%w: %q exports %T, not %T",
			ErrExportsTypeMismatch, id, exp, zero)
	}
	return value, nil
}
