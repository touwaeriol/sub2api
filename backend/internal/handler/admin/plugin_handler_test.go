//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plugin/eventbus"
	"github.com/Wei-Shaw/sub2api/internal/plugin/loader"
	"github.com/Wei-Shaw/sub2api/internal/plugin/repository"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ------------------------------------------------------------------
// Stubs
// ------------------------------------------------------------------

type stubLoader struct {
	installErr   error
	enableErr    error
	disableErr   error
	uninstallErr error
	listErr      error
	findErr      error
	states       []loader.PluginState
	findResult   *loader.PluginState

	installCalls   []string
	enableCalls    []string
	disableCalls   []string
	uninstallCalls []struct {
		id    string
		purge bool
	}
}

func (s *stubLoader) Install(ctx context.Context, p plugin.Plugin) error {
	s.installCalls = append(s.installCalls, p.Meta().ID)
	return s.installErr
}

func (s *stubLoader) Enable(ctx context.Context, id string) error {
	s.enableCalls = append(s.enableCalls, id)
	return s.enableErr
}

func (s *stubLoader) Disable(ctx context.Context, id string) error {
	s.disableCalls = append(s.disableCalls, id)
	return s.disableErr
}

func (s *stubLoader) Uninstall(ctx context.Context, id string, purge bool) error {
	s.uninstallCalls = append(s.uninstallCalls, struct {
		id    string
		purge bool
	}{id, purge})
	return s.uninstallErr
}

func (s *stubLoader) ListStates(ctx context.Context) ([]loader.PluginState, error) {
	return s.states, s.listErr
}

func (s *stubLoader) FindState(ctx context.Context, id string) (*loader.PluginState, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	return s.findResult, nil
}

type stubDeadLetterRepo struct {
	listEntries []eventbus.DeadLetterEntry
	listErr     error
	retryErr    error
	deleteErr   error

	retryCalls  []int64
	deleteCalls []int64
}

func (r *stubDeadLetterRepo) Record(_ context.Context, _ eventbus.DeadLetterEntry) error {
	return nil
}

func (r *stubDeadLetterRepo) List(_ context.Context, _ eventbus.DeadLetterFilter) ([]eventbus.DeadLetterEntry, error) {
	return r.listEntries, r.listErr
}

func (r *stubDeadLetterRepo) Retry(_ context.Context, id int64, retrier eventbus.DeadLetterRetrier) error {
	r.retryCalls = append(r.retryCalls, id)
	if r.retryErr != nil {
		return r.retryErr
	}
	// Exercise the retrier to surface panics in the test.
	if retrier != nil {
		_ = retrier(context.Background(), "test.topic", []byte(`{}`))
	}
	return nil
}

func (r *stubDeadLetterRepo) Delete(_ context.Context, id int64) error {
	r.deleteCalls = append(r.deleteCalls, id)
	return r.deleteErr
}

type stubRetrier struct {
	retrier eventbus.DeadLetterRetrier
}

func (s *stubRetrier) Retrier() eventbus.DeadLetterRetrier { return s.retrier }

// ------------------------------------------------------------------
// Test helpers
// ------------------------------------------------------------------

