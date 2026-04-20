package paramoverride

import (
	"encoding/json"
	"log/slog"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ApplyToBodyBytes mutates the JSON body according to the given rules and
// returns the new byte slice. Rules whose Target is not TargetBody are
// skipped (the header applier handles those).
//
// On any per-rule failure the original body segment is preserved and a
// structured warning is logged (without the raw value payload, which may
// contain user-supplied system prompts). The function never returns an
// error: an invalid body falls back to the input unchanged.
func ApplyToBodyBytes(body []byte, rules []CompiledRule) []byte {
	if len(rules) == 0 {
		return body
	}
	if !gjson.ValidBytes(body) {
		// Non-JSON body (possibly streaming or empty). Apply no mutations.
		return body
	}
	out := body
	for i := range rules {
		rule := &rules[i]
		if rule.Target != TargetBody {
			continue
		}
		next, err := applyBodyRule(out, rule)
		if err != nil {
			slog.Warn("paramoverride: body rule skipped",
				"action", rule.Action,
				"path", rule.Path,
				"error", err.Error(),
			)
			continue
		}
		out = next
	}
	return out
}

// applyBodyRule dispatches on Action. It always returns a new slice on
// success; callers should replace the current body with the result.
func applyBodyRule(body []byte, rule *CompiledRule) ([]byte, error) {
	switch rule.Action {
	case ActionSet:
		return bodySet(body, rule)
	case ActionMerge:
		return bodyMerge(body, rule)
	case ActionRemove:
		return sjson.DeleteBytes(body, rule.Path)
	default:
		// ActionAppend is rejected at compile time for body targets; this
		// branch is reached only if a caller constructs CompiledRule by
		// hand without using Compile.
		return body, ErrAppendOnBody
	}
}

// bodySet writes the value verbatim at the path, preserving nested object
// and array structure via SetRawBytes.
func bodySet(body []byte, rule *CompiledRule) ([]byte, error) {
	if rule.valueKind == valueKindObject || rule.valueKind == valueKindArray {
		return sjson.SetRawBytes(body, rule.Path, rule.Value)
	}
	// Primitives (string / number / bool / null) are unmarshalled so sjson
	// can re-encode them correctly via SetBytes.
	var v any
	if err := json.Unmarshal(rule.Value, &v); err != nil {
		return body, err
	}
	return sjson.SetBytes(body, rule.Path, v)
}

// bodyMerge performs a shallow merge for object values at the target path.
// If the existing value at path is not an object (or missing), the merge
// falls back to a Set. Non-object values fall through to Set as well.
func bodyMerge(body []byte, rule *CompiledRule) ([]byte, error) {
	if rule.valueKind != valueKindObject {
		return bodySet(body, rule)
	}
	existing := gjson.GetBytes(body, rule.Path)
	if !existing.Exists() || !existing.IsObject() {
		return sjson.SetRawBytes(body, rule.Path, rule.Value)
	}
	merged, err := mergeObjects([]byte(existing.Raw), rule.Value)
	if err != nil {
		return body, err
	}
	return sjson.SetRawBytes(body, rule.Path, merged)
}

// mergeObjects performs a shallow merge: fields in override replace fields
// in base; base fields absent from override are kept. Nested objects are
// not recursively merged; callers who need that should emit deeper-path
// rules.
func mergeObjects(baseRaw, overrideRaw []byte) ([]byte, error) {
	var base map[string]json.RawMessage
	if err := json.Unmarshal(baseRaw, &base); err != nil {
		return nil, err
	}
	var override map[string]json.RawMessage
	if err := json.Unmarshal(overrideRaw, &override); err != nil {
		return nil, err
	}
	if base == nil {
		base = make(map[string]json.RawMessage, len(override))
	}
	for k, v := range override {
		base[k] = v
	}
	return json.Marshal(base)
}
