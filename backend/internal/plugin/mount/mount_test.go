//go:build unit

package mount

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/plugin/eventbus"
	"github.com/Wei-Shaw/sub2api/internal/plugin/loader"
	"github.com/Wei-Shaw/sub2api/internal/plugin/repository"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fakePluginRepo is a minimal in-memory repository.PluginRepository used
// by mount tests. It lets us seed state rows without touching ent.
type fakePluginRepo struct {
	mu      sync.Mutex
	records map[string]*repository.PluginRecord
}

func newFakeRepo() *fakePluginRepo {
	return &fakePluginRepo{records: map[string]*repository.PluginRecord{}}
}

func (r *fakePluginRepo) Upsert(_ context.Context, rec *repository.PluginRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *rec
	r.records[rec.ID] = &cp
	return nil
}

func (r *fakePluginRepo) Find(_ context.Context, id string) (*repository.PluginRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[id]
	if !ok {
		return nil, repository.ErrPluginNotFound
	}
	cp := *rec
	return &cp, nil
}

func (r *fakePluginRepo) List(_ context.Context) ([]*repository.PluginRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*repository.PluginRecord, 0, len(r.records))
	for _, rec := range r.records {
		cp := *rec
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakePluginRepo) UpdateState(_ context.Context, id string, state string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[id]
	if !ok {
		return repository.ErrPluginNotFound
	}
	rec.State = state
	return nil
}

func (r *fakePluginRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, id)
	return nil
}

// fakePlugin is a minimal plugin.Plugin carrying a configurable Meta.
type fakePlugin struct {
	meta plugin.Meta
}

func (f *fakePlugin) Meta() plugin.Meta                { return f.meta }
func (f *fakePlugin) Init(_ plugin.CoreAPI) error      { return nil }
func (f *fakePlugin) Start(_ context.Context) error    { return nil }
func (f *fakePlugin) Shutdown(_ context.Context) error { return nil }

// registerOnce registers a fakePlugin under a unique id so parallel tests
// don't clash in the global registry. We ignore duplicate-registration
// panics because once registered the plugin remains for the rest of the
// test binary.
func registerOnce(t *testing.T, p plugin.Plugin) {
	t.Helper()
	defer func() { _ = recover() }()
	plugin.Register(p)
}

func newLoaderWithRepo(repo repository.PluginRepository) *loader.Loader {
	// Migrator and driver are nil — mount tests never call Install/Enable.
	return loader.NewLoader(repo, nil, nil, nil)
}

func TestMountRoutes(t *testing.T) {
	id := "mounttest-alpha"
	handlerHits := 0
	fp := &fakePlugin{
		meta: plugin.Meta{
			ID:         id,
			Version:    "0.0.1",
			APIVersion: plugin.SDKVersion,
			Routes: []plugin.RouteSpec{
				{
					Method: http.MethodGet,
					Path:   "/plugin-mount-test/public",
					Auth:   plugin.AuthNone,
					Handler: func(c *gin.Context) {
						handlerHits++
						c.String(http.StatusOK, "public")
					},
				},
				{
					Method: http.MethodGet,
					Path:   "/plugin-mount-test/admin",
					Auth:   plugin.AuthAdmin,
					Handler: func(c *gin.Context) {
						handlerHits++
						c.String(http.StatusOK, "admin")
					},
				},
			},
		},
	}
	registerOnce(t, fp)

	repo := newFakeRepo()
	require.NoError(t, repo.Upsert(context.Background(), &repository.PluginRecord{
		ID: id, Version: "0.0.1", APIVersion: plugin.SDKVersion,
		State: repository.PluginStateEnabled,
	}))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminCalled := 0
	auth := AuthMiddlewares{
		Admin: func(c *gin.Context) {
			adminCalled++
			c.Next()
		},
	}
	err := MountRoutes(context.Background(), router, newLoaderWithRepo(repo), auth, slog.Default())
	require.NoError(t, err)

	// Public route: hits handler, no auth middleware.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/plugin-mount-test/public", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "public", w.Body.String())

	// Admin route: admin middleware runs before handler.
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/plugin-mount-test/admin", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, adminCalled, "admin middleware must run exactly once")
	require.Equal(t, 2, handlerHits, "both handlers should have run")
}

func TestMountRoutesSkipsDisabled(t *testing.T) {
	id := "mounttest-disabled"
	handlerHits := 0
	fp := &fakePlugin{
		meta: plugin.Meta{
			ID:         id,
			Version:    "0.0.1",
			APIVersion: plugin.SDKVersion,
			Routes: []plugin.RouteSpec{
				{
					Method: http.MethodGet,
					Path:   "/plugin-mount-test/disabled",
					Auth:   plugin.AuthNone,
					Handler: func(c *gin.Context) {
						handlerHits++
						c.String(http.StatusOK, "should not mount")
					},
				},
			},
		},
	}
	registerOnce(t, fp)

	repo := newFakeRepo()
	require.NoError(t, repo.Upsert(context.Background(), &repository.PluginRecord{
		ID: id, Version: "0.0.1", APIVersion: plugin.SDKVersion,
		State: repository.PluginStateDisabled,
	}))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	err := MountRoutes(context.Background(), router, newLoaderWithRepo(repo), AuthMiddlewares{}, slog.Default())
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/plugin-mount-test/disabled", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code, "disabled plugin routes must not mount")
	require.Equal(t, 0, handlerHits)
}

func TestMountSubscriptions(t *testing.T) {
	id := "mounttest-subs"
	fp := &fakePlugin{
		meta: plugin.Meta{
			ID:         id,
			Version:    "0.0.1",
			APIVersion: plugin.SDKVersion,
			Subscribes: []plugin.EventSubscription{
				{
					Topic:         plugin.TopicAccountCreated,
					Kind:          plugin.EventKindNotify,
					SubscriberTag: id + "/account-created",
					Handler: func(_ context.Context, _ any) error {
						return nil
					},
				},
				{
					Topic:         plugin.TopicOrderFulfilled,
					Kind:          plugin.EventKindNotify,
					SubscriberTag: id + "/order-fulfilled",
					Handler: func(_ context.Context, _ any) error {
						return nil
					},
				},
			},
		},
	}
	registerOnce(t, fp)

	repo := newFakeRepo()
	require.NoError(t, repo.Upsert(context.Background(), &repository.PluginRecord{
		ID: id, Version: "0.0.1", APIVersion: plugin.SDKVersion,
		State: repository.PluginStateEnabled,
	}))

	registry := eventbus.NewRegistry()
	require.NoError(t, eventbus.RegisterCoreSchemas(registry))
	bus, err := eventbus.NewBus(eventbus.BusOptions{Registry: registry})
	require.NoError(t, err)

	require.NoError(t, MountSubscriptions(context.Background(), bus, newLoaderWithRepo(repo), slog.Default()))

	// Publishing the subscribed topics must not error even though bus has
	// no async dispatcher (all topics are Notify kind).
	ctx := context.Background()
	bus.PublishNotify(ctx, plugin.TopicAccountCreated, &plugin.Account{})
	bus.PublishNotify(ctx, plugin.TopicOrderFulfilled, plugin.OrderEvent{})
}
