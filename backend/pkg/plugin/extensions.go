package plugin

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/gin-gonic/gin"
)

// GatewayPlugin registers a plugin as the forwarder for an upstream platform
// (e.g. "anthropic", "antigravity"). The host routes matching requests to
// this plugin's Forward method and surfaces it in the admin UI.
type GatewayPlugin interface {
	// Platform returns the unique platform id owned by this plugin. Must
	// match Account.Platform values the plugin can service.
	Platform() string

	// SupportedModels returns the current list of upstream model IDs. The
	// host caches the result; plugins should cache internally too.
	SupportedModels(ctx context.Context) []string

	// Forward performs the upstream call on behalf of the caller. The gin
	// context is provided so plugins can stream responses directly; the
	// returned ForwardResult is used for telemetry (usage, events) but the
	// body has already been written when streaming.
	Forward(ctx context.Context, c *gin.Context, account *Account, req ForwardRequest) (*ForwardResult, error)

	// TestConnection executes a lightweight probe against the upstream on
	// behalf of the admin. modelID may be empty — plugins then pick a
	// default test model.
	TestConnection(ctx context.Context, account *Account, modelID string) (*TestConnectionResult, error)

	// RefreshCredential refreshes short-lived credentials (e.g. OAuth
	// tokens) on the account in place. Called by the host's scheduler and
	// on-demand from the admin UI. Must persist the updated credentials
	// through AccountAPI.PatchCredentials.
	RefreshCredential(ctx context.Context, account *Account) error
}

// AccountTypePlugin teaches the host how to manage a new account kind beyond
// the built-ins (oauth, apikey, cookie).
type AccountTypePlugin interface {
	// TypeKey returns the stable identifier stored in accounts.type.
	TypeKey() string

	// Validate parses and sanity-checks the credentials an admin submits
	// through the create/update form. The returned map is persisted
	// verbatim (after encryption of sensitive fields by the host).
	Validate(ctx context.Context, creds map[string]any) (map[string]any, error)

	// Refresh periodically refreshes the credentials if needed. Returning
	// (nil, nil) signals "nothing changed"; returning a non-nil map
	// replaces accounts.credentials.
	Refresh(ctx context.Context, account *Account) (map[string]any, error)
}

// RateLimitParser overrides the host's default 429/529 handling for a
// particular upstream. Plugins declare it via Meta.RateLimit; the host calls
// ParseResetAt whenever it sees a rate-limit response.
type RateLimitParser interface {
	// ParseResetAt inspects the upstream response (headers, body, etc.)
	// and returns the wall-clock moment at which the quota resets. A zero
	// time.Time tells the host to fall back to its default heuristic.
	ParseResetAt(ctx context.Context, statusCode int, header map[string][]string, body []byte) time.Time
}

// PaymentProvider is aliased to the internal payment.Provider interface so
// plugins can register payment backends without a second definition.
//
// Deprecated: the alias will move into this SDK once the payment package is
// refactored. Plugins should still refer to [PaymentProvider] — the target
// will follow. The import of internal/payment here is intentional and
// temporary; it lives at the SDK boundary to avoid a one-off wrapper type
// during Phase 0.
type PaymentProvider = payment.Provider
