package service

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/paramoverride"
)

// ApplyParamOverrides mutates the outgoing request body and headers according
// to the channel-level override rules associated with the caller's group.
//
// groupID nil -> the caller is not associated with any channel (or does not
// belong to a group at all). The function returns the body unchanged and does
// not touch the headers.
//
// The returned slice is either the original body (no mutation) or a fresh
// buffer produced by sjson; callers should replace their reference.
//
// Headers are mutated in place. Passing a nil headers argument is safe and
// treated as "header overrides not applicable" (no-op).
func (s *ChannelService) ApplyParamOverrides(
	ctx context.Context,
	groupID *int64,
	platform string,
	model string,
	body []byte,
	headers http.Header,
) []byte {
	if groupID == nil {
		return body
	}
	compiled := s.getCompiledParamOverrides(ctx, *groupID)
	if compiled.IsEmpty() {
		return body
	}
	rules := compiled.Match(platform, model)
	if len(rules) == 0 {
		return body
	}
	if headers != nil {
		paramoverride.ApplyToHeaders(headers, rules)
	}
	return paramoverride.ApplyToBodyBytes(body, rules)
}

// getCompiledParamOverrides returns the compiled override snapshot for the
// given group, or nil when none is configured. Cache load failures are logged
// and treated as "no overrides" to avoid blocking the request.
func (s *ChannelService) getCompiledParamOverrides(ctx context.Context, groupID int64) *paramoverride.Compiled {
	cache, err := s.loadCache(ctx)
	if err != nil {
		slog.Warn("paramoverride: failed to load channel cache",
			"group_id", groupID,
			"error", err.Error(),
		)
		return nil
	}
	if cache == nil {
		return nil
	}
	return cache.paramOverridesByGroup[groupID]
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
