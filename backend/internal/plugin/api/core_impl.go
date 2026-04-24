package api

import (
	"log/slog"

	"entgo.io/ent/dialect"
	"github.com/redis/go-redis/v9"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// Dependencies collects every host-side dependency the factory injects into
// CoreAPI implementations. New sub-APIs should extend this struct; existing
// fields must not change meaning without a compat shim.
//
// All fields are optional — missing dependencies produce sub-APIs that
// return plugin.ErrNotImplemented for their methods. This keeps early-boot
// callers (before wire has finished) safe.
type Dependencies struct {
	AccountService   *service.AccountService
	AccountRepo      service.AccountRepository
	BillingService   *service.BillingService
	UsageBillingRepo service.UsageBillingRepository
	HTTPUpstream     service.HTTPUpstream
	Redis            *redis.Client
	Encryptor        service.SecretEncryptor
	SettingRepo      service.SettingRepository
	BaseLogger       *slog.Logger
	// EventBus is the shared host event bus. When non-nil, CoreAPI.Events()
	// returns this bus; otherwise a stub that always returns
	// plugin.ErrNotImplemented is used.
	EventBus plugin.EventBus
	// EntDriver is the shared ent dialect driver. Plugins that own their
	// own ent client build it with gen.NewClient(gen.Driver(driver)) inside
	// their Init. When nil, CoreAPI.EntDriver returns nil and plugins must
	// treat that as an unrecoverable boot-order error for any feature that
	// needs database access.
	EntDriver dialect.Driver
}

// coreAPIImpl is the host-side CoreAPI implementation returned by the
// factory. Sub-API wrappers are constructed eagerly in NewCoreAPIFactory
// and cached for the lifetime of the CoreAPI instance.
//
// Caching is safe because guards are keyed on the declared permission set,
// which cannot change at runtime (permission revocation requires a plugin
// disable/re-enable cycle that yields a fresh CoreAPI).
type coreAPIImpl struct {
	pluginID string
	guard    *guard
	deps     Dependencies
	logger   plugin.Logger

	accounts      plugin.AccountAPI
	users         plugin.UserAPI
	orders        plugin.OrderAPI
	subscriptions plugin.SubscriptionAPI
	billing       plugin.BillingAPI
	rateLimit     plugin.RateLimitAPI
	concurrency   plugin.ConcurrencyAPI
	scheduler     plugin.SchedulerAPI
	cache         plugin.CacheStore
	httpClient    plugin.HTTPUpstream
	jobs          plugin.JobQueue
	i18n          plugin.I18n
	crypto        plugin.Crypto
	settings      plugin.NamespacedSettings
	kv            plugin.PluginKV
	eventsLog     plugin.EventsLog
}

// PluginID returns the plugin id this CoreAPI was minted for.
func (c *coreAPIImpl) PluginID() string { return c.pluginID }

// Logger returns the plugin-tagged logger.
func (c *coreAPIImpl) Logger() plugin.Logger { return c.logger }

// Accounts returns the AccountAPI.
func (c *coreAPIImpl) Accounts() plugin.AccountAPI { return c.accounts }

// Users returns the UserAPI (Phase 0 stub).
func (c *coreAPIImpl) Users() plugin.UserAPI { return c.users }

// Orders returns the OrderAPI (Phase 0 stub).
func (c *coreAPIImpl) Orders() plugin.OrderAPI { return c.orders }

// Subscriptions returns the SubscriptionAPI (Phase 0 stub).
func (c *coreAPIImpl) Subscriptions() plugin.SubscriptionAPI { return c.subscriptions }

// Billing returns the BillingAPI.
func (c *coreAPIImpl) Billing() plugin.BillingAPI { return c.billing }

// RateLimit returns the RateLimitAPI (Phase 0 stub).
func (c *coreAPIImpl) RateLimit() plugin.RateLimitAPI { return c.rateLimit }

// Concurrency returns the ConcurrencyAPI (Phase 0 stub).
func (c *coreAPIImpl) Concurrency() plugin.ConcurrencyAPI { return c.concurrency }

// Scheduler returns the SchedulerAPI (Phase 0 stub).
func (c *coreAPIImpl) Scheduler() plugin.SchedulerAPI { return c.scheduler }

// Cache returns the CacheStore scoped to plugin:<id>:.
func (c *coreAPIImpl) Cache() plugin.CacheStore { return c.cache }

// HTTP returns the HTTPUpstream wrapper.
func (c *coreAPIImpl) HTTP() plugin.HTTPUpstream { return c.httpClient }

// Jobs returns the JobQueue (Phase 0 stub).
func (c *coreAPIImpl) Jobs() plugin.JobQueue { return c.jobs }

// I18n returns the I18n helper (Phase 0 stub).
func (c *coreAPIImpl) I18n() plugin.I18n { return c.i18n }

// Crypto returns the Crypto helper.
func (c *coreAPIImpl) Crypto() plugin.Crypto { return c.crypto }

// Settings returns the NamespacedSettings (read-only in Phase 0).
func (c *coreAPIImpl) Settings() plugin.NamespacedSettings { return c.settings }

// KV returns the PluginKV (Phase 0 stub).
func (c *coreAPIImpl) KV() plugin.PluginKV { return c.kv }

// EventsLog returns the EventsLog (Phase 0 stub).
func (c *coreAPIImpl) EventsLog() plugin.EventsLog { return c.eventsLog }

// Events returns the EventBus. The bus lives in a sibling package and is
// wired in a later wave; Phase 0 returns a stub that always fails.
func (c *coreAPIImpl) Events() plugin.EventBus {
	if c.deps.EventBus != nil {
		return c.deps.EventBus
	}
	return unimplementedBus{}
}

// Plugins is backed by the SDK's global registry so cross-plugin lookups
// (via plugin.PluginAs) work out of the box.
func (c *coreAPIImpl) Plugins() plugin.PluginRegistry { return sdkRegistry{} }

// EntDriver exposes the host's ent dialect driver so plugins can build
// their own ent clients. Returns nil when no driver has been wired into
// Dependencies yet.
func (c *coreAPIImpl) EntDriver() dialect.Driver { return c.deps.EntDriver }
