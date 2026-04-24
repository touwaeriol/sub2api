package plugin

import (
	"fmt"
	"sort"
	"sync"
)

// globalRegistry is the process-wide registry populated by plugin init
// functions through [Register]. The host reads it once during boot.
var globalRegistry = struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}{plugins: make(map[string]Plugin)}

// Register adds a plugin to the process-wide registry. Plugins MUST call
// this exactly once from an init() function:
//
//	func init() { plugin.Register(&myPlugin{}) }
//
// Register panics on duplicate ids or empty Meta.ID — these are programming
// errors that must fail fast during startup rather than produce a silently
// half-loaded process.
func Register(p Plugin) {
	if p == nil {
		panic("plugin: Register called with nil plugin")
	}
	meta := p.Meta()
	if meta.ID == "" {
		panic("plugin: Register called with empty Meta.ID")
	}

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	if _, dup := globalRegistry.plugins[meta.ID]; dup {
		panic(fmt.Errorf("%w: %q", ErrDuplicateRegistration, meta.ID))
	}
	globalRegistry.plugins[meta.ID] = p
}

// Registered returns every plugin registered so far, sorted by Meta.ID for
// deterministic iteration. Intended for the host's loader at boot time.
func Registered() []Plugin {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	out := make([]Plugin, 0, len(globalRegistry.plugins))
	for _, p := range globalRegistry.plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta().ID < out[j].Meta().ID })
	return out
}

// Lookup returns the plugin registered under id, if any. Host code (and
// [PluginAs]) uses it to resolve peer plugins.
func Lookup(id string) (Plugin, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	p, ok := globalRegistry.plugins[id]
	return p, ok
}
