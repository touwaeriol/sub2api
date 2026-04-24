package plugin

import (
	"context"
	"net/http"
	"time"

	"entgo.io/ent/dialect"
)

// CoreAPI is the aggregate surface the host exposes to a plugin. The host
// produces one implementation per plugin, scoped to that plugin's id, with
// every child API enforcing Meta.Permissions via the permission guard.
//
// The getter pattern (instead of embedding) lets the host lazily construct
// sub-APIs and return ErrPermissionDenied wrappers when the plugin lacks
// permission for the corresponding capability.
type CoreAPI interface {
	// PluginID returns the id of the plugin this CoreAPI was created for.
	PluginID() string
	// Logger returns a slog-style logger tagged with the plugin id.
	Logger() Logger

	Accounts() AccountAPI
	Users() UserAPI
	Orders() OrderAPI
	Subscriptions() SubscriptionAPI
	Billing() BillingAPI
	RateLimit() RateLimitAPI
	Concurrency() ConcurrencyAPI
	Scheduler() SchedulerAPI

	Cache() CacheStore
	HTTP() HTTPUpstream
	Jobs() JobQueue
	I18n() I18n
	Crypto() Crypto

	Settings() NamespacedSettings
	KV() PluginKV
	EventsLog() EventsLog
	Events() EventBus

	// Plugins yields a minimal lookup into peer plugins for cross-plugin
	// export retrieval; use [PluginAs] rather than calling this directly.
	Plugins() PluginRegistry

	// EntDriver returns the shared ent dialect driver used by the host.
	// Plugins that bring their own ent client construct it via
	// gen.NewClient(gen.Driver(core.EntDriver())) inside Init.
	//
	// Returns nil during early Phase 0 boot when the host has not wired a
	// driver yet; plugins should treat that as "ent not available" and
	// fall back gracefully (e.g. record a log line and disable features
	// that need it).
	EntDriver() dialect.Driver
}

// Logger is a minimal logging contract. Implementations wrap slog.
type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// PatchFunc describes a pure mutation over a map. Implementations MUST return
// a new map (or the same map after mutation) and MUST NOT retain a reference
// after the call returns.
type PatchFunc func(current map[string]any) map[string]any

// AccountAPI exposes account queries and safe mutations. Mutations are scoped
// to Extra and Credentials; other fields are managed by the host.
type AccountAPI interface {
	Find(ctx context.Context, id int64) (*Account, error)
	List(ctx context.Context, filter AccountFilter) ([]*Account, error)
	PatchExtra(ctx context.Context, id int64, patch PatchFunc) error
	PatchCredentials(ctx context.Context, id int64, patch PatchFunc) error
}

// AccountFilter is the filter shape accepted by [AccountAPI.List]. Zero
// values mean "no constraint".
type AccountFilter struct {
	Platform string
	Type     string
	Status   string
	Search   string
	GroupID  int64
	Limit    int
	Offset   int
}

// UserAPI exposes user reads. Mutations go through the host's user
// management (via events or admin routes); plugins may write to Extra for
// their own side-state.
type UserAPI interface {
	Find(ctx context.Context, id int64) (*User, error)
	List(ctx context.Context, filter UserFilter) ([]*User, error)
	PatchExtra(ctx context.Context, id int64, patch PatchFunc) error
}

// UserFilter filters user listings.
type UserFilter struct {
	Search string
	Role   string
	Status string
	Limit  int
	Offset int
}

// OrderAPI exposes order queries and targeted mutations (extra only).
type OrderAPI interface {
	Find(ctx context.Context, id string) (*Order, error)
	List(ctx context.Context, filter OrderFilter) ([]*Order, error)
	PatchExtra(ctx context.Context, id string, patch PatchFunc) error
}

// OrderFilter filters order listings.
type OrderFilter struct {
	UserID int64
	Status string
	Type   string
	Limit  int
	Offset int
}

// SubscriptionAPI exposes subscription queries.
type SubscriptionAPI interface {
	Find(ctx context.Context, id int64) (*Subscription, error)
	List(ctx context.Context, filter SubscriptionFilter) ([]*Subscription, error)
}

// SubscriptionFilter filters subscription listings.
type SubscriptionFilter struct {
	UserID int64
	PlanID int64
	Status string
	Limit  int
	Offset int
}

// BillingAPI lets plugins record usage and inspect the host's pricing
// table. Record fans the UsageRecord out to usage_log, pricing and balance
// deduction; GetModelPricing / ListSupportedModels expose the read-only
// model catalogue used by admin pages and quota estimators.
type BillingAPI interface {
	Record(ctx context.Context, record UsageRecord) error
	// GetModelPricing returns the per-token pricing for model. Returns
	// (nil, error) when the model is unknown.
	GetModelPricing(ctx context.Context, model string) (*ModelPricing, error)
	// ListSupportedModels returns the ids of every model with a known price.
	ListSupportedModels(ctx context.Context) ([]string, error)
}

