//go:build unit

package loader

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"github.com/stretchr/testify/require"
)

// stubPlugin is a minimal Plugin used to exercise topology sorting.
type stubPlugin struct {
	meta plugin.Meta
}

func (s *stubPlugin) Meta() plugin.Meta                { return s.meta }
func (s *stubPlugin) Init(_ plugin.CoreAPI) error      { return nil }
func (s *stubPlugin) Start(_ context.Context) error    { return nil }
func (s *stubPlugin) Shutdown(_ context.Context) error { return nil }

func newStub(id, version string, deps ...plugin.Dep) *stubPlugin {
	return &stubPlugin{meta: plugin.Meta{
		ID:           id,
		Version:      version,
		APIVersion:   plugin.SDKVersion,
		Dependencies: deps,
	}}
}

func TestSort_Linear(t *testing.T) {
	a := newStub("alpha", "1.0.0")
	b := newStub("beta", "1.0.0", plugin.Dep{ID: "alpha"})
	c := newStub("gamma", "1.0.0", plugin.Dep{ID: "beta"})

	ordered, err := Sort([]plugin.Plugin{c, a, b})
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta", "gamma"}, pluginIDs(ordered))
}

func TestSort_DetectsCycle(t *testing.T) {
	a := newStub("alpha", "1.0.0", plugin.Dep{ID: "beta"})
	b := newStub("beta", "1.0.0", plugin.Dep{ID: "alpha"})

	_, err := Sort([]plugin.Plugin{a, b})
	require.Error(t, err)
	require.ErrorIs(t, err, plugin.ErrCircularDependency)
}

func TestSort_MissingRequiredDep(t *testing.T) {
	a := newStub("alpha", "1.0.0", plugin.Dep{ID: "missing"})
	_, err := Sort([]plugin.Plugin{a})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing dependency")
}

func TestSort_MissingOptionalDepIgnored(t *testing.T) {
	a := newStub("alpha", "1.0.0", plugin.Dep{ID: "missing", Optional: true})
	ordered, err := Sort([]plugin.Plugin{a})
	require.NoError(t, err)
	require.Equal(t, []string{"alpha"}, pluginIDs(ordered))
}

func TestSort_VersionRangeUnsatisfied(t *testing.T) {
	a := newStub("alpha", "1.0.0")
	b := newStub("beta", "1.0.0", plugin.Dep{ID: "alpha", VersionRange: ">=2.0.0"})

	_, err := Sort([]plugin.Plugin{a, b})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not satisfy")
}

func TestSort_VersionRangeSatisfied(t *testing.T) {
	a := newStub("alpha", "1.2.0")
	b := newStub("beta", "1.0.0", plugin.Dep{ID: "alpha", VersionRange: "^1.0.0"})

	ordered, err := Sort([]plugin.Plugin{b, a})
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta"}, pluginIDs(ordered))
}

func TestSort_DuplicateIDRejected(t *testing.T) {
	a := newStub("alpha", "1.0.0")
	dup := newStub("alpha", "1.0.1")
	_, err := Sort([]plugin.Plugin{a, dup})
	require.Error(t, err)
	require.ErrorIs(t, err, plugin.ErrDuplicateRegistration)
}

func TestSort_EmptyIDRejected(t *testing.T) {
	bad := &stubPlugin{meta: plugin.Meta{Version: "1.0.0"}}
	_, err := Sort([]plugin.Plugin{bad})
	require.Error(t, err)
}

func TestCheckVersionRange_Clauses(t *testing.T) {
	// Empty range always passes.
	require.NoError(t, checkVersionRange("", "1.0.0"))
	// Exact match (no operator).
	require.NoError(t, checkVersionRange("1.2.3", "1.2.3"))
	// Caret matches same major, min version.
	require.NoError(t, checkVersionRange("^1.0.0", "1.5.0"))
	require.Error(t, checkVersionRange("^1.0.0", "2.0.0"))
	// Composite AND clauses.
	require.NoError(t, checkVersionRange(">=1.0.0 <2.0.0", "1.9.0"))
	require.Error(t, checkVersionRange(">=1.0.0 <2.0.0", "2.0.0"))
}

func TestSort_StableOutputOrdering(t *testing.T) {
	a := newStub("aaa", "1.0.0")
	b := newStub("bbb", "1.0.0")
	c := newStub("ccc", "1.0.0")
	// No dependencies — expect alphabetical order regardless of input order.
	ordered, err := Sort([]plugin.Plugin{c, b, a})
	require.NoError(t, err)
	require.Equal(t, []string{"aaa", "bbb", "ccc"}, pluginIDs(ordered))
}

func pluginIDs(ps []plugin.Plugin) []string {
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.Meta().ID)
	}
	return ids
}
