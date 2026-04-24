package api

import (
	"log/slog"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// Factory produces a plugin.CoreAPI per plugin id, sharing host services
// across all plugins but isolating each instance by pluginID and
// declared permission set.
type Factory struct {
	deps Dependencies
}

// NewCoreAPIFactory constructs a Factory with the given host dependencies.
// Every field of deps is optional: missing fields cause the corresponding
// sub-API to return plugin.ErrNotImplemented at call time, not at boot.
func NewCoreAPIFactory(deps Dependencies) *Factory {
	if deps.BaseLogger == nil {
		deps.BaseLogger = slog.Default()
	}
	return &Factory{deps: deps}
}

// For materialises a CoreAPI for pluginID, scoped to perms. Always returns
// a non-nil CoreAPI — permission enforcement happens at call time, so the
// loader can hand the result to the plugin's Init even when perms is empty.
func (f *Factory) For(pluginID string, perms []plugin.Permission) plugin.CoreAPI {
	g := newGuard(pluginID, perms)
	impl := &coreAPIImpl{
		pluginID: pluginID,
		guard:    g,
		deps:     f.deps,
		logger:   newLogger(f.deps.BaseLogger, pluginID),
	}

	impl.accounts = newAccountAPI(impl)
	impl.billing = newBillingAPI(impl)
	impl.httpClient = newHTTPUpstream(impl)
	impl.cache = newCacheStore(impl)
	impl.crypto = newCrypto(impl)
	impl.settings = newSettings(impl)

	impl.users = unimplementedUserAPI{}
	impl.orders = unimplementedOrderAPI{}
	impl.subscriptions = unimplementedSubscriptionAPI{}
	impl.rateLimit = unimplementedRateLimitAPI{}
	impl.concurrency = unimplementedConcurrencyAPI{}
	impl.scheduler = unimplementedSchedulerAPI{}
	impl.jobs = unimplementedJobQueue{}
	impl.i18n = unimplementedI18n{}
	impl.kv = unimplementedKV{}
	impl.eventsLog = unimplementedEventsLog{}

	return impl
}
