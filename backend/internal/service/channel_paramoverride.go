package service

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/paramoverride"
)

// ApplyParamOverrides mutates the outgoing request body according to the
// channel-level override rules associated with the caller's group, and
// publishes any header-targeted overrides through the returned context so
// that the upstream-request builders (which rebuild the wire request) can
// apply them past their own header allow-lists.
//
// Signature is framework-agnostic on purpose: a *ChannelService is core
// domain, it must not depend on gin. The handler-facing wrappers
// (GatewayService.ApplyParamOverrides, OpenAIGatewayService.ApplyParamOverrides)
// are the place where the returned ctx gets re-attached to the gin.Context's
// *http.Request.
//
// groupID nil -> the caller is not associated with any channel. The function
// returns the input body and input ctx unchanged.
//
// The returned slice is either the original body (no mutation) or a fresh
// buffer produced by sjson; callers should replace their reference.
// The returned ctx is either the input ctx (no headers to publish) or a
// child carrying the HeaderPayload.
func (s *ChannelService) ApplyParamOverrides(
	ctx context.Context,
	groupID *int64,
	platform string,
	model string,
	body []byte,
) ([]byte, context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if groupID == nil {
		return body, ctx
	}
	compiled := s.getCompiledParamOverrides(ctx, *groupID)
	if compiled.IsEmpty() {
		return body, ctx
	}
	rules := compiled.Match(platform, model)
	if len(rules) == 0 {
		return body, ctx
	}
	ctx = publishHeaderOverrides(ctx, rules)
	return paramoverride.ApplyToBodyBytes(body, rules), ctx
}

// publishHeaderOverrides applies header rules into a fresh http.Header and
// returns a child of ctx carrying the resulting payload (headers + append-key
// metadata) so that upstream builders (Anthropic / OpenAI / Antigravity) can
// re-apply them past their own header allow-lists.
//
// The append-key metadata is what lets ApplyContextHeadersToRequest preserve
// Beta-policy / fingerprint defaults when merging — without it, a user
// append rule on anthropic-beta would wipe the "context-1m-2025-08-07"
// token the Beta policy had just set.
//
// Returns ctx unchanged when no header rules fired (zero allocation fast
// path).
func publishHeaderOverrides(ctx context.Context, rules []paramoverride.CompiledRule) context.Context {
	headers := http.Header{}
	appendKeys := paramoverride.ApplyToHeadersWithMetadata(headers, rules)
	if len(headers) == 0 {
		return ctx
	}
	payload := paramoverride.HeaderPayload{Headers: headers, AppendKeys: appendKeys}
	return paramoverride.WithHeaderPayload(ctx, payload)
}

// getCompiledParamOverrides returns the compiled override snapshot for the
// given group, or nil when none is configured. Cache load failures are logged
// and treated as "no overrides" to avoid blocking the request.
//
// The log line includes channel_id when the cache maps the group to a
// channel — matches the context that compileChannelParamOverrides already
// logs, so ops can correlate "cache load failed" with "which channel's
// config is at stake" without grepping groupID → channelID by hand.
func (s *ChannelService) getCompiledParamOverrides(ctx context.Context, groupID int64) *paramoverride.Compiled {
	cache, err := s.loadCache(ctx)
	if err != nil {
		slog.Warn("paramoverride: failed to load channel cache",
			"group_id", groupID,
			"channel_id", s.lookupCachedChannelID(groupID),
			"error", err.Error(),
		)
		return nil
	}
	if cache == nil {
		return nil
	}
	return cache.paramOverridesByGroup[groupID]
}

// lookupCachedChannelID reads the last-known channel ID for a group from
// whatever cache snapshot is currently stored (even a stale one). Returns 0
// when no snapshot is available — callers should treat 0 as "unknown".
// Never returns an error; log enrichment must never block the request path.
func (s *ChannelService) lookupCachedChannelID(groupID int64) int64 {
	cached, ok := s.cache.Load().(*channelCache)
	if !ok || cached == nil {
		return 0
	}
	if ch, ok := cached.channelByGroupID[groupID]; ok && ch != nil {
		return ch.ID
	}
	return 0
}

// compileChannelParamOverrides pre-compiles the override rules for a single
// channel. Returns nil when the channel has no rules or compilation fails
// (failure is logged; the service degrades gracefully to "no overrides").
func compileChannelParamOverrides(ch *Channel) *paramoverride.Compiled {
	if ch == nil || len(ch.ParamOverrides) == 0 {
		return nil
	}
	input := ch.ParamOverrides.ToParamOverrideRules()
	compiled, err := paramoverride.Compile(input)
	if err != nil {
		slog.Warn("paramoverride: failed to compile rules; channel will forward requests unchanged",
			"channel_id", ch.ID,
			"error", err.Error(),
		)
		return nil
	}
	if compiled.IsEmpty() {
		return nil
	}
	return compiled
}
