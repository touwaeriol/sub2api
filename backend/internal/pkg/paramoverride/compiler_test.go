//go:build unit

package paramoverride

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCompile_RejectsInvalidTarget(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {{
			Enabled: true,
			Target:  "cookie",
			Action:  ActionSet,
			Path:    "x",
			Value:   json.RawMessage(`"y"`),
		}},
	}
	_, err := Compile(rules)
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected ErrInvalidTarget, got %v", err)
	}
}

func TestCompile_RejectsInvalidAction(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {{
			Enabled: true,
			Target:  TargetBody,
			Action:  "patch",
			Path:    "x",
			Value:   json.RawMessage(`"y"`),
		}},
	}
	_, err := Compile(rules)
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("expected ErrInvalidAction, got %v", err)
	}
}

func TestCompile_RejectsAppendOnBody(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {{
			Enabled: true,
			Target:  TargetBody,
			Action:  ActionAppend,
			Path:    "x",
			Value:   json.RawMessage(`"y"`),
		}},
	}
	_, err := Compile(rules)
	if !errors.Is(err, ErrAppendOnBody) {
		t.Fatalf("expected ErrAppendOnBody, got %v", err)
	}
}

func TestCompile_RequiresValueForNonRemove(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {{
			Enabled: true,
			Target:  TargetBody,
			Action:  ActionSet,
			Path:    "x",
			Value:   nil,
		}},
	}
	_, err := Compile(rules)
	if !errors.Is(err, ErrValueRequired) {
		t.Fatalf("expected ErrValueRequired, got %v", err)
	}
}

func TestCompile_RemoveIgnoresValue(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {{
			Enabled: true,
			Target:  TargetBody,
			Action:  ActionRemove,
			Path:    "x",
		}},
	}
	snapshot, err := Compile(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	matched := snapshot.Match("anthropic", "claude-3")
	if len(matched) != 1 || matched[0].Action != ActionRemove || matched[0].Value != nil {
		t.Fatalf("unexpected compiled rule: %+v", matched)
	}
}

func TestCompile_InvalidValueJSON(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {{
			Enabled: true,
			Target:  TargetBody,
			Action:  ActionSet,
			Path:    "x",
			Value:   json.RawMessage("{not json"),
		}},
	}
	_, err := Compile(rules)
	if !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("expected ErrInvalidValue, got %v", err)
	}
}

func TestCompile_SkipsDisabledRules(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {
			{Enabled: false, Target: TargetBody, Action: ActionSet, Path: "x", Value: json.RawMessage(`"y"`)},
			{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "z", Value: json.RawMessage(`"w"`)},
		},
	}
	snapshot, err := Compile(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	matched := snapshot.Match("anthropic", "claude-3")
	if len(matched) != 1 || matched[0].Path != "z" {
		t.Fatalf("expected only the enabled rule, got %+v", matched)
	}
}

func TestCompile_GlobCompilation(t *testing.T) {
	cases := []struct {
		glob    string
		model   string
		matches bool
	}{
		{"", "anything", true},
		{"*", "anything", true},
		{"claude-*", "claude-3-opus", true},
		{"claude-*", "gpt-4", false},
		{"gpt-?", "gpt-4", true},
		{"gpt-?", "gpt-40", false},
		{"exact", "exact", true},
		{"exact", "exacts", false},
	}
	for _, tc := range cases {
		rules := map[string][]Rule{
			"anthropic": {{
				Enabled:   true,
				ModelGlob: tc.glob,
				Target:    TargetBody,
				Action:    ActionSet,
				Path:      "x",
				Value:     json.RawMessage(`"y"`),
			}},
		}
		snapshot, err := Compile(rules)
		if err != nil {
			t.Fatalf("compile failed for glob %q: %v", tc.glob, err)
		}
		matched := snapshot.Match("anthropic", tc.model)
		got := len(matched) == 1
		if got != tc.matches {
			t.Fatalf("glob=%q model=%q: want matches=%v got=%v", tc.glob, tc.model, tc.matches, got)
		}
	}
}

func TestCompile_RejectsOverlongGlob(t *testing.T) {
	glob := strings.Repeat("a", MaxModelGlobLength+1)
	rules := map[string][]Rule{
		"anthropic": {{
			Enabled:   true,
			ModelGlob: glob,
			Target:    TargetBody,
			Action:    ActionSet,
			Path:      "x",
			Value:     json.RawMessage(`"y"`),
		}},
	}
	_, err := Compile(rules)
	if !errors.Is(err, ErrGlobTooLong) {
		t.Fatalf("expected ErrGlobTooLong, got %v", err)
	}
}

