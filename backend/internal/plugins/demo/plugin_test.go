//go:build unit

package demo

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	"github.com/gin-gonic/gin"

	gen "github.com/Wei-Shaw/sub2api/internal/plugins/demo/ent/gen"
	"github.com/Wei-Shaw/sub2api/internal/plugins/demo/ent/gen/enttest"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// openTestClient spins up an in-memory sqlite ent client for the demo
// plugin. The schema migration runs implicitly via enttest.NewClient so
// tests can issue queries without scaffolding their own migrator.
func openTestClient(t *testing.T) *gen.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:demo_plugin_test?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	drv := entsql.OpenDB(dialect.SQLite, db)
	return enttest.NewClient(t, enttest.WithOptions(gen.Driver(drv)))
}

// stubCore is a minimal plugin.CoreAPI implementation for handler and
// subscriber unit tests. Only the methods exercised by the demo plugin
// are backed by real stubs; the rest panic if accidentally called so
// missing wiring surfaces loudly.
type stubCore struct {
	settings *stubSettings
	logger   *stubLogger
	driver   dialect.Driver
}

func newStubCore() *stubCore {
	return &stubCore{
		settings: &stubSettings{values: map[string]any{}},
		logger:   &stubLogger{},
	}
}

func (s *stubCore) PluginID() string                      { return PluginID }
func (s *stubCore) Logger() plugin.Logger                 { return s.logger }
func (s *stubCore) Accounts() plugin.AccountAPI           { panic("not used") }
func (s *stubCore) Users() plugin.UserAPI                 { panic("not used") }
func (s *stubCore) Orders() plugin.OrderAPI               { panic("not used") }
func (s *stubCore) Subscriptions() plugin.SubscriptionAPI { panic("not used") }
func (s *stubCore) Billing() plugin.BillingAPI            { panic("not used") }
func (s *stubCore) RateLimit() plugin.RateLimitAPI        { panic("not used") }
func (s *stubCore) Concurrency() plugin.ConcurrencyAPI    { panic("not used") }
func (s *stubCore) Scheduler() plugin.SchedulerAPI        { panic("not used") }
func (s *stubCore) Cache() plugin.CacheStore              { panic("not used") }
func (s *stubCore) HTTP() plugin.HTTPUpstream             { panic("not used") }
func (s *stubCore) Jobs() plugin.JobQueue                 { panic("not used") }
func (s *stubCore) I18n() plugin.I18n                     { panic("not used") }
func (s *stubCore) Crypto() plugin.Crypto                 { panic("not used") }
func (s *stubCore) Settings() plugin.NamespacedSettings   { return s.settings }
func (s *stubCore) KV() plugin.PluginKV                   { panic("not used") }
func (s *stubCore) EventsLog() plugin.EventsLog           { panic("not used") }
func (s *stubCore) Events() plugin.EventBus               { panic("not used") }
func (s *stubCore) Plugins() plugin.PluginRegistry        { panic("not used") }
func (s *stubCore) EntDriver() dialect.Driver             { return s.driver }

// stubSettings is a bare NamespacedSettings keyed in memory.
type stubSettings struct {
	values map[string]any
}

