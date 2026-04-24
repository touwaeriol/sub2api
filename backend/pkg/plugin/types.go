package plugin

import (
	"io"
	"net/http"
	"time"
)

// Account is a simplified, plugin-facing view of an account row. The host
// materialises it from the internal ent model — plugins MUST NOT import ent
// types directly.
//
// Mutations happen through [AccountAPI.PatchExtra] / [AccountAPI.PatchCredentials];
// fields returned here are snapshots.
type Account struct {
	ID          int64
	Name        string
	Platform    string
	Type        string
	Status      string
	Credentials map[string]any
	Extra       map[string]any
	Priority    int
	Concurrency int
	GroupIDs    []int64
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Optional fields populated by the host when available. Plugins should
	// treat zero-values as "unset". Added during Phase 0 so plugins can
	// observe scheduling state without importing internal types.
	Notes          string
	ProxyID        int64
	RateMultiplier *float64
	ExpiresAt      *time.Time
	Schedulable    bool
	ErrorMessage   string
}

// User is a simplified, plugin-facing user snapshot.
type User struct {
	ID        int64
	Email     string
	Role      string
	Status    string
	Balance   string // decimal as string to preserve precision
	Extra     map[string]any
	CreatedAt time.Time
}

// Order is a simplified, plugin-facing payment order snapshot.
type Order struct {
	ID          string
	UserID      int64
	Type        string // "balance", "subscription"
	Amount      string
	Currency    string
	Status      string
	Provider    string
	OutTradeNo  string
	Extra       map[string]any
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// Subscription is a simplified, plugin-facing subscription snapshot.
type Subscription struct {
	ID        int64
	UserID    int64
	PlanID    int64
	Status    string
	ExpiresAt *time.Time
	Extra     map[string]any
}

// ForwardRequest carries the upstream call parameters into
// [GatewayPlugin.Forward] and related hooks.
type ForwardRequest struct {
	// Method is the HTTP method to issue upstream.
	Method string
	// URL is the absolute upstream URL (may be rewritten by plugins).
	URL string
	// Headers is a copy of the caller's headers; plugins may mutate before
	// the call is actually issued.
	Headers http.Header
	// Body is the request body; plugins reading it must preserve it (re-
	// assign after reading).
	Body []byte
	// Model is the model id requested by the client.
	Model string
	// Stream reports whether the caller wants a streamed response.
	Stream bool
	// SessionHash is a stable identifier for the upstream session, used
	// for concurrency accounting.
	SessionHash string
	// Extra carries plugin-specific hints; contents are documented by each
	// gateway plugin.
	Extra map[string]any
}

// ForwardResult is the outcome of a gateway call. Plugins that write their
// own Forward implementation return this; Notify hooks receive it.
type ForwardResult struct {
	// StatusCode is the upstream HTTP status.
	StatusCode int
	// Headers is the response headers as returned (or rewritten).
	Headers http.Header
	// Body is either the full response body (non-streaming) or nil when
	// Stream is populated instead.
	Body []byte
	// Stream, if non-nil, yields the streaming payload; the host pipes it
	// back to the caller and closes it when done.
	Stream io.ReadCloser
	// Usage reports token / image counts for billing.
	Usage *Usage
	// Error is a terminal error if the forward failed after retries.
	Error error
}

// TestConnectionResult is returned by [GatewayPlugin.TestConnection] and
// surfaced in the admin UI.
type TestConnectionResult struct {
	Success bool
	// Model reports the model the probe actually used (may differ from
	// request when plugin chose a default).
	Model string
	// Message is a localized or English description shown to admins.
	Message string
	// LatencyMS is the round-trip duration in milliseconds.
	LatencyMS int64
}

// Usage describes the resources consumed by a single forward, fed to
// [BillingAPI.Record].
type Usage struct {
	InputTokens       int64
	OutputTokens      int64
	CachedInputTokens int64
	ImageInputTokens  int64
	ImageOutputTokens int64
	// Extra is a free-form map for usage dimensions plugins invent (e.g.
	// seconds-of-audio). Billing plugins may price these keys.
	Extra map[string]int64
}

// HealthStatus is returned by [HealthChecker.Health].
type HealthStatus struct {
	// OK reports overall readiness.
	OK bool
	// Message is a one-line summary suitable for admin UIs.
	Message string
	// Details allows plugins to report structured probe results (db latency,
	// queue depth, etc.).
	Details map[string]any
}

// AccountStateChanged is the payload of [TopicAccountStateChanged].
type AccountStateChanged struct {
	AccountID int64
	OldStatus string
	NewStatus string
	Reason    string
}

// OrderEvent is the payload of [TopicOrderPaid] and [TopicOrderFulfilled].
type OrderEvent struct {
	Order *Order
	// DeltaAmount is the positive amount applied by this event (matters
	// for partial refunds / top-ups).
	DeltaAmount string
	// Source is "webhook", "manual", "timer", etc.
	Source string
}

// SettingsChanged is the payload of [TopicPluginSettingsChanged].
type SettingsChanged struct {
	PluginID string
	Key      string
	OldValue any
	NewValue any
}
