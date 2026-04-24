// Package loader implements the plugin lifecycle: resolving the
// dependency order declared in Meta.Dependencies, applying schema and
// migrations, and driving Init/Start/Shutdown transitions.
package loader

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"golang.org/x/mod/semver"
)

// Sort returns the plugins ordered so that each plugin's required
// dependencies appear before it. Unknown dependencies, version mismatches
// and cycles are surfaced as errors.
//
// The algorithm is an iterative DFS that colours each node white / grey /
// black (unvisited / on-stack / finished) so cycles are detected as
// back-edges.
func Sort(plugins []plugin.Plugin) ([]plugin.Plugin, error) {
	index, err := indexPlugins(plugins)
	if err != nil {
		return nil, err
	}
	if err := validateDeps(plugins, index); err != nil {
		return nil, err
	}
	return topologicalSort(plugins, index)
}

// indexPlugins builds an id -> Plugin map and rejects duplicates.
func indexPlugins(plugins []plugin.Plugin) (map[string]plugin.Plugin, error) {
	index := make(map[string]plugin.Plugin, len(plugins))
	for _, p := range plugins {
		id := p.Meta().ID
		if id == "" {
			return nil, fmt.Errorf("loader: plugin with empty Meta.ID")
		}
		if _, dup := index[id]; dup {
			return nil, fmt.Errorf("%w: %q", plugin.ErrDuplicateRegistration, id)
		}
		index[id] = p
	}
	return index, nil
}

// validateDeps checks that every required dependency exists and satisfies
// the declared version range. Missing Optional deps are silently dropped.
func validateDeps(plugins []plugin.Plugin, index map[string]plugin.Plugin) error {
	for _, p := range plugins {
		for _, dep := range p.Meta().Dependencies {
			depPlugin, ok := index[dep.ID]
			if !ok {
				if dep.Optional {
					continue
				}
				return fmt.Errorf("loader: plugin %q requires missing dependency %q",
					p.Meta().ID, dep.ID)
			}
			if err := checkVersionRange(dep.VersionRange, depPlugin.Meta().Version); err != nil {
				return fmt.Errorf("loader: plugin %q dependency %q: %w",
					p.Meta().ID, dep.ID, err)
			}
		}
	}
	return nil
}

// checkVersionRange returns nil if version satisfies range spec. Empty spec
// accepts any version. Supported operators: "^x.y.z", ">=x", ">x", "<=x",
// "<x", "=x" (the last is the default when no operator prefix is present).
// Multiple clauses separated by whitespace are AND-combined.
func checkVersionRange(spec, version string) error {
	if spec == "" {
		return nil
	}
	ver := withVPrefix(version)
	if !semver.IsValid(ver) {
		return fmt.Errorf("invalid dep version %q", version)
	}
	for _, clause := range strings.Fields(spec) {
		if err := matchClause(clause, ver); err != nil {
			return err
		}
	}
	return nil
}

// matchClause evaluates a single range clause against ver (already v-prefixed).
func matchClause(clause, ver string) error {
	op, operand := splitClause(clause)
	bound := withVPrefix(operand)
	if !semver.IsValid(bound) {
		return fmt.Errorf("invalid version clause %q", clause)
	}
	cmp := semver.Compare(ver, bound)
	switch op {
	case "^":
		if semver.Major(ver) != semver.Major(bound) || cmp < 0 {
			return fmt.Errorf("version %s does not satisfy %s", ver, clause)
		}
	case ">=":
		if cmp < 0 {
			return fmt.Errorf("version %s does not satisfy %s", ver, clause)
		}
	case ">":
		if cmp <= 0 {
			return fmt.Errorf("version %s does not satisfy %s", ver, clause)
		}
	case "<=":
		if cmp > 0 {
			return fmt.Errorf("version %s does not satisfy %s", ver, clause)
		}
	case "<":
		if cmp >= 0 {
			return fmt.Errorf("version %s does not satisfy %s", ver, clause)
		}
	default: // "=" — exact match on major.minor.patch
		if cmp != 0 {
			return fmt.Errorf("version %s does not satisfy %s", ver, clause)
		}
	}
	return nil
}

// splitClause separates the operator prefix from the version operand.
func splitClause(clause string) (op, operand string) {
	for _, prefix := range []string{">=", "<=", "^", ">", "<", "="} {
		if strings.HasPrefix(clause, prefix) {
			return prefix, strings.TrimSpace(clause[len(prefix):])
		}
	}
	return "=", clause
}

// withVPrefix ensures a "v" prefix so golang.org/x/mod/semver accepts it.
func withVPrefix(v string) string {
	if v == "" || v[0] == 'v' {
		return v
	}
	return "v" + v
}

// topologicalSort returns plugins in an order that honours dependencies.
// Grey-node re-visits are reported as plugin.ErrCircularDependency.
func topologicalSort(plugins []plugin.Plugin, index map[string]plugin.Plugin) ([]plugin.Plugin, error) {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	colour := make(map[string]int, len(plugins))
	out := make([]plugin.Plugin, 0, len(plugins))

	// Visit in deterministic alphabetic order for repeatable output.
	ids := make([]string, 0, len(plugins))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var visit func(id string, trail []string) error
	visit = func(id string, trail []string) error {
		switch colour[id] {
		case grey:
			return fmt.Errorf("%w: %s", plugin.ErrCircularDependency,
				strings.Join(append(trail, id), " -> "))
		case black:
			return nil
		}
		colour[id] = grey
		for _, dep := range index[id].Meta().Dependencies {
			if _, ok := index[dep.ID]; !ok {
				continue // optional missing dep already validated
			}
			if err := visit(dep.ID, append(trail, id)); err != nil {
				return err
			}
		}
		colour[id] = black
		out = append(out, index[id])
		return nil
	}

	for _, id := range ids {
		if err := visit(id, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}
