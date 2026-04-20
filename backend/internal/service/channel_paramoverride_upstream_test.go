//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/paramoverride"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// These tests verify the end-to-end contract of the handler -> service ->
// upstream-request pipeline for parameter overrides. They exercise the public
// ChannelService.ApplyParamOverrides entry point and then the upstream
// request builders (Anthropic / OpenAI / Antigravity) to confirm that both
// the body mutation and the header propagation survive the path that
// production requests take.
//
// The tests bypass the handlers and call the service methods directly,
// because a full handler-level integration test would require mounting the
// entire routing / auth / usage pipeline; the intermediate service contract
// is the narrowest place to pin the behaviour down without introducing
// large mock scaffolding.

// newAnthropicUpstreamTestContext returns a ChannelService wired to a single
// test channel and a gin.Context scaffolded with the incoming request, so
// callers can invoke ApplyParamOverrides exactly as a handler would.
func newAnthropicUpstreamTestContext(t *testing.T, overrides ChannelParamOverrides, groupPlatform string) (*ChannelService, *gin.Context) {
	t.Helper()
	repo := makeStandardRepo(
		paramOverrideTestChannel(overrides),
		map[int64]string{1: groupPlatform},
	)
	svc := newTestChannelService(repo)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(`{"model":"claude-3"}`)))
	c.Request = req
	return svc, c
}

func TestApplyParamOverrides_AnthropicBodyAndHeaderReachUpstream(t *testing.T) {
	overrides := ChannelParamOverrides{
		"anthropic": {
			makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet,
				"thinking.budget_tokens", json.RawMessage(`2048`)),
			makeParamOverrideRule(ParamOverrideTargetHeader, ParamOverrideActionSet,
				"X-Forced", json.RawMessage(`"yes"`)),
		},
	}
	svc, c := newAnthropicUpstreamTestContext(t, overrides, "anthropic")

	body := []byte(`{"model":"claude-3-opus","thinking":{"budget_tokens":512}}`)
	gid := int64(1)
	body = svc.ApplyParamOverrides(c.Request.Context(), c, &gid, "anthropic", "claude-3-opus", body)

	if got := gjson.GetBytes(body, "thinking.budget_tokens").Int(); got != 2048 {
		t.Fatalf("body override not applied, got %s", string(body))
	}

	// Simulate the final step of Anthropic's buildUpstreamRequest: build a
	// fresh request on the mutated context and apply paramoverride headers.
	upstream, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build upstream request: %v", err)
	}
	ApplyParamOverrideHeadersToRequest(c.Request.Context(), upstream)

	if got := upstream.Header.Get("X-Forced"); got != "yes" {
		t.Fatalf("expected X-Forced=yes on upstream request, got %q", got)
	}
}

func TestApplyParamOverrides_OpenAIBodyAndHeaderReachUpstream(t *testing.T) {
	overrides := ChannelParamOverrides{
		"openai": {
			makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet,
				"reasoning.effort", json.RawMessage(`"high"`)),
			makeParamOverrideRule(ParamOverrideTargetHeader, ParamOverrideActionAppend,
				"OpenAI-Beta", json.RawMessage(`"responses=experimental"`)),
		},
	}
	svc, c := newAnthropicUpstreamTestContext(t, overrides, "openai")

	body := []byte(`{"model":"gpt-5","reasoning":{"effort":"medium"}}`)
	gid := int64(1)
	body = svc.ApplyParamOverrides(c.Request.Context(), c, &gid, "openai", "gpt-5", body)

	if got := gjson.GetBytes(body, "reasoning.effort").String(); got != "high" {
		t.Fatalf("expected reasoning.effort=high, got %q", got)
	}

	upstream, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
	upstream.Header.Set("OpenAI-Beta", "existing-feature")
	ApplyParamOverrideHeadersToRequest(c.Request.Context(), upstream)

	if got := upstream.Header.Get("OpenAI-Beta"); got != "responses=experimental" {
		// ApplyToHeaders Append + ApplyContextHeadersToRequest overwrite:
		// the published header map already contains the appended value
		// under the header name, so the upstream request ends with the
		// published value (overwrites "existing-feature"). This is the
		// intended behaviour: overrides are final.
		t.Fatalf("expected override OpenAI-Beta on upstream request, got %q", got)
	}
}

