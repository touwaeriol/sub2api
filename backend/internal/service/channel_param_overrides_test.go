//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/paramoverride"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// makeParamOverrideRule constructs a rule with sensible defaults so tests stay
// focused on the interesting fields.
func makeParamOverrideRule(target, action, path string, value json.RawMessage) ChannelParamOverrideRule {
	return ChannelParamOverrideRule{
		Enabled: true,
		Target:  target,
		Action:  action,
		Path:    path,
		Value:   value,
	}
}

// paramOverrideTestChannel returns a channel with the given overrides bound
// to group 1 (anthropic platform).
func paramOverrideTestChannel(overrides ChannelParamOverrides) Channel {
	return Channel{
		ID:             1,
		Name:           "test-channel",
		Status:         StatusActive,
		GroupIDs:       []int64{1},
		ParamOverrides: overrides,
	}
}

// newGinTestContext returns a fresh gin.Context with an http.Request whose
// context is a freshly derived context.Background. Tests that need to inspect
// header overrides published on the request context use this helper.
func newGinTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest(http.MethodPost, "/test", nil)
	return c
}

func TestApplyParamOverrides_NilGroupIDReturnsBodyUnchanged(t *testing.T) {
	repo := makeStandardRepo(
		paramOverrideTestChannel(ChannelParamOverrides{
			"anthropic": {
				makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet, "x", json.RawMessage(`1`)),
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"claude-3"}`)
	c := newGinTestContext()
	c.Request.Header.Set("X-Keep", "original")

	out := svc.ApplyParamOverrides(c, nil, "anthropic", "claude-3", body)
	if string(out) != string(body) {
		t.Fatalf("expected body unchanged, got %s", string(out))
	}
	if c.Request.Header.Get("X-Keep") != "original" {
		t.Fatalf("expected X-Keep header preserved, got %q", c.Request.Header.Get("X-Keep"))
	}
	if got := paramoverride.HeadersFromContext(c.Request.Context()); got != nil {
		t.Fatalf("expected no override headers published, got %+v", got)
	}
}

func TestApplyParamOverrides_EmptyRulesReturnsBodyUnchanged(t *testing.T) {
	repo := makeStandardRepo(
		paramOverrideTestChannel(nil),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"claude-3"}`)
	gid := int64(1)
	out := svc.ApplyParamOverrides(nil, &gid, "anthropic", "claude-3", body)
	if string(out) != string(body) {
		t.Fatalf("expected body unchanged, got %s", string(out))
	}
}

func TestApplyParamOverrides_BodySetApplied(t *testing.T) {
	repo := makeStandardRepo(
		paramOverrideTestChannel(ChannelParamOverrides{
			"anthropic": {
				makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet,
					"thinking.budget_tokens", json.RawMessage(`2048`)),
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"claude-3","thinking":{"budget_tokens":512}}`)
	gid := int64(1)
	out := svc.ApplyParamOverrides(nil, &gid, "anthropic", "claude-3", body)
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 2048 {
		t.Fatalf("expected budget_tokens=2048, got %d", got)
	}
}

func TestApplyParamOverrides_CrossPlatformIsolation(t *testing.T) {
	// Channel configures a body rule only for anthropic. Calling with
	// platform="openai" (even with the same groupID) must leave the body
	// unchanged.
	repo := makeStandardRepo(
		paramOverrideTestChannel(ChannelParamOverrides{
			"anthropic": {
				makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet,
					"thinking.budget_tokens", json.RawMessage(`2048`)),
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"gpt-4"}`)
	gid := int64(1)
	out := svc.ApplyParamOverrides(nil, &gid, "openai", "gpt-4", body)
	if string(out) != string(body) {
		t.Fatalf("expected openai body unchanged, got %s", string(out))
	}
}

func TestApplyParamOverrides_HeaderPublishedToContext(t *testing.T) {
	repo := makeStandardRepo(
		paramOverrideTestChannel(ChannelParamOverrides{
			"anthropic": {
				makeParamOverrideRule(ParamOverrideTargetHeader, ParamOverrideActionSet,
					"X-Api-Version", json.RawMessage(`"2024-10"`)),
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"claude-3"}`)
	c := newGinTestContext()
	gid := int64(1)
	_ = svc.ApplyParamOverrides(c, &gid, "anthropic", "claude-3", body)

	overrides := paramoverride.HeadersFromContext(c.Request.Context())
	if overrides == nil {
		t.Fatalf("expected override headers stored on request context")
	}
	if got := overrides.Get("X-Api-Version"); got != "2024-10" {
		t.Fatalf("expected X-Api-Version=2024-10, got %q", got)
	}
	// Mirror also lives on the gin store for handlers that prefer it.
	stored, ok := c.Get(ParamOverrideHeadersGinKey)
	if !ok {
		t.Fatalf("expected override headers stored on gin context")
	}
	if _, ok := stored.(http.Header); !ok {
		t.Fatalf("expected stored value to be http.Header, got %T", stored)
	}
}

// Note: tests for the now-removed service-layer wrappers
// ApplyParamOverrideHeadersToRequest / ParamOverrideHeadersFromContext were
// removed in PR-6 Commit C alongside the wrappers. The underlying
// behaviour is covered by the paramoverride package's own tests; see
// backend/internal/pkg/paramoverride/context_test.go.

func TestApplyParamOverrides_ModelGlobFiltering(t *testing.T) {
	repo := makeStandardRepo(
		paramOverrideTestChannel(ChannelParamOverrides{
			"anthropic": {
				{
					Enabled:   true,
					ModelGlob: "claude-*",
					Target:    ParamOverrideTargetBody,
					Action:    ParamOverrideActionSet,
					Path:      "max_tokens",
					Value:     json.RawMessage(`4096`),
				},
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	gid := int64(1)

	// Matches claude-*
	out := svc.ApplyParamOverrides(nil, &gid, "anthropic", "claude-3-opus", []byte(`{}`))
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 4096 {
		t.Fatalf("expected max_tokens=4096 for claude-3-opus, got %d", got)
	}

	// Does not match gpt-*
	out = svc.ApplyParamOverrides(nil, &gid, "anthropic", "gpt-4", []byte(`{}`))
	if gjson.GetBytes(out, "max_tokens").Exists() {
		t.Fatalf("expected gpt-4 body unchanged, got %s", string(out))
	}
}

func TestApplyParamOverrides_CacheInvalidatedAfterUpdate(t *testing.T) {
	// Initial channel has no overrides.
	initial := paramOverrideTestChannel(nil)
	stored := &initial
	updated := paramOverrideTestChannel(ChannelParamOverrides{
		"anthropic": {
			makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet,
				"thinking.budget_tokens", json.RawMessage(`4096`)),
		},
	})

	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{*stored}, nil
		},
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{1: "anthropic"}, nil
		},
		getByIDFn: func(_ context.Context, _ int64) (*Channel, error) {
			return stored, nil
		},
		updateFn: func(_ context.Context, ch *Channel) error {
			stored = ch
			return nil
		},
	}
	svc := newTestChannelService(repo)

	// First call: no overrides -> body unchanged.
	ctx := context.Background()
	gid := int64(1)
	out := svc.ApplyParamOverrides(nil, &gid, "anthropic", "claude-3", []byte(`{}`))
	if gjson.GetBytes(out, "thinking.budget_tokens").Exists() {
		t.Fatalf("expected initial body unchanged, got %s", string(out))
	}

	// Simulate channel update injecting rules.
	stored = &updated
	_, err := svc.Update(ctx, updated.ID, &UpdateChannelInput{})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Second call: overrides now applied.
	out = svc.ApplyParamOverrides(nil, &gid, "anthropic", "claude-3", []byte(`{}`))
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 4096 {
		t.Fatalf("expected budget_tokens=4096 after update, got %d", got)
	}
}
