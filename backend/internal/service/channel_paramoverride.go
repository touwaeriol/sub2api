package service

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/paramoverride"
	"github.com/gin-gonic/gin"
)

// ParamOverrideHeadersGinKey is the gin.Context key under which the published
// override headers are mirrored. Defined as a string constant so handlers and
// tests can reference it without importing private types.
const ParamOverrideHeadersGinKey = "paramOverrideHeaders"

// ApplyParamOverrides mutates the outgoing request body according to the
// channel-level override rules associated with the caller's group, and
// publishes any header-targeted overrides through the request context so that
// the upstream-request builders (which rebuild the wire request) can apply
// them past their own header allow-lists.
//
// groupID nil -> the caller is not associated with any channel. The function
// returns the body unchanged and does not touch the context.
//
// The returned slice is either the original body (no mutation) or a fresh
// buffer produced by sjson; callers should replace their reference.
//
// When c is non-nil, header overrides are stored on c.Request.Context() AND
// the gin store, and c.Request is rewritten with the new context so that
// downstream service code calling c.Request.Context() observes them. Passing
// c=nil disables header propagation entirely (body-only mutations still
// apply).
func (s *ChannelService) ApplyParamOverrides(
	ctx context.Context,
	c *gin.Context,
	groupID *int64,
	platform string,
	model string,
	body []byte,
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
	publishHeaderOverrides(c, rules)
	return paramoverride.ApplyToBodyBytes(body, rules)
}

// publishHeaderOverrides applies header rules into a fresh http.Header and
// stores it on both the request context and the gin store so that upstream
// builders (Anthropic / OpenAI / Antigravity) can re-apply them past their
// own header allow-lists.
func publishHeaderOverrides(c *gin.Context, rules []paramoverride.CompiledRule) {
	if c == nil || c.Request == nil {
		return
	}
	headers := http.Header{}
	paramoverride.ApplyToHeaders(headers, rules)
	if len(headers) == 0 {
		return
	}
	c.Set(ParamOverrideHeadersGinKey, headers)
	c.Request = c.Request.WithContext(paramoverride.WithHeaders(c.Request.Context(), headers))
}

// ParamOverrideHeadersFromContext is a thin re-export of
// paramoverride.HeadersFromContext so callers in the service layer can
// inspect the published overrides without importing the low-level package.
func ParamOverrideHeadersFromContext(ctx context.Context) http.Header {
	return paramoverride.HeadersFromContext(ctx)
}

// ApplyParamOverrideHeadersToRequest is a thin re-export of
// paramoverride.ApplyContextHeadersToRequest. It copies the override headers
// stored on ctx onto req.Header, replacing any existing values.
func ApplyParamOverrideHeadersToRequest(ctx context.Context, req *http.Request) {
	if req == nil {
		return
	}
	// Use req's own context if the caller passed it as the source; this
	// matches paramoverride.ApplyContextHeadersToRequest's behaviour.
	if ctx == nil {
		ctx = req.Context()
	}
	overrides := paramoverride.HeadersFromContext(ctx)
	if len(overrides) == 0 {
		return
	}
	for name, values := range overrides {
		req.Header.Del(name)
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
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