func TestApplyParamOverrides_AntigravityHeaderReachesUpstream(t *testing.T) {
	// Antigravity's client.go rebuilds the upstream header from scratch,
	// so this test verifies paramoverride.ApplyContextHeadersToRequest
	// (which is called at the end of NewAPIRequestWithURL) correctly
	// carries the override through the context value.
	overrides := ChannelParamOverrides{
		"antigravity": {
			makeParamOverrideRule(ParamOverrideTargetHeader, ParamOverrideActionSet,
				"X-Goog-User-Project", json.RawMessage(`"project-abc"`)),
		},
	}
	svc, c := newAnthropicUpstreamTestContext(t, overrides, "antigravity")

	body := []byte(`{"model":"gemini-2.5"}`)
	gid := int64(1)
	_ = svc.ApplyParamOverrides(c.Request.Context(), c, &gid, "antigravity", "gemini-2.5", body)

	// Simulate Antigravity's NewAPIRequestWithURL: fresh request with only
	// the hard-coded Content-Type / Authorization / User-Agent, then the
	// context-header hook.
	upstream, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "https://example.com/v1/x", bytes.NewReader(body))
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Authorization", "Bearer token")
	upstream.Header.Set("User-Agent", "test-ua")
	paramoverride.ApplyContextHeadersToRequest(upstream)

	if got := upstream.Header.Get("X-Goog-User-Project"); got != "project-abc" {
		t.Fatalf("expected Antigravity upstream to receive override header, got %q", got)
	}
	// Existing hard-coded headers must remain.
	if got := upstream.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type preserved, got %q", got)
	}
}

func TestApplyParamOverrides_NoGroupIDSkipsEverything(t *testing.T) {
	overrides := ChannelParamOverrides{
		"anthropic": {
			makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet,
				"x", json.RawMessage(`1`)),
			makeParamOverrideRule(ParamOverrideTargetHeader, ParamOverrideActionSet,
				"X-Forced", json.RawMessage(`"yes"`)),
		},
	}
	svc, c := newAnthropicUpstreamTestContext(t, overrides, "anthropic")

	body := []byte(`{"model":"claude-3"}`)
	body = svc.ApplyParamOverrides(c.Request.Context(), c, nil, "anthropic", "claude-3", body)

	if string(body) != `{"model":"claude-3"}` {
		t.Fatalf("expected body unchanged, got %s", string(body))
	}
	if h := ParamOverrideHeadersFromContext(c.Request.Context()); h != nil {
		t.Fatalf("expected no published headers, got %+v", h)
	}

	upstream, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "https://example.com/x", bytes.NewReader(body))
	ApplyParamOverrideHeadersToRequest(c.Request.Context(), upstream)
	if upstream.Header.Get("X-Forced") != "" {
		t.Fatalf("expected no override header propagated, got %q", upstream.Header.Get("X-Forced"))
	}
}

func TestApplyParamOverrides_ContextPropagatesThroughRequestWithContext(t *testing.T) {
	// Verifies that c.Request.WithContext inside publishHeaderOverrides
	// correctly replaces c.Request so subsequent reads of c.Request.Context()
	// pick up the override headers. Matches how buildUpstreamRequest reads
	// ctx inside the service layer.
	overrides := ChannelParamOverrides{
		"anthropic": {
			makeParamOverrideRule(ParamOverrideTargetHeader, ParamOverrideActionSet,
				"X-Version", json.RawMessage(`"v1"`)),
		},
	}
	svc, c := newAnthropicUpstreamTestContext(t, overrides, "anthropic")

	originalCtx := c.Request.Context()
	gid := int64(1)
	_ = svc.ApplyParamOverrides(originalCtx, c, &gid, "anthropic", "claude-3", []byte(`{}`))

	// After the call, c.Request.Context() should carry the override map.
	newCtx := c.Request.Context()
	if newCtx == originalCtx {
		t.Fatalf("expected c.Request to be replaced with a derived context")
	}
	overridesFromCtx := ParamOverrideHeadersFromContext(newCtx)
	if overridesFromCtx == nil || overridesFromCtx.Get("X-Version") != "v1" {
		t.Fatalf("expected override headers accessible via c.Request.Context(), got %+v", overridesFromCtx)
	}
}

// sanity: context.Background should never carry override headers accidentally
// (guards against cross-test pollution via package-level state).
func TestApplyParamOverrides_NoGlobalContextLeak(t *testing.T) {
	if h := paramoverride.HeadersFromContext(context.Background()); h != nil {
		t.Fatalf("expected clean global context, got %+v", h)
	}
}

// TestOpenAIBuildUpstreamRequest_AppliesContextHeaderOverrides verifies the
// real OpenAI buildUpstreamRequest applies headers stored on ctx past the
// openaiAllowedHeaders allow-list. This mirrors the production path where
// ApplyParamOverrides publishes the map before Forward is called.
func TestOpenAIBuildUpstreamRequest_AppliesContextHeaderOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	overrides := http.Header{}
	overrides.Set("X-Custom-Override", "yes")
	ctx := paramoverride.WithHeaders(c.Request.Context(), overrides)

	svc := &OpenAIGatewayService{}
	account := &Account{
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}

	req, err := svc.buildUpstreamRequest(ctx, c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", false)
	if err != nil {
		t.Fatalf("buildUpstreamRequest: %v", err)
	}
	if got := req.Header.Get("X-Custom-Override"); got != "yes" {
		t.Fatalf("expected override header to reach upstream, got %q", got)
	}
}
