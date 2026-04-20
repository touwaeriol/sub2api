package paramoverride

import (
	"context"
	"net/http"
)

// contextKey is the unexported context key under which override headers are
// stored for downstream upstream-request builders.
type contextKey struct{}

// WithHeaders returns a copy of ctx that carries the given override headers.
// Callers should pass an http.Header populated by ApplyToHeaders. Passing an
// empty/nil header set returns ctx unchanged.
func WithHeaders(ctx context.Context, h http.Header) context.Context {
	if ctx == nil || len(h) == 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, h)
}

// HeadersFromContext returns the override headers stored on ctx, or nil when
// none were attached. Callers must not mutate the returned map (it is shared
// across the request lifecycle).
func HeadersFromContext(ctx context.Context) http.Header {
	if ctx == nil {
		return nil
	}
	h, _ := ctx.Value(contextKey{}).(http.Header)
	return h
}

// ApplyContextHeadersToRequest copies the override headers stored on
// req.Context() onto req.Header, replacing any existing values. This is the
// canonical hook used by upstream-request builders to ensure user-configured
// header overrides reach the upstream regardless of per-platform allow-lists.
//
// Calling with a request that has no override headers attached is a safe
// no-op.
func ApplyContextHeadersToRequest(req *http.Request) {
	if req == nil {
		return
	}
	overrides := HeadersFromContext(req.Context())
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