func newTestHandler(l pluginLifecycle, dlr eventbus.DeadLetterRepo, retrier deadLetterRetrier) *PluginAdminHandler {
	return &PluginAdminHandler{
		loader:         l,
		deadLetterRepo: dlr,
		busAdapter:     retrier,
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// fixLogger guarantees a non-nil logger so error-path tests don't NPE.
func fixLogger(h *PluginAdminHandler) *PluginAdminHandler {
	if h.logger == nil {
		h.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return h
}

func newEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	if len(body) == 0 {
		return nil
	}
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

// ------------------------------------------------------------------
// Tests
// ------------------------------------------------------------------

func TestPluginAdminHandler_ListPlugins_ReturnsStates(t *testing.T) {
	sl := &stubLoader{
		states: []loader.PluginState{
			{
				ID:          "demo",
				Version:     "0.1.0",
				APIVersion:  "1.0.0",
				State:       "enabled",
				InstalledAt: time.Unix(1_700_000_000, 0),
			},
		},
	}
	h := newTestHandler(sl, &stubDeadLetterRepo{}, nil)
	// Wrap logger with real slog to avoid nil-deref in error paths.
	h = fixLogger(h)

	router := newEngine()
	router.GET("/api/v1/admin/plugins", h.ListPlugins)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeBody(t, rec)
	require.Equal(t, float64(0), body["code"])
	data := body["data"].(map[string]any)
	plugins := data["plugins"].([]any)
	require.GreaterOrEqual(t, len(plugins), 1)
	first := plugins[0].(map[string]any)
	require.Equal(t, "demo", first["id"])
	require.Equal(t, "enabled", first["state"])
}

func TestPluginAdminHandler_ListPlugins_LoaderError(t *testing.T) {
	sl := &stubLoader{listErr: errors.New("db down")}
	h := fixLogger(newTestHandler(sl, &stubDeadLetterRepo{}, nil))

	router := newEngine()
	router.GET("/api/v1/admin/plugins", h.ListPlugins)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	body := decodeBody(t, rec)
	require.Equal(t, float64(ErrCodePluginInternal), body["code"])
}

func TestPluginAdminHandler_EnableDisable_CallsLoader(t *testing.T) {
	sl := &stubLoader{}
	h := fixLogger(newTestHandler(sl, &stubDeadLetterRepo{}, nil))

	router := newEngine()
	router.POST("/api/v1/admin/plugins/:id/enable", h.EnablePlugin)
	router.POST("/api/v1/admin/plugins/:id/disable", h.DisablePlugin)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/demo/enable", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"demo"}, sl.enableCalls)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/demo/disable", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"demo"}, sl.disableCalls)
}

func TestPluginAdminHandler_Enable_NotFound(t *testing.T) {
	sl := &stubLoader{enableErr: fmt.Errorf("enable: %w", plugin.ErrPluginNotFound)}
	h := fixLogger(newTestHandler(sl, &stubDeadLetterRepo{}, nil))

	router := newEngine()
	router.POST("/api/v1/admin/plugins/:id/enable", h.EnablePlugin)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/missing/enable", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	body := decodeBody(t, rec)
	require.Equal(t, float64(ErrCodePluginNotFound), body["code"])
	details := body["details"].(map[string]any)
	require.Equal(t, "missing", details[ErrDetailPluginID])
}

func TestPluginAdminHandler_Enable_InvalidState(t *testing.T) {
	sl := &stubLoader{enableErr: fmt.Errorf("wrap: %w", loader.ErrInvalidState)}
	h := fixLogger(newTestHandler(sl, &stubDeadLetterRepo{}, nil))

	router := newEngine()
	router.POST("/api/v1/admin/plugins/:id/enable", h.EnablePlugin)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/x/enable", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	body := decodeBody(t, rec)
	require.Equal(t, float64(ErrCodePluginInvalidState), body["code"])
}

func TestPluginAdminHandler_Uninstall_WithPurge(t *testing.T) {
	sl := &stubLoader{}
	h := fixLogger(newTestHandler(sl, &stubDeadLetterRepo{}, nil))

	router := newEngine()
	router.POST("/api/v1/admin/plugins/:id/uninstall", h.UninstallPlugin)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/demo/uninstall",
		bytes.NewReader([]byte(`{"purge":true}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, sl.uninstallCalls, 1)
	require.True(t, sl.uninstallCalls[0].purge)
}

func TestPluginAdminHandler_GetPlugin_Missing(t *testing.T) {
	sl := &stubLoader{findErr: repository.ErrPluginNotFound}
	h := fixLogger(newTestHandler(sl, &stubDeadLetterRepo{}, nil))

	router := newEngine()
	router.GET("/api/v1/admin/plugins/:id", h.GetPlugin)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins/unknown", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	body := decodeBody(t, rec)
	require.Equal(t, float64(ErrCodePluginNotFound), body["code"])
}

func TestPluginAdminHandler_ListDeadLetters(t *testing.T) {
	dlr := &stubDeadLetterRepo{
		listEntries: []eventbus.DeadLetterEntry{
			{ID: 1, Topic: "account.created", SubscriberTag: "demo", AttemptCount: 3,
				Payload: []byte(`{"hello":"world"}`),
				FirstFailedAt: time.Unix(1_700_000_000, 0), LastAttemptAt: time.Unix(1_700_000_100, 0)},
		},
	}
	h := fixLogger(newTestHandler(&stubLoader{}, dlr, nil))

	router := newEngine()
	router.GET("/api/v1/admin/plugins/dead-letters", h.ListDeadLetters)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/plugins/dead-letters?plugin_id=demo&page=1&page_size=25", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeBody(t, rec)
	data := body["data"].(map[string]any)
	items := data["items"].([]any)
	require.Len(t, items, 1)
	first := items[0].(map[string]any)
	require.Equal(t, "account.created", first["topic"])
	require.Equal(t, "demo", first["subscriber_tag"])
}

func TestPluginAdminHandler_RetryDeadLetter_InvokesRetrier(t *testing.T) {
	dlr := &stubDeadLetterRepo{}
	called := 0
	retrier := &stubRetrier{retrier: func(_ context.Context, topic string, _ []byte) error {
		called++
		require.Equal(t, "test.topic", topic)
		return nil
	}}
	h := fixLogger(newTestHandler(&stubLoader{}, dlr, retrier))

	router := newEngine()
	router.POST("/api/v1/admin/plugins/dead-letters/:id/retry", h.RetryDeadLetter)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/dead-letters/42/retry", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []int64{42}, dlr.retryCalls)
	require.Equal(t, 1, called)
}

func TestPluginAdminHandler_RetryDeadLetter_NoAsync(t *testing.T) {
	dlr := &stubDeadLetterRepo{}
	retrier := &stubRetrier{retrier: nil}
	h := fixLogger(newTestHandler(&stubLoader{}, dlr, retrier))

	router := newEngine()
	router.POST("/api/v1/admin/plugins/dead-letters/:id/retry", h.RetryDeadLetter)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/dead-letters/7/retry", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	body := decodeBody(t, rec)
	require.Equal(t, float64(ErrCodeDeadLetterRetryFailed), body["code"])
}

func TestPluginAdminHandler_DeleteDeadLetter_NotFound(t *testing.T) {
	dlr := &stubDeadLetterRepo{deleteErr: eventbus.ErrDeadLetterNotFound}
	h := fixLogger(newTestHandler(&stubLoader{}, dlr, nil))

	router := newEngine()
	router.DELETE("/api/v1/admin/plugins/dead-letters/:id", h.DeleteDeadLetter)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/plugins/dead-letters/99", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	body := decodeBody(t, rec)
	require.Equal(t, float64(ErrCodeDeadLetterNotFound), body["code"])
}

func TestPluginAdminHandler_DeadLetter_BadPageParam(t *testing.T) {
	h := fixLogger(newTestHandler(&stubLoader{}, &stubDeadLetterRepo{}, nil))

	router := newEngine()
	router.GET("/api/v1/admin/plugins/dead-letters", h.ListDeadLetters)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins/dead-letters?page=0", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
