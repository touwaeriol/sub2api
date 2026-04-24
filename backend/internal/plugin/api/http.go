package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// httpUpstream adapts service.HTTPUpstream to the plugin contract. The
// host HTTP client wants (proxyURL, accountID, concurrency) tuning knobs
// that plugins do not own; Phase 0 passes zero values so outbound traffic
// goes through the default transport without per-account accounting.
type httpUpstream struct {
	guard    *guard
	upstream service.HTTPUpstream
}

// newHTTPUpstream returns the wrapper or an ErrNotImplemented stub when the
// host has no HTTPUpstream wired.
func newHTTPUpstream(c *coreAPIImpl) plugin.HTTPUpstream {
	if c.deps.HTTPUpstream == nil {
		return unimplementedHTTPUpstream{}
	}
	return &httpUpstream{guard: c.guard, upstream: c.deps.HTTPUpstream}
}

// Do forwards req through the host HTTP client. Requires PermHTTPOutbound.
// The request carries its own context; req.WithContext(ctx) lets the
// caller enforce per-call deadlines without mutating the input.
func (h *httpUpstream) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := h.guard.requirePerm(plugin.PermHTTPOutbound); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("plugin http: request is nil")
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	resp, err := h.upstream.Do(req, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("plugin http do: %w", err)
	}
	return resp, nil
}

// unimplementedHTTPUpstream stands in when the host booted without an
// HTTPUpstream implementation.
type unimplementedHTTPUpstream struct{}

func (unimplementedHTTPUpstream) Do(context.Context, *http.Request) (*http.Response, error) {
	return nil, plugin.ErrNotImplemented
}
