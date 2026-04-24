//go:build unit

package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// stubAccountRepo is a hand-rolled AccountRepository that only implements
// the methods accountAPI touches. Unused methods panic so the test surface
// stays tight — if a future code path calls through, the panic gives us a
// clear signal rather than a nil crash.
type stubAccountRepo struct {
	service.AccountRepository // embed nil to inherit method set; unused methods panic via override below

	getByID    func(ctx context.Context, id int64) (*service.Account, error)
	update     func(ctx context.Context, acc *service.Account) error
	updateX    func(ctx context.Context, id int64, updates map[string]any) error
	listByPlat func(ctx context.Context, platform string) ([]service.Account, error)
	listByGrp  func(ctx context.Context, groupID int64) ([]service.Account, error)
}

func (s *stubAccountRepo) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	return s.getByID(ctx, id)
}
func (s *stubAccountRepo) Update(ctx context.Context, acc *service.Account) error {
	return s.update(ctx, acc)
}
func (s *stubAccountRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return s.updateX(ctx, id, updates)
}
func (s *stubAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return s.listByPlat(ctx, platform)
}
func (s *stubAccountRepo) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	return s.listByGrp(ctx, groupID)
}

// ensure the embedded nil never executes: override unused methods to panic
// so callers notice at runtime.
func (s *stubAccountRepo) List(context.Context, pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	panic("List should not be called in these tests")
}

func newTestAccountAPI(t *testing.T, repo service.AccountRepository, perms []plugin.Permission) plugin.AccountAPI {
	t.Helper()
	factory := NewCoreAPIFactory(Dependencies{
		AccountRepo: repo,
	})
	core := factory.For("test-plugin", perms)
	return core.Accounts()
}

func TestAccountAPI_Find_DeniesWithoutPerm(t *testing.T) {
	api := newTestAccountAPI(t, &stubAccountRepo{}, nil)
	_, err := api.Find(context.Background(), 1)
	if !errors.Is(err, plugin.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAccountAPI_Find_ReturnsDTO(t *testing.T) {
	repo := &stubAccountRepo{
		getByID: func(_ context.Context, id int64) (*service.Account, error) {
			notes := "owner note"
			proxy := int64(42)
			return &service.Account{
				ID:       id,
				Name:     "acc",
				Platform: "anthropic",
				Notes:    &notes,
				ProxyID:  &proxy,
				Extra:    map[string]any{"k": "v"},
			}, nil
		},
	}
	api := newTestAccountAPI(t, repo, []plugin.Permission{plugin.PermAccountRead})
	acc, err := api.Find(context.Background(), 7)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if acc.ID != 7 || acc.Name != "acc" || acc.Notes != "owner note" || acc.ProxyID != 42 {
		t.Fatalf("DTO unexpected: %+v", acc)
	}
	if acc.Extra["k"] != "v" {
		t.Fatalf("Extra not propagated: %+v", acc.Extra)
	}
}

func TestAccountAPI_PatchExtra_ReadModifyWrite(t *testing.T) {
	var persisted map[string]any
	repo := &stubAccountRepo{
		getByID: func(_ context.Context, _ int64) (*service.Account, error) {
			return &service.Account{ID: 1, Extra: map[string]any{"existing": 1}}, nil
		},
		updateX: func(_ context.Context, _ int64, updates map[string]any) error {
			persisted = updates
			return nil
		},
	}
	api := newTestAccountAPI(t, repo, []plugin.Permission{plugin.PermAccountWrite})
	err := api.PatchExtra(context.Background(), 1, func(cur map[string]any) map[string]any {
		cur["added"] = "yes"
		return cur
	})
	if err != nil {
		t.Fatalf("PatchExtra: %v", err)
	}
	if persisted["existing"] != 1 || persisted["added"] != "yes" {
		t.Fatalf("persisted map not merged correctly: %+v", persisted)
	}
}

func TestAccountAPI_PatchExtra_DeniesWithoutWrite(t *testing.T) {
	api := newTestAccountAPI(t, &stubAccountRepo{}, []plugin.Permission{plugin.PermAccountRead})
	err := api.PatchExtra(context.Background(), 1, func(m map[string]any) map[string]any { return m })
	if !errors.Is(err, plugin.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAccountAPI_PatchExtra_NilPatchReturnsError(t *testing.T) {
	api := newTestAccountAPI(t, &stubAccountRepo{
		getByID: func(_ context.Context, _ int64) (*service.Account, error) {
			return &service.Account{ID: 1}, nil
		},
	}, []plugin.Permission{plugin.PermAccountWrite})
	if err := api.PatchExtra(context.Background(), 1, nil); err == nil {
		t.Fatal("expected error on nil patch")
	}
}

func TestAccountAPI_List_RoutesByPlatform(t *testing.T) {
	called := 0
	repo := &stubAccountRepo{
		listByPlat: func(_ context.Context, platform string) ([]service.Account, error) {
			called++
			if platform != "openai" {
				t.Fatalf("unexpected platform: %s", platform)
			}
			return []service.Account{{ID: 1, Name: "o1"}}, nil
		},
	}
	api := newTestAccountAPI(t, repo, []plugin.Permission{plugin.PermAccountRead})
	items, err := api.List(context.Background(), plugin.AccountFilter{Platform: "openai"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if called != 1 || len(items) != 1 || items[0].ID != 1 {
		t.Fatalf("List did not route correctly: called=%d items=%+v", called, items)
	}
}

func TestAccountAPI_List_RoutesByGroup(t *testing.T) {
	called := 0
	repo := &stubAccountRepo{
		listByGrp: func(_ context.Context, groupID int64) ([]service.Account, error) {
			called++
			if groupID != 17 {
				t.Fatalf("unexpected group: %d", groupID)
			}
			return nil, nil
		},
	}
	api := newTestAccountAPI(t, repo, []plugin.Permission{plugin.PermAccountRead})
	_, err := api.List(context.Background(), plugin.AccountFilter{GroupID: 17})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if called != 1 {
		t.Fatalf("ListByGroup not invoked")
	}
}

// Ensure time fields round-trip through the DTO.
func TestAccountDTO_PreservesTimestamps(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	src := &service.Account{ID: 1, CreatedAt: now, UpdatedAt: now, ExpiresAt: &now}
	dto := toAccountDTO(src)
	if !dto.CreatedAt.Equal(now) || !dto.UpdatedAt.Equal(now) || dto.ExpiresAt == nil || !dto.ExpiresAt.Equal(now) {
		t.Fatalf("timestamps lost: %+v", dto)
	}
}
