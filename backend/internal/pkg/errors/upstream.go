// Package errors provides application error types and helpers.
// nolint:mnd
package errors

import "net/http"

// UpstreamStatus maps an upstream HTTP status code to the status code this
// service should surface to its own caller, WITHOUT ever collapsing a
// meaningful upstream status into a generic 500.
//
//   - 4xx: preserved as-is (401/403/404/409/429/... stay meaningful so the
//     caller can react: re-authenticate, back off, fix input, ...).
//   - 5xx (and any unexpected status < 400, e.g. a malformed 0): the upstream
//     — not this service — failed, so surface 502 Bad Gateway.
//
// A 2xx status must never be passed here; it is treated defensively as 502.
func UpstreamStatus(upstreamCode int) int {
	if upstreamCode >= http.StatusBadRequest && upstreamCode < http.StatusInternalServerError {
		return upstreamCode
	}
	return http.StatusBadGateway
}

// FromUpstream builds an *ApplicationError carrying the mapped upstream status
// (see UpstreamStatus), so that response.ErrorFrom / ToHTTP / FromError never
// coerce an upstream failure into a 500.
//
// Use this at every upstream-HTTP boundary in place of a bare
// fmt.Errorf("... status %d ...") whose status would otherwise be lost.
func FromUpstream(upstreamCode int, reason, message string) *ApplicationError {
	return New(UpstreamStatus(upstreamCode), reason, message)
}
