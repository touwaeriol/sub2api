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

func TestApplyToBodyBytes_SetNull(t *testing.T) {
	body := []byte(`{"thinking":{"budget_tokens":1024}}`)
	rules := mustCompile(t, []Rule{{
		Enabled: true, Target: TargetBody, Action: ActionSet,
		Path: "thinking", Value: json.RawMessage(`null`),
	}})
	out := ApplyToBodyBytes(body, rules)
	if gjson.GetBytes(out, "thinking").Type != gjson.Null {
		t.Fatalf("expected null, got %s", gjson.GetBytes(out, "thinking").Raw)
	}
}
