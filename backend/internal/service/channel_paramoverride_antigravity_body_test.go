//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/tidwall/gjson"
)

// TestApplyParamOverrides_AntigravityThinkingBudgetPropagatesToGemini
// verifies the full round-trip that PR-4's BODY_PATH_PRESETS.antigravity
// promises to users:
//
//  1. client sends a Claude-format request body (action=set,
//     path="thinking.budget_tokens", value=5000);
//  2. ChannelService.ApplyParamOverrides rewrites the body via sjson;
//  3. AntigravityGatewayService.Forward's production pipeline then
//     json.Unmarshal's into ClaudeRequest and calls
//     TransformClaudeToGeminiWithOptions;
//  4. the Gemini upstream request carries
//     generationConfig.thinkingConfig.thinkingBudget = 5000.
//
// If step 3 ever rearranges or drops the thinking override the test will
// break, pointing us back at the preset list so we can update the
// docstring / i18n hint.
func TestApplyParamOverrides_AntigravityThinkingBudgetPropagatesToGemini(t *testing.T) {
	overrides := ChannelParamOverrides{
		"antigravity": {
			makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet,
				"thinking.budget_tokens", json.RawMessage(`5000`)),
		},
	}
	svc, c := newAnthropicUpstreamTestContext(t, overrides, "antigravity")

	// Claude body: thinking enabled but with a smaller budget that must be
	// overridden by the param rule. max_tokens is set above the new budget
	// so ensureMaxTokensGreaterThanBudget doesn't also fire.
	body := []byte(`{
		"model": "claude-opus-4-6",
		"max_tokens": 32000,
		"messages": [{"role": "user", "content": "hi"}],
		"thinking": {"type": "enabled", "budget_tokens": 512}
	}`)
	gid := int64(1)
	mutated := svc.ApplyParamOverrides(context.Background(), c, &gid, "antigravity", "claude-opus-4-6", body)

	// Sanity check the sjson-level result before we feed it through the
	// Claude→Gemini transformer.
	if got := gjson.GetBytes(mutated, "thinking.budget_tokens").Int(); got != 5000 {
		t.Fatalf("expected body.thinking.budget_tokens=5000 after override, got %d (body=%s)", got, string(mutated))
	}
	if got := gjson.GetBytes(mutated, "thinking.type").String(); got != "enabled" {
		t.Fatalf("expected body.thinking.type preserved, got %q", got)
	}

	// Simulate the production Antigravity dispatch: unmarshal into
	// ClaudeRequest and run the transformer (same two lines the real
	// Forward path uses).
	var claudeReq antigravity.ClaudeRequest
	if err := json.Unmarshal(mutated, &claudeReq); err != nil {
		t.Fatalf("unmarshal Claude request after override: %v", err)
	}
	if claudeReq.Thinking == nil {
		t.Fatalf("expected claudeReq.Thinking != nil after override")
	}
	if claudeReq.Thinking.BudgetTokens != 5000 {
		t.Fatalf("expected claudeReq.Thinking.BudgetTokens=5000, got %d", claudeReq.Thinking.BudgetTokens)
	}

	geminiBody, err := antigravity.TransformClaudeToGeminiWithOptions(&claudeReq, "test-project", "claude-opus-4-6", antigravity.DefaultTransformOptions())
	if err != nil {
		t.Fatalf("TransformClaudeToGeminiWithOptions: %v", err)
	}

	// Final assertion: the Gemini request's thinking budget mirrors the
	// overridden value.
	got := gjson.GetBytes(geminiBody, "request.generationConfig.thinkingConfig.thinkingBudget").Int()
	if got != 5000 {
		t.Fatalf("expected Gemini thinkingBudget=5000 end-to-end, got %d (body=%s)", got, string(geminiBody))
	}
	// includeThoughts must also remain on so the user actually sees the
	// reasoning with the bigger budget.
	if !gjson.GetBytes(geminiBody, "request.generationConfig.thinkingConfig.includeThoughts").Bool() {
		t.Fatalf("expected includeThoughts=true in Gemini request, got %s", string(geminiBody))
	}
}

// TestApplyParamOverrides_AntigravityThinkingTypeDisabledStripsThinkingConfig
// covers the other half of the BODY_PATH_PRESETS.antigravity preset:
// overriding thinking.type to "disabled" should cause the Gemini
// generationConfig to omit thinkingConfig entirely, because
// buildGenerationConfig only emits it for "enabled" / "adaptive".
func TestApplyParamOverrides_AntigravityThinkingTypeDisabledStripsThinkingConfig(t *testing.T) {
	overrides := ChannelParamOverrides{
		"antigravity": {
			makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet,
				"thinking.type", json.RawMessage(`"disabled"`)),
		},
	}
	svc, c := newAnthropicUpstreamTestContext(t, overrides, "antigravity")

	body := []byte(`{
		"model": "claude-opus-4-6",
		"max_tokens": 8192,
		"messages": [{"role": "user", "content": "hi"}],
		"thinking": {"type": "enabled", "budget_tokens": 1024}
	}`)
	gid := int64(1)
	mutated := svc.ApplyParamOverrides(context.Background(), c, &gid, "antigravity", "claude-opus-4-6", body)

	if got := gjson.GetBytes(mutated, "thinking.type").String(); got != "disabled" {
		t.Fatalf("expected body.thinking.type=disabled, got %q", got)
	}

	var claudeReq antigravity.ClaudeRequest
	if err := json.Unmarshal(mutated, &claudeReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	geminiBody, err := antigravity.TransformClaudeToGeminiWithOptions(&claudeReq, "test-project", "claude-opus-4-6", antigravity.DefaultTransformOptions())
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	if gjson.GetBytes(geminiBody, "request.generationConfig.thinkingConfig").Exists() {
		t.Fatalf("expected thinkingConfig stripped when thinking.type=disabled, got %s", string(geminiBody))
	}
}
