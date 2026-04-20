package paramoverride

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Compile validates and pre-processes the per-platform rule set. The returned
// snapshot is safe for concurrent read access. Compile returns a non-nil
// *Compiled even when the input is empty.
//
// Validation is strict: any invalid rule fails the whole Compile call with an
// error wrapping the relevant sentinel from errors.go. The error message
// includes the platform and rule index so callers can surface a structured
// response.
func Compile(perPlatform map[string][]Rule) (*Compiled, error) {
	snapshot := &Compiled{
		byPlatform: make(map[string][]CompiledRule, len(perPlatform)),
	}
	for platform, rules := range perPlatform {
		if len(rules) > MaxRulesPerPlatform {
			return nil, fmt.Errorf("platform %q: %w (got %d, max %d)",
				platform, ErrTooManyRules, len(rules), MaxRulesPerPlatform)
		}
		compiled, err := compilePlatformRules(platform, rules)
		if err != nil {
			return nil, err
		}
		snapshot.byPlatform[platform] = compiled
	}
	return snapshot, nil
}

// compilePlatformRules compiles a single platform's rule list, skipping any
// rule whose Enabled flag is false.
func compilePlatformRules(platform string, rules []Rule) ([]CompiledRule, error) {
	compiled := make([]CompiledRule, 0, len(rules))
	for idx, r := range rules {
		if !r.Enabled {
			continue
		}
		cr, err := compileOne(r)
		if err != nil {
			return nil, fmt.Errorf("platform %q rule #%d: %w", platform, idx, err)
		}
		compiled = append(compiled, cr)
	}
	return compiled, nil
}

// compileOne validates a single Rule and returns its compiled counterpart.
func compileOne(r Rule) (CompiledRule, error) {
	if err := validateShape(r); err != nil {
		return CompiledRule{}, err
	}
	regex, matchAll, err := compileGlob(r.ModelGlob)
	if err != nil {
		return CompiledRule{}, err
	}
	kind, err := classifyValue(r.Action, r.Value)
	if err != nil {
		return CompiledRule{}, err
	}
	return CompiledRule{
		Target:     r.Target,
		Action:     r.Action,
		Path:       r.Path,
		matchAll:   matchAll,
		modelRegex: regex,
		Value:      cloneValue(r.Action, r.Value),
		valueKind:  kind,
	}, nil
}

// validateShape checks the static shape of a rule (enum values, path length,
// required-value constraint). It does not touch the glob or the value payload.
func validateShape(r Rule) error {
	if r.Target != TargetBody && r.Target != TargetHeader {
		return ErrInvalidTarget
	}
	switch r.Action {
	case ActionSet, ActionMerge, ActionRemove, ActionAppend:
	default:
		return ErrInvalidAction
	}
	if r.Action == ActionAppend && r.Target != TargetHeader {
		return ErrAppendOnBody
	}
	if r.Path == "" {
		return ErrPathRequired
	}
	if len(r.Path) > MaxPathLength {
		return ErrPathTooLong
	}
	if len(r.ModelGlob) > MaxModelGlobLength {
		return ErrGlobTooLong
	}
	if r.Action != ActionRemove && len(r.Value) == 0 {
		return ErrValueRequired
	}
	return nil
}

// compileGlob translates a user glob ("*", "?", literals) into a regexp.
// An empty glob or "*" is treated as "match all" and no regex is compiled.
func compileGlob(glob string) (*regexp.Regexp, bool, error) {
	if glob == "" || glob == "*" {
		return nil, true, nil
	}
	var b strings.Builder
	b.Grow(len(glob) + 4)
	_, _ = b.WriteString("^")
	for _, ch := range glob {
		switch ch {
		case '*':
			_, _ = b.WriteString(".*")
		case '?':
			_, _ = b.WriteString(".")
		default:
			_, _ = b.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	_, _ = b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, false, fmt.Errorf("%w: %s", ErrInvalidGlob, err.Error())
	}
	return re, false, nil
}

// classifyValue inspects the raw JSON payload to decide how the applier must
// write it back. Remove actions have no value and return valueKindUnknown.
func classifyValue(action string, value json.RawMessage) (valueKind, error) {
	if action == ActionRemove {
		return valueKindUnknown, nil
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return valueKindUnknown, ErrValueRequired
	}
	// Validate that the payload is syntactically legal JSON.
	var tmp any
	if err := json.Unmarshal(trimmed, &tmp); err != nil {
		return valueKindUnknown, fmt.Errorf("%w: %s", ErrInvalidValue, err.Error())
	}
	switch trimmed[0] {
	case '{':
		return valueKindObject, nil
	case '[':
		return valueKindArray, nil
	case 'n':
		return valueKindNull, nil
	default:
		return valueKindPrimitive, nil
	}
}

// cloneValue copies the raw payload so the compiled rule is decoupled from
// the caller's slice storage. Remove actions retain a nil payload.
func cloneValue(action string, value json.RawMessage) json.RawMessage {
	if action == ActionRemove || len(value) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(value))
	copy(out, value)
	return out
}
