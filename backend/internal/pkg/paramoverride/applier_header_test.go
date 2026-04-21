//go:build unit

package paramoverride

import (
	"encoding/json"
	"net/http"
	"testing"
)

func mustCompileHeaders(t *testing.T, rules []Rule) []CompiledRule {
	t.Helper()
	snap, err := Compile(map[string][]Rule{"anthropic": rules})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return snap.Match("anthropic", "claude-3")
}

func TestApplyToHeaders_Set(t *testing.T) {
	h := http.Header{}
	rules := mustCompileHeaders(t, []Rule{{
		Enabled: true, Target: TargetHeader, Action: ActionSet,
		Path: "X-Api-Version", Value: json.RawMessage(`"2024-10"`),
	}})
	ApplyToHeaders(h, rules)
	if got := h.Get("X-Api-Version"); got != "2024-10" {
		t.Fatalf("expected X-Api-Version=2024-10, got %q", got)
	}
}

func TestApplyToHeaders_Append_New(t *testing.T) {
	h := http.Header{}
	rules := mustCompileHeaders(t, []Rule{{
		Enabled: true, Target: TargetHeader, Action: ActionAppend,
		Path: "Anthropic-Beta", Value: json.RawMessage(`"experimental"`),
	}})
	ApplyToHeaders(h, rules)
	if got := h.Get("Anthropic-Beta"); got != "experimental" {
		t.Fatalf("expected experimental, got %q", got)
	}
}

func TestApplyToHeaders_Append_ExistingCommaSeparated(t *testing.T) {
	h := http.Header{}
	h.Set("Anthropic-Beta", "feature-a,feature-b")
	rules := mustCompileHeaders(t, []Rule{{
		Enabled: true, Target: TargetHeader, Action: ActionAppend,
		Path: "Anthropic-Beta", Value: json.RawMessage(`"feature-c"`),
	}})
	ApplyToHeaders(h, rules)
	if got := h.Get("Anthropic-Beta"); got != "feature-a,feature-b,feature-c" {
		t.Fatalf("expected appended list, got %q", got)
	}
}

func TestApplyToHeaders_Append_Dedup(t *testing.T) {
	h := http.Header{}
	h.Set("Anthropic-Beta", "feature-a,feature-b")
	rules := mustCompileHeaders(t, []Rule{{
		Enabled: true, Target: TargetHeader, Action: ActionAppend,
		Path: "Anthropic-Beta", Value: json.RawMessage(`"feature-b"`),
	}})
	ApplyToHeaders(h, rules)
	if got := h.Get("Anthropic-Beta"); got != "feature-a,feature-b" {
		t.Fatalf("expected dedup, got %q", got)
	}
}

// TestApplyToHeaders_Append_DedupIgnoresCase pins the case-insensitive dedup
// contract: HTTP header tokens are case-insensitive, so appending a token
// that differs only in case from an existing one must be a no-op.
func TestApplyToHeaders_Append_DedupIgnoresCase(t *testing.T) {
	h := http.Header{}
	h.Set("Anthropic-Beta", "Feature-A,feature-b")
	rules := mustCompileHeaders(t, []Rule{{
		Enabled: true, Target: TargetHeader, Action: ActionAppend,
		// Different casing vs. "Feature-A" already in the list.
		Path: "Anthropic-Beta", Value: json.RawMessage(`"FEATURE-a"`),
	}})
	ApplyToHeaders(h, rules)
	if got := h.Get("Anthropic-Beta"); got != "Feature-A,feature-b" {
		t.Fatalf("expected case-insensitive dedup, got %q", got)
	}
}

func TestApplyToHeaders_Remove(t *testing.T) {
	h := http.Header{}
	h.Set("X-Extra", "to-be-removed")
	rules := mustCompileHeaders(t, []Rule{{
		Enabled: true, Target: TargetHeader, Action: ActionRemove,
		Path: "X-Extra",
	}})
	ApplyToHeaders(h, rules)
	if got := h.Get("X-Extra"); got != "" {
		t.Fatalf("expected header removed, got %q", got)
	}
}

func TestApplyToHeaders_NonStringValueSkipped(t *testing.T) {
	h := http.Header{}
	h.Set("X-Api-Version", "initial")
	rules := mustCompileHeaders(t, []Rule{{
		Enabled: true, Target: TargetHeader, Action: ActionSet,
		Path: "X-Api-Version", Value: json.RawMessage(`42`),
	}})
	ApplyToHeaders(h, rules)
	if got := h.Get("X-Api-Version"); got != "initial" {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestApplyToHeaders_SkipsBodyRules(t *testing.T) {
	h := http.Header{}
	rules := mustCompileHeaders(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionSet,
		Path: "thinking.budget_tokens", Value: json.RawMessage(`1024`),
	}})
	ApplyToHeaders(h, rules)
	if len(h) != 0 {
		t.Fatalf("expected empty header, got %+v", h)
	}
}

func TestApplyToHeaders_NilHeaderNoop(t *testing.T) {
	rules := mustCompileHeaders(t, []Rule{{
		Enabled: true, Target: TargetHeader, Action: ActionSet,
		Path: "X-Test", Value: json.RawMessage(`"v"`),
	}})
	// Should not panic.
	ApplyToHeaders(nil, rules)
}

func TestApplyToHeaders_EmptyRuleSlice(t *testing.T) {
	h := http.Header{}
	h.Set("X", "keep")
	ApplyToHeaders(h, nil)
	if got := h.Get("X"); got != "keep" {
		t.Fatalf("expected unchanged, got %q", got)
	}
}