func TestCompile_RejectsOverlongPath(t *testing.T) {
	path := strings.Repeat("a", MaxPathLength+1)
	rules := map[string][]Rule{
		"anthropic": {{
			Enabled: true,
			Target:  TargetBody,
			Action:  ActionSet,
			Path:    path,
			Value:   json.RawMessage(`"y"`),
		}},
	}
	_, err := Compile(rules)
	if !errors.Is(err, ErrPathTooLong) {
		t.Fatalf("expected ErrPathTooLong, got %v", err)
	}
}

func TestCompile_RejectsTooManyRules(t *testing.T) {
	over := make([]Rule, MaxRulesPerPlatform+1)
	for i := range over {
		over[i] = Rule{
			Enabled: true,
			Target:  TargetBody,
			Action:  ActionSet,
			Path:    "x",
			Value:   json.RawMessage(`"y"`),
		}
	}
	rules := map[string][]Rule{"anthropic": over}
	_, err := Compile(rules)
	if !errors.Is(err, ErrTooManyRules) {
		t.Fatalf("expected ErrTooManyRules, got %v", err)
	}
}

func TestCompile_EmptySnapshotReportsEmpty(t *testing.T) {
	snap, err := Compile(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !snap.IsEmpty() {
		t.Fatalf("expected IsEmpty=true")
	}
	// Nil receiver branch.
	var nilSnap *Compiled
	if !nilSnap.IsEmpty() {
		t.Fatalf("nil *Compiled should report IsEmpty=true")
	}
}

func TestCompile_ValueKindClassification(t *testing.T) {
	cases := []struct {
		value json.RawMessage
		want  valueKind
	}{
		{json.RawMessage(`{"a":1}`), valueKindObject},
		{json.RawMessage(`[1,2]`), valueKindArray},
		{json.RawMessage(`42`), valueKindPrimitive},
		{json.RawMessage(`"hello"`), valueKindPrimitive},
		{json.RawMessage(`true`), valueKindPrimitive},
	}
	for _, tc := range cases {
		rules := map[string][]Rule{
			"anthropic": {{
				Enabled: true,
				Target:  TargetBody,
				Action:  ActionSet,
				Path:    "x",
				Value:   tc.value,
			}},
		}
		snap, err := Compile(rules)
		if err != nil {
			t.Fatalf("compile failed for value %s: %v", string(tc.value), err)
		}
		matched := snap.Match("anthropic", "any")
		if matched[0].valueKind != tc.want {
			t.Fatalf("value=%s: want kind=%d got=%d", string(tc.value), tc.want, matched[0].valueKind)
		}
	}
}

// TestCompile_RejectsNullForNonRemove pins the library contract that null is
// not a legal value for set/merge/append — remove is the only way to delete a
// field. Accepting null silently would persist a literal JSON null into the
// request body, which is never what the user means.
func TestCompile_RejectsNullForNonRemove(t *testing.T) {
	for _, action := range []string{ActionSet, ActionMerge, ActionAppend} {
		target := TargetBody
		if action == ActionAppend {
			target = TargetHeader
		}
		rules := map[string][]Rule{
			"anthropic": {{
				Enabled: true,
				Target:  target,
				Action:  action,
				Path:    "x",
				Value:   json.RawMessage(`null`),
			}},
		}
		_, err := Compile(rules)
		if err == nil {
			t.Fatalf("expected compile to reject null for %s, got nil", action)
		}
		if !errors.Is(err, ErrValueNullForbidden) {
			t.Fatalf("expected ErrValueNullForbidden for %s, got %v", action, err)
		}
	}
}

// TestCompile_AcceptsNullRemoveIsNoopOnValue confirms remove rules never look
// at the value slot — the user can leave it nil, and compile must succeed.
func TestCompile_AcceptsNullRemoveIsNoopOnValue(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {{
			Enabled: true,
			Target:  TargetBody,
			Action:  ActionRemove,
			Path:    "x",
			Value:   json.RawMessage(`null`),
		}},
	}
	if _, err := Compile(rules); err != nil {
		t.Fatalf("expected remove+null to compile cleanly, got %v", err)
	}
}
