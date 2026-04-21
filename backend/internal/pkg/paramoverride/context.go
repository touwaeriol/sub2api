package paramoverride

import (
	"context"
	"net/http"
	"strings"
)

// contextKey is the unexported context key under which override headers are
// stored for downstream upstream-request builders.
type contextKey struct{}

// HeaderPayload carries the override headers published by
// ApplyParamOverrides. headers holds the canonicalised (name → values) map
// produced by ApplyToHeaders on a fresh http.Header; appendKeys marks which
// of those names came from ActionAppend rules. Upstream builders use the
// payload via ApplyContextHeadersToRequest:
//   - Set / Remove-derived entries: overwrite whatever the builder already set
//     for that key (pre-PR-7 behaviour, preserved).
//   - Append-derived entries: merge with whatever the builder already set,
//     case-insensitively de-duping tokens within the existing values.
//
// The struct is intentionally concrete (not an interface) so callers never
// have to reach through a nil pointer — HeadersFromContext still returns
// plain http.Header for back-compat.
type HeaderPayload struct {
	Headers    http.Header
	AppendKeys map[string]struct{}
}

// WithHeaders returns a copy of ctx carrying the given override headers as a
// payload where every name is treated as Set (overwrite-on-apply). Exists
// for callers that only need the simple overwrite semantics — notably unit
// tests that construct headers by hand. Passing an empty/nil header set
// returns ctx unchanged.
func WithHeaders(ctx context.Context, h http.Header) context.Context {
	if ctx == nil || len(h) == 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, HeaderPayload{Headers: h})
}

// WithHeaderPayload is the richer counterpart of WithHeaders that also
// records which canonical header names were built from Append rules, so
// ApplyContextHeadersToRequest can merge them with whatever the upstream
// builder already set rather than overwriting.
func WithHeaderPayload(ctx context.Context, payload HeaderPayload) context.Context {
	if ctx == nil || len(payload.Headers) == 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, payload)
}

// HeadersFromContext returns the override headers stored on ctx, or nil when
// none were attached. Callers must not mutate the returned map (it is shared
// across the request lifecycle).
func HeadersFromContext(ctx context.Context) http.Header {
	payload, ok := headerPayloadFromContext(ctx)
	if !ok {
		return nil
	}
	return payload.Headers
}

// headerPayloadFromContext returns the full payload. Unexported because the
// struct is an internal transport format; external readers should stick to
// HeadersFromContext when they only need the headers.
func headerPayloadFromContext(ctx context.Context) (HeaderPayload, bool) {
	if ctx == nil {
		return HeaderPayload{}, false
	}
	payload, ok := ctx.Value(contextKey{}).(HeaderPayload)
	return payload, ok
}

// ApplyContextHeadersToRequest merges the override headers stored on
// req.Context() onto req.Header. Per-key behaviour:
//
//   - Keys marked as append (built from ActionAppend rules): the override
//     values are appended to whatever the builder already set on req.Header,
//     case-insensitively de-duping tokens in the existing value(s) so
//     repeated apply cycles stay idempotent. This is critical for
//     comma-separated tokens lists like anthropic-beta / openai-beta where
//     a user-configured append should extend (not replace) the builder's
//     defaults.
//
//   - Keys NOT marked as append (ActionSet / ActionRemove-derived): overwrite
//     any existing values with the override values. This preserves the
//     pre-PR-7 "overrides always win" contract for explicit Set rules.
//
// Calling with a request that has no override headers attached is a safe
// no-op.
func ApplyContextHeadersToRequest(req *http.Request) {
	if req == nil {
		return
	}
	payload, ok := headerPayloadFromContext(req.Context())
	if !ok || len(payload.Headers) == 0 {
		return
	}
	for name, values := range payload.Headers {
		if _, isAppend := payload.AppendKeys[name]; isAppend {
			mergeHeaderValues(req.Header, name, values)
			continue
		}
		req.Header.Del(name)
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
}

// mergeHeaderValues appends values to req.Header[name], splitting on commas
// and case-insensitively de-duping tokens across the existing and new
// values. The existing values are kept in their original order; new tokens
// are added in the order they appear in values.
func mergeHeaderValues(h http.Header, name string, values []string) {
	existing := h.Get(name)
	for _, v := range values {
		for _, token := range strings.Split(v, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if headerTokenContainsFold(existing, token) {
				continue
			}
			if existing == "" {
				existing = token
			} else {
				existing = existing + "," + token
			}
		}
	}
	h.Set(name, existing)
}

// headerTokenContainsFold reports whether any comma-separated token in
// joined matches token case-insensitively.
func headerTokenContainsFold(joined, token string) bool {
	if joined == "" {
		return false
	}
	for _, part := range strings.Split(joined, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}
