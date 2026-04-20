package paramoverride

import (
	"encoding/json"
	"regexp"
)

// Target identifies where a rule mutates the outgoing request.
const (
	TargetBody   = "body"
	TargetHeader = "header"
)

// Action identifies the mutation performed by a rule.
const (
	ActionSet    = "set"
	ActionMerge  = "merge"
	ActionRemove = "remove"
	ActionAppend = "append"
)

// Limits for a single Compile invocation. These also act as hard caps for the
// admin validation layer.
const (
	MaxRulesPerPlatform = 64
	MaxModelGlobLength  = 128
	MaxPathLength       = 256
)

// Rule is the user-facing representation of a single override rule. Callers
// (typically the service layer) translate their own rule structs into Rule
// before compilation.
type Rule struct {
	Enabled     bool
	ModelGlob   string
	Target      string          // TargetBody | TargetHeader
	Action      string          // ActionSet | ActionMerge | ActionRemove | ActionAppend
	Path        string          // sjson path for body; header name for header
	Value       json.RawMessage // raw JSON payload; ignored when Action==ActionRemove
	Description string
}

// CompiledRule is the post-compilation representation used on the hot path.
// Fields are read-only once Compile returns.
type CompiledRule struct {
	Target string
	Action string
	Path   string

	// matchAll is true when the rule applies to every model (empty glob or "*").
	matchAll bool
	// modelRegex is nil when matchAll is true.
	modelRegex *regexp.Regexp

	// Value carries the raw JSON payload for set/merge/append actions.
	// For ActionRemove it is always nil.
	Value json.RawMessage
	// valueKind indicates the top-level JSON kind of Value after Unmarshal.
	// Used by the body applier to decide between sjson.SetBytes and
	// sjson.SetRawBytes.
	valueKind valueKind
}

// MatchesModel reports whether the rule applies to the given model name.
// Both sides are matched case-sensitively on the raw string; callers are
// expected to pass the exact model string they would forward upstream.
func (r *CompiledRule) MatchesModel(model string) bool {
	if r.matchAll {
		return true
	}
	if r.modelRegex == nil {
		return false
	}
	return r.modelRegex.MatchString(model)
}

// Compiled is an immutable snapshot indexed by platform.
type Compiled struct {
	byPlatform map[string][]CompiledRule
}

// IsEmpty reports whether the snapshot carries any rules.
func (c *Compiled) IsEmpty() bool {
	if c == nil {
		return true
	}
	for _, rules := range c.byPlatform {
		if len(rules) > 0 {
			return false
		}
	}
	return true
}

// valueKind categorizes the JSON payload type to select the correct sjson
// setter. Numbers/strings/booleans go through sjson.SetBytes while objects
// and arrays must use sjson.SetRawBytes to preserve nested structure.
type valueKind int

const (
	valueKindUnknown valueKind = iota
	valueKindPrimitive
	valueKindObject
	valueKindArray
	valueKindNull
)
