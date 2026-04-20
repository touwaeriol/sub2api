//go:build unit

package paramoverride

import (
	"encoding/json"
	"testing"
)

func TestMatch_PlatformIsolation(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "a", Value: json.RawMessage(`1`)}},
		"openai":    {{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "b", Value: json.RawMessage(`2`)}},
	}
	snap, err := Compile(rules)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	anthropic := snap.Match("anthropic", "claude-3")
	if len(anthropic) != 1 || anthropic[0].Path != "a" {
		t.Fatalf("anthropic bucket mismatch: %+v", anthropic)
	}
	openai := snap.Match("openai", "gpt-4")
	if len(openai) != 1 || openai[0].Path != "b" {
		t.Fatalf("openai bucket mismatch: %+v", openai)
	}
	none := snap.Match("gemini", "any")
	if none != nil {
		t.Fatalf("expected nil for unregistered platform, got %+v", none)
	}
}

func TestMatch_PreservesInputOrder(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {
			{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "first", Value: json.RawMessage(`1`)},
			{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "second", Value: json.RawMessage(`2`)},
			{Enabled: true, Target: TargetBody, Action: ActionSet, Path: "third", Value: json.RawMessage(`3`)},
		},
	}
	snap, err := Compile(rules)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	matched := snap.Match("anthropic", "claude-3")
	if len(matched) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matched))
	}
	for i, expected := range []string{"first", "second", "third"} {
		if matched[i].Path != expected {
			t.Fatalf("position %d: want %s got %s", i, expected, matched[i].Path)
		}
	}
}

func TestMatch_ModelFiltering(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {
			{Enabled: true, ModelGlob: "claude-*", Target: TargetBody, Action: ActionSet, Path: "a", Value: json.RawMessage(`1`)},
			{Enabled: true, ModelGlob: "gpt-*", Target: TargetBody, Action: ActionSet, Path: "b", Value: json.RawMessage(`2`)},
		},
	}
	snap, err := Compile(rules)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	only := snap.Match("anthropic", "claude-3-opus")
	if len(only) != 1 || only[0].Path != "a" {
		t.Fatalf("expected only claude rule, got %+v", only)
	}
}

func TestMatch_NoMatchReturnsNil(t *testing.T) {
	rules := map[string][]Rule{
		"anthropic": {
			{Enabled: true, ModelGlob: "gemini-*", Target: TargetBody, Action: ActionSet, Path: "a", Value: json.RawMessage(`1`)},
		},
	}
	snap, err := Compile(rules)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	out := snap.Match("anthropic", "claude-3")
	if out != nil {
		t.Fatalf("expected nil, got %+v", out)
	}
}

func TestMatch_NilReceiverSafe(t *testing.T) {
	var snap *Compiled
	if got := snap.Match("anthropic", "claude-3"); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}
