//go:build unit

package paramoverride

import (
	"context"
	"net/http"
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
