// Package plugin wires the plugin subsystem (loader, eventbus, core API
// factory, repositories) into the main application via Google Wire.
//
// The exported [ProviderSet] is consumed by backend/cmd/server/wire.go and
// assembles:
//
//   - repositories (plugin_repo, migration_repo)
//   - loader (Migrator + Loader)
//   - CoreAPI factory
//   - in-memory event bus (registry, dead-letter repo, job queue, bus)
//
// The plugin subsystem is intentionally independent from the service layer:
// plugins reach the host only through the CoreAPI contract defined in
// github.com/Wei-Shaw/sub2api/pkg/plugin, never via wire. This keeps
// service packages from growing a hidden dependency on plugin internals.
package plugin

import (
	"database/sql"
	"errors"
	"log/slog"

	"entgo.io/ent/dialect"

	"github.com/Wei-Shaw/sub2api/ent"
	pluginapi "github.com/Wei-Shaw/sub2api/internal/plugin/api"
	"github.com/Wei-Shaw/sub2api/internal/plugin/eventbus"
	"github.com/Wei-Shaw/sub2api/internal/plugin/loader"
	"github.com/Wei-Shaw/sub2api/internal/plugin/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	plugin "github.com/Wei-Shaw/sub2api/pkg/plugin"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

// asyncWorkerCount sizes the in-memory job queue used by the async
// dispatcher. One worker is enough for Phase 0 — no plugin ships async
// hooks yet and we still want serial ordering within a topic.
const asyncWorkerCount = 1

// errNilEntClient is returned by ProvideDialectDriver when wire invokes
// it with a nil client (should never happen in practice because the
// ent.Client provider fails earlier, but keeping the guard makes the
// intent explicit).
var errNilEntClient = errors.New("plugin wire: nil ent client")

// ProviderSet bundles every plugin-subsystem constructor so the main wire
// graph can add it with a single line. Keep this set small and cohesive;
// handler-level wiring lives in internal/handler/admin/wire.go.
var ProviderSet = wire.NewSet(
	// Repositories
	repository.NewPluginRepository,
	ProvideMigrationRepository,

	// Core infrastructure
	ProvideDialectDriver,
	ProvideLoaderLogger,
	loader.NewMigrator,
	loader.NewLoader,

	// CoreAPI factory
	pluginapi.NewCoreAPIFactory,
	ProvidePluginDependencies,
	wire.Bind(new(loader.CoreAPIFactory), new(*pluginapi.Factory)),

	// Event bus
	eventbus.NewRegistry,
	ProvideInMemoryJobQueue,
	ProvideDeadLetterRepo,
	ProvideEventBus,
	wire.Bind(new(plugin.EventBus), new(*eventbus.Bus)),
)

// ProvideDialectDriver exposes the *entsql.Driver already owned by the
// ent.Client as a dialect.Driver. The migrator needs the driver to run
// SchemaProvider.CreateOrUpgrade without opening a second connection.
func ProvideDialectDriver(client *ent.Client) (dialect.Driver, error) {
	if client == nil {
		return nil, errNilEntClient
	}
	return client.Driver(), nil
}

// ProvideLoaderLogger returns the default slog logger tagged for the
// plugin subsystem. Keeping a dedicated constructor avoids pulling a
// package-wide logger provider.
func ProvideLoaderLogger() *slog.Logger {
	return slog.Default().With("subsystem", "plugin")
}

// ProvideMigrationRepository wraps the SQL-backed migration repo
// constructor so the wire graph does not need to know the underlying
// return signature.
func ProvideMigrationRepository(db *sql.DB) (repository.MigrationRepository, error) {
	return repository.NewMigrationRepository(db)
}

// ProvideInMemoryJobQueue constructs the Phase 0 async job queue.
// A SQL-backed implementation will replace this in a later phase; the
// public wire.go signature stays stable because callers depend on the
// JobEnqueuer and HandlerRegistry interfaces.
func ProvideInMemoryJobQueue() *eventbus.InMemoryJobQueue {
	return eventbus.NewInMemoryJobQueue(asyncWorkerCount)
}

// ProvideDeadLetterRepo binds the SQL-backed dead-letter repository used
// by the async dispatcher.
func ProvideDeadLetterRepo(db *sql.DB) eventbus.DeadLetterRepo {
	return eventbus.NewSQLDeadLetterRepo(db)
}

// ProvideEventBus assembles the bus and registers core schemas so plugins
// booted by the loader can publish/subscribe immediately after Init.
func ProvideEventBus(
	registry *eventbus.Registry,
	jobQueue *eventbus.InMemoryJobQueue,
	deadLetter eventbus.DeadLetterRepo,
) (*eventbus.Bus, error) {
	if err := eventbus.RegisterCoreSchemas(registry); err != nil {
		return nil, err
	}
	bus, err := eventbus.NewBus(eventbus.BusOptions{
		Registry:        registry,
		JobQueue:        jobQueue,
		HandlerRegistry: jobQueue,
		DeadLetterRepo:  deadLetter,
	})
	if err != nil {
		return nil, err
	}
	return bus, nil
}

// ProvidePluginDependencies adapts already-injected service/repo and
// infrastructure instances into the api.Dependencies struct consumed by
// the CoreAPI factory. The bus is passed as plugin.EventBus so the
// CoreAPI contract stays interface-only.
func ProvidePluginDependencies(
	accountSvc *service.AccountService,
	accountRepo service.AccountRepository,
	billingSvc *service.BillingService,
	usageBillingRepo service.UsageBillingRepository,
	httpUpstream service.HTTPUpstream,
	rdb *redis.Client,
	encryptor service.SecretEncryptor,
	settingRepo service.SettingRepository,
	logger *slog.Logger,
	bus plugin.EventBus,
) pluginapi.Dependencies {
	return pluginapi.Dependencies{
		AccountService:   accountSvc,
		AccountRepo:      accountRepo,
		BillingService:   billingSvc,
		UsageBillingRepo: usageBillingRepo,
		HTTPUpstream:     httpUpstream,
		Redis:            rdb,
		Encryptor:        encryptor,
		SettingRepo:      settingRepo,
		BaseLogger:       logger,
		EventBus:         bus,
	}
}
