//go:build unit

package paramoverride

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func mustCompile(t *testing.T, rules []Rule) []CompiledRule {
	t.Helper()
	snap, err := Compile(map[string][]Rule{"anthropic": rules})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return snap.Match("anthropic", "claude-3")
}

func TestApplyToBodyBytes_SetScalar(t *testing.T) {
	body := []byte(`{"model":"claude-3","max_tokens":1024}`)
	rules := mustCompile(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionSet,
		Path: "max_tokens", Value: json.RawMessage(`2048`),
	}})
	out := ApplyToBodyBytes(body, rules)
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 2048 {
		t.Fatalf("expected 2048, got %d", got)
	}
}

func TestApplyToBodyBytes_SetObject(t *testing.T) {
	body := []byte(`{"model":"claude-3"}`)
	rules := mustCompile(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionSet,
		Path: "thinking", Value: json.RawMessage(`{"budget_tokens":1024,"type":"enabled"}`),
	}})
	out := ApplyToBodyBytes(body, rules)
	thinking := gjson.GetBytes(out, "thinking")
	if !thinking.IsObject() {
		t.Fatalf("expected thinking object, got %s", thinking.Raw)
	}
	if thinking.Get("budget_tokens").Int() != 1024 {
		t.Fatalf("expected budget_tokens=1024, got %s", thinking.Raw)
	}
}

func TestApplyToBodyBytes_MergeObject(t *testing.T) {
	body := []byte(`{"thinking":{"budget_tokens":512,"type":"enabled"}}`)
	rules := mustCompile(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionMerge,
		Path: "thinking", Value: json.RawMessage(`{"budget_tokens":2048}`),
	}})
	out := ApplyToBodyBytes(body, rules)
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 2048 {
		t.Fatalf("expected merged budget_tokens=2048, got %d", got)
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "enabled" {
		t.Fatalf("merge should keep unrelated keys, got type=%q", got)
	}
}

func TestApplyToBodyBytes_MergeNonObjectFallsBackToSet(t *testing.T) {
	body := []byte(`{"thinking":1024}`)
	rules := mustCompile(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionMerge,
		Path: "thinking", Value: json.RawMessage(`{"budget_tokens":2048}`),
	}})
	out := ApplyToBodyBytes(body, rules)
	thinking := gjson.GetBytes(out, "thinking")
	if !thinking.IsObject() || thinking.Get("budget_tokens").Int() != 2048 {
		t.Fatalf("expected object replacement, got %s", thinking.Raw)
	}
}

func TestApplyToBodyBytes_MergeMissingFieldSets(t *testing.T) {
	body := []byte(`{"model":"claude-3"}`)
	rules := mustCompile(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionMerge,
		Path: "thinking", Value: json.RawMessage(`{"budget_tokens":2048}`),
	}})
	out := ApplyToBodyBytes(body, rules)
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 2048 {
		t.Fatalf("expected budget_tokens=2048, got %d", got)
	}
}

func TestApplyToBodyBytes_Remove(t *testing.T) {
	body := []byte(`{"model":"claude-3","thinking":{"budget_tokens":1024}}`)
	rules := mustCompile(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionRemove,
		Path: "thinking",
	}})
	out := ApplyToBodyBytes(body, rules)
	if gjson.GetBytes(out, "thinking").Exists() {
		t.Fatalf("expected thinking removed, got %s", string(out))
	}
}

func TestApplyToBodyBytes_NestedPath(t *testing.T) {
	body := []byte(`{"thinking":{"budget_tokens":512}}`)
	rules := mustCompile(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionSet,
		Path: "thinking.budget_tokens", Value: json.RawMessage(`2048`),
	}})
	out := ApplyToBodyBytes(body, rules)
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 2048 {
		t.Fatalf("expected 2048, got %d", got)
	}
}

func TestApplyToBodyBytes_InvalidJSONBodyPreserved(t *testing.T) {
	body := []byte(`not json`)
	rules := mustCompile(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionSet,
		Path: "x", Value: json.RawMessage(`"y"`),
	}})
	out := ApplyToBodyBytes(body, rules)
	if string(out) != "not json" {
		t.Fatalf("expected body unchanged, got %s", string(out))
	}
}

func TestApplyToBodyBytes_SkipsHeaderRules(t *testing.T) {
	body := []byte(`{"model":"claude-3"}`)
	rules := mustCompile(t, []Rule{
		{Enabled: true, Target: TargetHeader, Action: ActionSet, Path: "X-Test", Value: json.RawMessage(`"abc"`)},
	})
	out := ApplyToBodyBytes(body, rules)
	if string(out) != `{"model":"claude-3"}` {
		t.Fatalf("header rule should not mutate body, got %s", string(out))
	}
}

func TestApplyToBodyBytes_EmptyRuleSlice(t *testing.T) {
	body := []byte(`{"x":1}`)
	out := ApplyToBodyBytes(body, nil)
	if string(out) != `{"x":1}` {
		t.Fatalf("expected body unchanged, got %s", string(out))
	}
}

// TestApplyToBodyBytes_SetNullRejectedAtCompile documents that Set with a
// null payload is rejected at Compile time — users who want to delete a
// field must declare an Action=Remove rule instead. Keeps the applier's
// contract narrow: it never has to decide "is null an intentional write or
// a user mistake?".
func TestApplyToBodyBytes_SetNullRejectedAtCompile(t *testing.T) {
	_, err := Compile(map[string][]Rule{"anthropic": {{
		Enabled: true, Target: TargetBody, Action: ActionSet,
		Path: "thinking", Value: json.RawMessage(`null`),
	}}})
	if err == nil {
		t.Fatalf("expected compile to reject Set+null, got nil")
	}
}

// TestApplyToBodyBytes_LastWriteWins verifies that when multiple rules target
// the same path, the later rule in the input order wins. This pins the
// "rules are applied in declared order, no priority field" contract.
func TestApplyToBodyBytes_LastWriteWins(t *testing.T) {
	body := []byte(`{}`)
	rules := mustCompile(t, []Rule{
		{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "thinking.budget_tokens", Value: json.RawMessage(`1`)},
		{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "thinking.budget_tokens", Value: json.RawMessage(`2`)},
		{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "thinking.budget_tokens", Value: json.RawMessage(`3`)},
	})
	out := ApplyToBodyBytes(body, rules)
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 3 {
		t.Fatalf("expected last rule to win (3), got %d", got)
	}
}

// TestApplyToBodyBytes_OrderedChainSetThenMerge verifies that a Set followed
// by a Merge on the same object path stacks correctly: the Set seeds the
// object and the Merge layers additional fields on top without losing the
// seed. This is the typical "initialize then adjust" pattern.
func TestApplyToBodyBytes_OrderedChainSetThenMerge(t *testing.T) {
	body := []byte(`{}`)
	rules := mustCompile(t, []Rule{
		{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "thinking", Value: json.RawMessage(`{"budget_tokens":1024,"type":"enabled"}`)},
		{Enabled: true, Target: TargetBody, Action: ActionMerge, Path: "thinking", Value: json.RawMessage(`{"budget_tokens":2048}`)},
	})
	out := ApplyToBodyBytes(body, rules)
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 2048 {
		t.Fatalf("expected merged budget_tokens=2048, got %d", got)
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "enabled" {
		t.Fatalf("expected type preserved by merge, got %q", got)
	}
}