// ModelPricing is the plugin-facing view of a model price row. Units are
// USD per token; zero fields mean "not priced in that dimension".
type ModelPricing struct {
	InputPricePerToken         float64
	OutputPricePerToken        float64
	CacheCreationPricePerToken float64
	CacheReadPricePerToken     float64
	ImageOutputPricePerToken   float64
}

// UsageRecord is the payload for [BillingAPI.Record].
type UsageRecord struct {
	UserID      int64
	AccountID   int64
	ChannelID   int64
	Model       string
	RequestID   string
	Usage       Usage
	OccurredAt  time.Time
	Extra       map[string]any
	BillingMode string
}

// RateLimitAPI manages per-account / per-model rate-limit state.
type RateLimitAPI interface {
	TryAcquire(ctx context.Context, accountID int64, model string) (bool, error)
	MarkLimited(ctx context.Context, accountID int64, model string, resetAt time.Time) error
	Remaining(ctx context.Context, accountID int64, model string) time.Duration
}

// ConcurrencyAPI manages session-level concurrency counters.
type ConcurrencyAPI interface {
	Acquire(ctx context.Context, key string, limit int) (release func(), err error)
	Release(ctx context.Context, key string)
}

// SchedulerAPI exposes read-only scheduler state for admin pages.
type SchedulerAPI interface {
	Snapshot(ctx context.Context) (SchedulerSnapshot, error)
}

// SchedulerSnapshot summarises the host's scheduling state at a point in
// time.
type SchedulerSnapshot struct {
	Accounts map[int64]AccountSchedulingState
}

// AccountSchedulingState describes one account's scheduler view.
type AccountSchedulingState struct {
	AccountID   int64
	Concurrency int
	InFlight    int
	LastUsedAt  time.Time
	RateLimited bool
}

// CacheStore is a key-value cache scoped to the plugin. Keys are
// automatically namespaced with "plugin:<id>:"; callers write bare keys.
type CacheStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Scan(ctx context.Context, pattern string) ([]string, error)
}

// HTTPUpstream performs outbound HTTP with the host's shared TLS fingerprint
// and rate-limit controls.
type HTTPUpstream interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

// JobQueue enqueues background work. Implementations back onto the host's
// existing worker pool; plugins should keep handlers idempotent.
type JobQueue interface {
	Enqueue(ctx context.Context, queue string, payload []byte, opts JobOptions) error
}

// JobOptions tune individual job submissions.
type JobOptions struct {
	MaxRetries int
	RunAfter   time.Duration
	// DedupeKey, when set, collapses concurrent submissions with the same
	// key into a single execution.
	DedupeKey string
}

// I18n is a minimal translation helper.
type I18n interface {
	// T returns the translation for key in the given lang, substituting
	// {name} placeholders with params. Falls back to key when the lang or
	// key is missing.
	T(lang, key string, params map[string]any) string
}

// Crypto wraps the host's symmetric encryption (the same key configured via
// PAYMENT_ENCRYPTION_KEY). Suitable for plugin-owned secrets persisted in
// side-tables.
type Crypto interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// NamespacedSettings reads/writes plugin-scoped settings. Keys are stored
// under "plugin:<id>:<key>" by the host; callers use bare keys.
type NamespacedSettings interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any) error
	OnChange(key string, handler func(ctx context.Context, oldValue, newValue any))
}

// PluginKV is a small, non-cached, durable key-value store for plugins that
// want to persist configuration without declaring their own table.
type PluginKV interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
	Scan(ctx context.Context, prefix string) (map[string][]byte, error)
}

// EventsLog is an append-only audit log plugins can use to record domain
// events. The host persists entries to a shared table and surfaces them in
// the admin UI under the plugin's tab.
type EventsLog interface {
	Append(ctx context.Context, entry EventLogEntry) error
	Query(ctx context.Context, filter EventLogFilter) ([]EventLogEntry, error)
}

// EventLogEntry is one record in the EventsLog.
type EventLogEntry struct {
	ID         int64
	PluginID   string
	Kind       string
	ActorID    int64
	TargetKind string
	TargetID   string
	Detail     map[string]any
	CreatedAt  time.Time
}

// EventLogFilter filters EventsLog queries.
type EventLogFilter struct {
	Kind       string
	TargetKind string
	TargetID   string
	Since      time.Time
	Until      time.Time
	Limit      int
	Offset     int
}

// PluginRegistry is a tiny read-only view of peer plugins, used by
// [PluginAs]. Implementations should be cheap and concurrency-safe.
type PluginRegistry interface {
	Lookup(id string) (Plugin, bool)
}
