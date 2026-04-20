//go:build unit

package antigravity

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/paramoverride"
)

// TestNewAPIRequestWithURL_AppliesContextHeaderOverrides verifies that the
// Antigravity client's request builder applies header overrides attached to
// ctx via paramoverride.WithHeaders. This is the final safety net because
// the Antigravity upstream header is fully rebuilt (only 3 hard-coded
// headers); without this hook user-configured header overrides for Gemini
// upstream requests would silently disappear.
func TestNewAPIRequestWithURL_AppliesContextHeaderOverrides(t *testing.T) {
	overrides := http.Header{}
	overrides.Set("X-Goog-User-Project", "project-abc")
	overrides.Set("User-Agent", "override-ua")
	ctx := paramoverride.WithHeaders(context.Background(), overrides)

	req, err := NewAPIRequestWithURL(ctx, "https://example.com", "generateContent", "tok", []byte(`{}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if got := req.Header.Get("X-Goog-User-Project"); got != "project-abc" {
		t.Fatalf("expected X-Goog-User-Project override, got %q", got)
	}
	// User-Agent override must win over the hard-coded default.
	if got := req.Header.Get("User-Agent"); got != "override-ua" {
		t.Fatalf("expected User-Agent override to replace hard-coded default, got %q", got)
	}
	// Unrelated default must remain when not overridden.
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("expected Authorization preserved, got %q", got)
	}
}

func TestNewAPIRequestWithURL_NoOverridesLeavesDefaults(t *testing.T) {
	req, err := NewAPIRequestWithURL(context.Background(), "https://example.com", "generateContent", "tok", []byte(`{}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type default, got %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("expected Authorization default, got %q", got)
	}
	if req.Header.Get("User-Agent") == "" {
		t.Fatalf("expected non-empty User-Agent default")
	}
}
