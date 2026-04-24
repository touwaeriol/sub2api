// Package api implements the host side of the sub2api plugin CoreAPI
// surface. The factory wires a CoreAPI instance per plugin id, scoped by
// the plugin's declared Meta.Permissions through a lightweight permission
// guard, and delegates every call to the pre-existing service layer.
//
// Phase 0 delivers the minimum viable subset (Accounts / Billing /
// HTTP / Cache / Crypto / Logger / Settings read-only); all other
// sub-APIs return plugin.ErrNotImplemented.
package api

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// guard is the shared helper embedded in each sub-API implementation. It
// carries the plugin id (for logging) and a lookup-cheap permission set
// derived once from Meta.Permissions.
type guard struct {
	pluginID string
	perms    map[plugin.Permission]struct{}
}

// newGuard materialises a guard from the Meta.Permissions slice. An empty
// or nil slice yields a guard that denies every permission check.
func newGuard(pluginID string, perms []plugin.Permission) *guard {
	set := make(map[plugin.Permission]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return &guard{pluginID: pluginID, perms: set}
}

// requirePerm returns plugin.ErrPermissionDenied (wrapped with context) if
// the plugin has not declared p in Meta.Permissions.
func (g *guard) requirePerm(p plugin.Permission) error {
	if _, ok := g.perms[p]; !ok {
		return fmt.Errorf("%w: plugin=%s perm=%s", plugin.ErrPermissionDenied, g.pluginID, p)
	}
	return nil
}

// requireAny returns nil if the plugin declared at least one of perms.
// Useful for sub-APIs that expose read+write methods and want callers to
// hold either permission.
func (g *guard) requireAny(perms ...plugin.Permission) error {
	for _, p := range perms {
		if _, ok := g.perms[p]; ok {
			return nil
		}
	}
	return fmt.Errorf("%w: plugin=%s need one of %v", plugin.ErrPermissionDenied, g.pluginID, perms)
}
