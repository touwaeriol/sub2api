package paramoverride

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// ApplyToHeaders mutates the given http.Header in place. Rules whose Target
// is not TargetHeader are skipped. Body-specific actions do not appear here:
// ActionAppend is only defined for headers.
//
// Rule value payloads are interpreted as strings (for set/append) or
// ignored (for remove). Non-string payloads for set/append trigger a
// structured warning and the rule is skipped.
func ApplyToHeaders(h http.Header, rules []CompiledRule) {
	if h == nil || len(rules) == 0 {
		return
	}
	for i := range rules {
		rule := &rules[i]
		if rule.Target != TargetHeader {
			continue
		}
		applyHeaderRule(h, rule)
	}
}

// applyHeaderRule dispatches on the rule action for a single header.
func applyHeaderRule(h http.Header, rule *CompiledRule) {
	switch rule.Action {
	case ActionRemove:
		h.Del(rule.Path)
	case ActionSet:
		value, ok := decodeHeaderValue(rule)
		if !ok {
			return
		}
		h.Set(rule.Path, value)
	case ActionAppend:
		value, ok := decodeHeaderValue(rule)
		if !ok {
			return
		}
		appendHeaderValue(h, rule.Path, value)
	default:
		// ActionMerge has no defined semantics for HTTP headers.
		slog.Warn("paramoverride: header rule skipped (unsupported action)",
			"action", rule.Action,
			"name", rule.Path,
		)
	}
}

// decodeHeaderValue unmarshals the rule's JSON value to a string. The return
// boolean indicates whether the value is usable.
func decodeHeaderValue(rule *CompiledRule) (string, bool) {
	if len(rule.Value) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(rule.Value, &s); err != nil {
		slog.Warn("paramoverride: header rule skipped (value not a string)",
			"action", rule.Action,
			"name", rule.Path,
			"error", err.Error(),
		)
		return "", false
	}
	return s, true
}

// appendHeaderValue appends value to the existing comma-separated list under
// name, deduplicating so repeated Compile→Apply cycles stay idempotent.
func appendHeaderValue(h http.Header, name, value string) {
	existing := h.Get(name)
	if existing == "" {
		h.Set(name, value)
		return
	}
	for _, part := range strings.Split(existing, ",") {
		if strings.TrimSpace(part) == value {
			return
		}
	}
	h.Set(name, existing+","+value)
}
