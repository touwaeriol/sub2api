//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/paramoverride"
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

func TestApplyParamOverrides_NilGroupIDReturnsBodyUnchanged(t *testing.T) {
	repo := makeStandardRepo(
		paramOverrideTestChannel(ChannelParamOverrides{
			"anthropic": {
				makeParamOverrideRule(paramoverride.TargetBody, paramoverride.ActionSet, "x", json.RawMessage(`1`)),
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"claude-3"}`)
	ctx := context.Background()

	out, outCtx := svc.ApplyParamOverrides(ctx, nil, "anthropic", "claude-3", body)
	if string(out) != string(body) {
		t.Fatalf("expected body unchanged, got %s", string(out))
	}
	if outCtx != ctx {
		t.Fatalf("expected context unchanged when groupID is nil")
	}
	if got := paramoverride.HeadersFromContext(outCtx); got != nil {
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
	out, _ := svc.ApplyParamOverrides(context.Background(), &gid, "anthropic", "claude-3", body)
	if string(out) != string(body) {
		t.Fatalf("expected body unchanged, got %s", string(out))
	}
}

func TestApplyParamOverrides_BodySetApplied(t *testing.T) {
	repo := makeStandardRepo(
		paramOverrideTestChannel(ChannelParamOverrides{
			"anthropic": {
				makeParamOverrideRule(paramoverride.TargetBody, paramoverride.ActionSet,
					"thinking.budget_tokens", json.RawMessage(`2048`)),
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"claude-3","thinking":{"budget_tokens":512}}`)
	gid := int64(1)
	out, _ := svc.ApplyParamOverrides(context.Background(), &gid, "anthropic", "claude-3", body)
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
				makeParamOverrideRule(paramoverride.TargetBody, paramoverride.ActionSet,
					"thinking.budget_tokens", json.RawMessage(`2048`)),
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"gpt-4"}`)
	gid := int64(1)
	out, _ := svc.ApplyParamOverrides(context.Background(), &gid, "openai", "gpt-4", body)
	if string(out) != string(body) {
		t.Fatalf("expected openai body unchanged, got %s", string(out))
	}
}

// TestApplyParamOverrides_HeaderPublishedToContext verifies the core
// contract: ChannelService.ApplyParamOverrides returns a child context
// carrying the override HeaderPayload, which upstream builders later read
// via paramoverride.ApplyContextHeadersToRequest. Framework-agnostic: no
// gin dependency.
func TestApplyParamOverrides_HeaderPublishedToContext(t *testing.T) {
	repo := makeStandardRepo(
		paramOverrideTestChannel(ChannelParamOverrides{
			"anthropic": {
				makeParamOverrideRule(paramoverride.TargetHeader, paramoverride.ActionSet,
					"X-Api-Version", json.RawMessage(`"2024-10"`)),
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"claude-3"}`)
	ctx := context.Background()
	gid := int64(1)
	_, outCtx := svc.ApplyParamOverrides(ctx, &gid, "anthropic", "claude-3", body)

	if outCtx == ctx {
		t.Fatalf("expected a child context carrying the header payload")
	}
	overrides := paramoverride.HeadersFromContext(outCtx)
	if overrides == nil {
		t.Fatalf("expected override headers stored on returned context")
	}
	if got := overrides.Get("X-Api-Version"); got != "2024-10" {
		t.Fatalf("expected X-Api-Version=2024-10, got %q", got)
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
					Target:    paramoverride.TargetBody,
					Action:    paramoverride.ActionSet,
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
	out, _ := svc.ApplyParamOverrides(context.Background(), &gid, "anthropic", "claude-3-opus", []byte(`{}`))
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 4096 {
		t.Fatalf("expected max_tokens=4096 for claude-3-opus, got %d", got)
	}

	// Does not match gpt-*
	out, _ = svc.ApplyParamOverrides(context.Background(), &gid, "anthropic", "gpt-4", []byte(`{}`))
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
			makeParamOverrideRule(paramoverride.TargetBody, paramoverride.ActionSet,
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
	out, _ := svc.ApplyParamOverrides(ctx, &gid, "anthropic", "claude-3", []byte(`{}`))
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
	out, _ = svc.ApplyParamOverrides(ctx, &gid, "anthropic", "claude-3", []byte(`{}`))
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 4096 {
		t.Fatalf("expected budget_tokens=4096 after update, got %d", got)
	}
}
