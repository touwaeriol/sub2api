//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

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
				makeParamOverrideRule(ParamOverrideTargetBody, ParamOverrideActionSet, "x", json.RawMessage(`1`)),
			},
		}),
		map[int64]string{1: "anthropic"},
	)
	svc := newTestChannelService(repo)

	body := []byte(`{"model":"claude-3"}`)
	headers := http.Header{"X-Keep": []string{"original"}}

	out := svc.ApplyParamOverrides(context.Background(), nil, "anthropic", "claude-3", body, headers)
	if string(out) != string(body) {
		t.Fatalf("expected body unchanged, got %s", string(out))
	}
	if headers.Get("X-Keep") != "original" {
		t.Fatalf("expected header unchanged, got %s", headers.Get("X-Keep"))
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
	out := svc.ApplyParamOverrides(context.Background(), &gid, "anthropic", "claude-3", body, nil)
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
	out := svc.ApplyParamOverrides(context.Background(), &gid, "anthropic", "claude-3", body, nil)
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
	out := svc.ApplyParamOverrides(context.Background(), &gid, "openai", "gpt-4", body, nil)
	if string(out) != string(body) {
		t.Fatalf("expected openai body unchanged, got %s", string(out))
	}
}

func TestApplyParamOverrides_HeaderMutation(t *testing.T) {
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
	headers := http.Header{}
	gid := int64(1)
	_ = svc.ApplyParamOverrides(context.Background(), &gid, "anthropic", "claude-3", body, headers)
	if got := headers.Get("X-Api-Version"); got != "2024-10" {
		t.Fatalf("expected X-Api-Version=2024-10, got %q", got)
	}
}

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
	ctx := context.Background()

	// Matches claude-*
	out := svc.ApplyParamOverrides(ctx, &gid, "anthropic", "claude-3-opus", []byte(`{}`), nil)
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 4096 {
		t.Fatalf("expected max_tokens=4096 for claude-3-opus, got %d", got)
	}

	// Does not match gpt-*
	out = svc.ApplyParamOverrides(ctx, &gid, "anthropic", "gpt-4", []byte(`{}`), nil)
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
	out := svc.ApplyParamOverrides(ctx, &gid, "anthropic", "claude-3", []byte(`{}`), nil)
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
	out = svc.ApplyParamOverrides(ctx, &gid, "anthropic", "claude-3", []byte(`{}`), nil)
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 4096 {
		t.Fatalf("expected budget_tokens=4096 after update, got %d", got)
	}
}