func (s *stubSettings) Get(_ context.Context, key string) (any, error) {
	v, ok := s.values[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (s *stubSettings) Set(_ context.Context, key string, value any) error {
	s.values[key] = value
	return nil
}

func (s *stubSettings) OnChange(string, func(context.Context, any, any)) {}

// stubLogger captures log calls for assertions.
type stubLogger struct{ entries []string }

func (l *stubLogger) Debug(msg string, _ ...any) { l.entries = append(l.entries, "DEBUG "+msg) }
func (l *stubLogger) Info(msg string, _ ...any)  { l.entries = append(l.entries, "INFO "+msg) }
func (l *stubLogger) Warn(msg string, _ ...any)  { l.entries = append(l.entries, "WARN "+msg) }
func (l *stubLogger) Error(msg string, _ ...any) { l.entries = append(l.entries, "ERROR "+msg) }

// invokeHandler runs a handler against a fresh test recorder.
func invokeHandler(t *testing.T, h gin.HandlerFunc, method, url string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.Handle(method, url, h)
	req := httptest.NewRequest(method, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// decodeJSON decodes the recorder body into a generic map. Fails the test
// on decode error.
func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	body, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode json: %v raw=%s", err, body)
	}
	return out
}

// TestMetaFields verifies the declarative Meta returned by the plugin has
// every required field populated so the host loader does not reject it.
func TestMetaFields(t *testing.T) {
	p := &Plugin{}
	meta := p.Meta()
	if meta.ID != PluginID {
		t.Fatalf("Meta.ID=%q want %q", meta.ID, PluginID)
	}
	if meta.Name == "" || meta.Description == "" || meta.Version == "" {
		t.Fatalf("Meta missing human-readable metadata: %+v", meta)
	}
	if meta.APIVersion != plugin.SDKVersion {
		t.Fatalf("Meta.APIVersion=%q want %q", meta.APIVersion, plugin.SDKVersion)
	}
	if len(meta.Tables) != 1 || !strings.HasPrefix(meta.Tables[0], "plugin_demo_") {
		t.Fatalf("Meta.Tables should declare plugin_demo_ prefixed tables; got %v", meta.Tables)
	}
	if err := plugin.AssertTableName(PluginID, meta.Tables[0]); err != nil {
		t.Fatalf("declared table violates prefix rule: %v", err)
	}
	if meta.Schema == nil {
		t.Fatalf("Meta.Schema is nil; host cannot apply DDL")
	}
	if len(meta.Routes) != 2 {
		t.Fatalf("Meta.Routes count=%d want 2", len(meta.Routes))
	}
	if len(meta.Settings) != 1 || meta.Settings[0].Key != settingKeyGreeting {
		t.Fatalf("Meta.Settings missing greeting slot: %+v", meta.Settings)
	}
	if len(meta.Subscribes) != 1 ||
		meta.Subscribes[0].Topic != plugin.TopicAccountCreated ||
		meta.Subscribes[0].Kind != plugin.EventKindNotify {
		t.Fatalf("Meta.Subscribes should subscribe to account.created Notify: %+v", meta.Subscribes)
	}
	if meta.Exports == nil {
		t.Fatalf("Meta.Exports is nil; cross-plugin handle unavailable")
	}
}

// TestHelloHandler exercises the public /hello route with and without a
// configured greeting.
func TestHelloHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core := newStubCore()
	p := &Plugin{core: core}

	resp := invokeHandler(t, p.hello, "GET", "/api/v1/plugin/demo/hello")
	payload := decodeJSON(t, resp)
	if payload["greeting"] != defaultGreeting {
		t.Fatalf("greeting=%v want %q", payload["greeting"], defaultGreeting)
	}
	if payload["plugin"] != PluginID {
		t.Fatalf("plugin=%v want %q", payload["plugin"], PluginID)
	}

	_ = core.settings.Set(context.Background(), settingKeyGreeting, "hola")
	resp = invokeHandler(t, p.hello, "GET", "/api/v1/plugin/demo/hello")
	payload = decodeJSON(t, resp)
	if payload["greeting"] != "hola" {
		t.Fatalf("greeting=%v want hola", payload["greeting"])
	}
}

// TestListNotesHandler exercises the admin listing route against an in-
// memory ent client.
func TestListNotesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := openTestClient(t)
	ctx := context.Background()
	_, err := client.Note.Create().
		SetAccountID(42).
		SetContent("test note").
		SetCreatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed note: %v", err)
	}

	p := &Plugin{core: newStubCore(), client: client}
	resp := invokeHandler(t, p.listNotes, "GET", "/api/v1/plugin/demo/notes")
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Notes []noteView `json:"notes"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Notes) != 1 || payload.Notes[0].AccountID != 42 {
		t.Fatalf("unexpected notes: %+v", payload.Notes)
	}
}

// TestListNotesHandlerWithoutClient guards the degraded path when no
// driver has been wired — the handler must return 503 rather than panic.
func TestListNotesHandlerWithoutClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p := &Plugin{core: newStubCore()}
	resp := invokeHandler(t, p.listNotes, "GET", "/api/v1/plugin/demo/notes")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", resp.Code)
	}
}

// TestOnAccountCreatedWritesNote proves the event subscriber persists an
// audit row end to end.
func TestOnAccountCreatedWritesNote(t *testing.T) {
	client := openTestClient(t)
	p := &Plugin{core: newStubCore(), client: client}
	ctx := context.Background()

	err := p.onAccountCreated(ctx, &plugin.Account{ID: 7, Platform: "antigravity"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	notes, err := client.Note.Query().All(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].AccountID != 7 || !strings.Contains(notes[0].Content, "antigravity") {
		t.Fatalf("unexpected note: %+v", notes[0])
	}
}

// TestOnAccountCreatedUnexpectedPayload verifies the subscriber tolerates
// a wrong payload type (returns nil so Notify delivery is not marked
// failed).
func TestOnAccountCreatedUnexpectedPayload(t *testing.T) {
	p := &Plugin{core: newStubCore()}
	if err := p.onAccountCreated(context.Background(), "wrong-type"); err != nil {
		t.Fatalf("handler should swallow bad payload, got: %v", err)
	}
}

// TestExportsLatestNote verifies the cross-plugin export surface returns
// the newest note and correctly reports "absent" with (nil, nil).
func TestExportsLatestNote(t *testing.T) {
	client := openTestClient(t)
	p := &Plugin{core: newStubCore(), client: client}
	p.exports = &demoExports{owner: p, client: client}
	ctx := context.Background()

	note, err := p.exports.LatestNote(ctx, 99)
	if err != nil || note != nil {
		t.Fatalf("expected nil/nil, got note=%+v err=%v", note, err)
	}

	_, _ = client.Note.Create().
		SetAccountID(99).SetContent("old").
		SetCreatedAt(time.Now().Add(-time.Hour)).Save(ctx)
	latest, _ := client.Note.Create().
		SetAccountID(99).SetContent("new").
		SetCreatedAt(time.Now()).Save(ctx)

	note, err = p.exports.LatestNote(ctx, 99)
	if err != nil {
		t.Fatalf("LatestNote: %v", err)
	}
	if note == nil || note.ID != latest.ID || note.Content != "new" {
		t.Fatalf("wrong note returned: %+v", note)
	}
}
