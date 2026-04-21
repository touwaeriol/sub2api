//go:build unit

package paramoverride

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestWithHeaders_RoundTrip(t *testing.T) {
	h := http.Header{}
	h.Set("X-Test", "value")
	ctx := WithHeaders(context.Background(), h)
	got := HeadersFromContext(ctx)
	if got == nil {
		t.Fatalf("expected non-nil headers from context")
	}
	if got.Get("X-Test") != "value" {
		t.Fatalf("expected X-Test=value, got %q", got.Get("X-Test"))
	}
}

func TestWithHeaders_EmptyReturnsContextUnchanged(t *testing.T) {
	ctx := context.Background()
	got := WithHeaders(ctx, http.Header{})
	if got != ctx {
		t.Fatalf("expected context unchanged for empty headers")
	}
	got = WithHeaders(ctx, nil)
	if got != ctx {
		t.Fatalf("expected context unchanged for nil headers")
	}
}

func TestWithHeaders_NilContext(t *testing.T) {
	h := http.Header{}
	h.Set("X-Test", "value")
	if WithHeaders(nil, h) != nil { //nolint:staticcheck // intentional nil ctx test
		t.Fatalf("expected nil ctx returned unchanged")
	}
}

func TestHeadersFromContext_AbsentReturnsNil(t *testing.T) {
	if HeadersFromContext(context.Background()) != nil {
		t.Fatalf("expected nil for context without headers")
	}
	if HeadersFromContext(nil) != nil {
		t.Fatalf("expected nil for nil context")
	}
}

func TestApplyContextHeadersToRequest_OverridesExisting(t *testing.T) {
	overrides := http.Header{}
	overrides.Set("X-Api-Version", "2024-10")
	overrides.Set("Anthropic-Beta", "feature-a,feature-b")

	ctx := WithHeaders(context.Background(), overrides)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/upstream", nil)
	req.Header.Set("X-Api-Version", "previous")
	req.Header.Set("Other", "untouched")

	ApplyContextHeadersToRequest(req)

	if got := req.Header.Get("X-Api-Version"); got != "2024-10" {
		t.Fatalf("expected X-Api-Version overwritten, got %q", got)
	}
	if got := req.Header.Get("Anthropic-Beta"); got != "feature-a,feature-b" {
		t.Fatalf("expected Anthropic-Beta set, got %q", got)
	}
	if got := req.Header.Get("Other"); got != "untouched" {
		t.Fatalf("expected unrelated header preserved, got %q", got)
	}
}

func TestApplyContextHeadersToRequest_NoOpWithoutOverrides(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "/upstream", nil)
	req.Header.Set("X", "keep")
	ApplyContextHeadersToRequest(req)
	if got := req.Header.Get("X"); got != "keep" {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestApplyContextHeadersToRequest_NilRequest(t *testing.T) {
	// Should not panic.
	ApplyContextHeadersToRequest(nil)
}

// TestApplyContextHeadersToRequest_AppendPreservesExistingValues pins the
// PR-7 contract: when the override headers were built from an ActionAppend
// rule, applying them to a request that already has values for the same key
// must merge (not overwrite) so Beta-policy / fingerprint defaults survive.
//
// Production scenario this guards:
//  1. buildUpstreamRequest sets Anthropic-Beta: "context-1m-2025-08-07"
//  2. User configures paramoverride append Anthropic-Beta =
//     "interleaved-thinking-2025-05-14"
//  3. ApplyContextHeadersToRequest must produce both tokens, not just the
//     appended one.
func TestApplyContextHeadersToRequest_AppendPreservesExistingValues(t *testing.T) {
	overrides := http.Header{}
	overrides.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14")

	ctx := WithHeaderPayload(context.Background(), HeaderPayload{
		Headers: overrides,
		AppendKeys: map[string]struct{}{
			"Anthropic-Beta": {},
		},
	})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/upstream", nil)
	// Simulate Beta-policy having written its default token first.
	req.Header.Set("Anthropic-Beta", "context-1m-2025-08-07")

	ApplyContextHeadersToRequest(req)

	got := req.Header.Get("Anthropic-Beta")
	// Both tokens must be present; order is "existing first, appended after".
	if !strings.Contains(got, "context-1m-2025-08-07") {
		t.Fatalf("expected existing Beta-policy token preserved, got %q", got)
	}
	if !strings.Contains(got, "interleaved-thinking-2025-05-14") {
		t.Fatalf("expected appended token present, got %q", got)
	}
}

// TestApplyContextHeadersToRequest_AppendDedupIgnoresCase verifies that the
// merge is idempotent / case-insensitive — re-applying the same payload (or
// appending a token that only differs in case) is a no-op.
func TestApplyContextHeadersToRequest_AppendDedupIgnoresCase(t *testing.T) {
	overrides := http.Header{}
	overrides.Set("Anthropic-Beta", "FEATURE-X")
	ctx := WithHeaderPayload(context.Background(), HeaderPayload{
		Headers:    overrides,
		AppendKeys: map[string]struct{}{"Anthropic-Beta": {}},
	})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/upstream", nil)
	req.Header.Set("Anthropic-Beta", "feature-x,context-1m-2025-08-07")

	ApplyContextHeadersToRequest(req)

	got := req.Header.Get("Anthropic-Beta")
	// "FEATURE-X" is a case-insensitive dup of the existing "feature-x" and
	// must be dropped; the existing list must be preserved verbatim.
	if got != "feature-x,context-1m-2025-08-07" {
		t.Fatalf("expected case-insensitive dedup, got %q", got)
	}
}

// TestApplyContextHeadersToRequest_SetKeysStillOverwrite ensures the
// backwards-compatible Set path is unchanged — keys NOT in AppendKeys must
// still replace whatever was on req.Header.
func TestApplyContextHeadersToRequest_SetKeysStillOverwrite(t *testing.T) {
	overrides := http.Header{}
	overrides.Set("X-Api-Version", "2024-10")
	ctx := WithHeaderPayload(context.Background(), HeaderPayload{
		Headers:    overrides,
		AppendKeys: nil, // empty — Set semantics
	})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/upstream", nil)
	req.Header.Set("X-Api-Version", "previous")

	ApplyContextHeadersToRequest(req)

	if got := req.Header.Get("X-Api-Version"); got != "2024-10" {
		t.Fatalf("expected Set override to replace existing, got %q", got)
	}
}
