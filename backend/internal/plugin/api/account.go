package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// accountAPI is the host-side AccountAPI backed by the existing
// service.AccountService + AccountRepository.
//
// Reads use AccountService (which already centralises pagination and error
// wrapping); writes use the repository directly because the service layer
// does not expose a partial-extra patch helper.
type accountAPI struct {
	guard       *guard
	accountSvc  *service.AccountService
	accountRepo service.AccountRepository
}

// newAccountAPI binds the host services to a new AccountAPI. When either
// dependency is missing the returned instance returns ErrNotImplemented for
// every method, so plugins can optionally tolerate a partially-wired host.
func newAccountAPI(c *coreAPIImpl) plugin.AccountAPI {
	if c.deps.AccountRepo == nil {
		return unimplementedAccountAPI{}
	}
	return &accountAPI{
		guard:       c.guard,
		accountSvc:  c.deps.AccountService,
		accountRepo: c.deps.AccountRepo,
	}
}

// Find returns the account by id. Requires PermAccountRead.
func (a *accountAPI) Find(ctx context.Context, id int64) (*plugin.Account, error) {
	if err := a.guard.requirePerm(plugin.PermAccountRead); err != nil {
		return nil, err
	}
	acc, err := a.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("plugin account find: %w", err)
	}
	return toAccountDTO(acc), nil
}

// List honours the filter, falling back to the richest repository method
// available. Filter.Platform/GroupID are used; Type/Status/Search go
// through ListWithFilters when any of them is set.
func (a *accountAPI) List(ctx context.Context, filter plugin.AccountFilter) ([]*plugin.Account, error) {
	if err := a.guard.requirePerm(plugin.PermAccountRead); err != nil {
		return nil, err
	}
	accounts, err := a.listInternal(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("plugin account list: %w", err)
	}
	return toAccountDTOs(accounts), nil
}

// listInternal selects the narrowest repository call that satisfies filter.
// Kept separate so the guard/DTO conversion in List stays within 30 lines.
func (a *accountAPI) listInternal(ctx context.Context, f plugin.AccountFilter) ([]service.Account, error) {
	needsRich := f.Type != "" || f.Status != "" || f.Search != ""
	if needsRich {
		params := buildPaginationParams(f.Offset, f.Limit)
		items, _, err := a.accountRepo.ListWithFilters(ctx, params, f.Platform, f.Type, f.Status, f.Search, f.GroupID, "")
		return items, err
	}
	if f.GroupID != 0 {
		return a.accountRepo.ListByGroup(ctx, f.GroupID)
	}
	if f.Platform != "" {
		return a.accountRepo.ListByPlatform(ctx, f.Platform)
	}
	params := buildPaginationParams(f.Offset, f.Limit)
	items, _, err := a.accountRepo.List(ctx, params)
	return items, err
}

// PatchExtra loads the account, runs patch on a defensive copy of Extra,
// and persists the result through AccountRepository.UpdateExtra. Requires
// PermAccountWrite.
func (a *accountAPI) PatchExtra(ctx context.Context, id int64, patch plugin.PatchFunc) error {
	if err := a.guard.requirePerm(plugin.PermAccountWrite); err != nil {
		return err
	}
	if patch == nil {
		return errors.New("plugin account patch: patch func is nil")
	}
	acc, err := a.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("plugin account patch load: %w", err)
	}
	updated := patch(cloneMap(acc.Extra))
	if updated == nil {
		updated = map[string]any{}
	}
	if err := a.accountRepo.UpdateExtra(ctx, id, updated); err != nil {
		return fmt.Errorf("plugin account patch persist: %w", err)
	}
	return nil
}

// PatchCredentials behaves like PatchExtra but targets the credentials map.
// Repository has no dedicated helper; we replay through the full update
// path via AccountService when available, otherwise read-modify-write via
// Update.
func (a *accountAPI) PatchCredentials(ctx context.Context, id int64, patch plugin.PatchFunc) error {
	if err := a.guard.requirePerm(plugin.PermAccountWrite); err != nil {
		return err
	}
	if patch == nil {
		return errors.New("plugin account patch: patch func is nil")
	}
	acc, err := a.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("plugin account patch load: %w", err)
	}
	updated := patch(cloneMap(acc.Credentials))
	if updated == nil {
		updated = map[string]any{}
	}
	acc.Credentials = updated
	if err := a.accountRepo.Update(ctx, acc); err != nil {
		return fmt.Errorf("plugin account patch persist: %w", err)
	}
	return nil
}

// buildPaginationParams converts the plugin-facing Offset/Limit pair into
// a service.PaginationParams. A Limit<=0 means "use repository default"
// (the pagination helper coerces it further).
func buildPaginationParams(offset, limit int) pagination.PaginationParams {
	return pagination.PaginationParams{Page: pageFromOffset(offset, limit), PageSize: limit}
}

// pageFromOffset converts an Offset/Limit filter pair into a 1-based page
// index the pagination helper expects. Limit<=0 means the caller accepts
// the repository default.
func pageFromOffset(offset, limit int) int {
	if limit <= 0 {
		return 1
	}
	page := offset/limit + 1
	if page < 1 {
		return 1
	}
	return page
}

// cloneMap returns a shallow copy of src. A nil src yields an empty map so
// patch functions can safely assume a non-nil input.
func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// toAccountDTO materialises the plugin-facing snapshot from a service-layer
// Account. Internal-only fields (modelMappingCache, scheduling internals)
// are dropped; Extra/Credentials maps are passed through by reference, so
// callers MUST copy before mutating.
func toAccountDTO(a *service.Account) *plugin.Account {
	if a == nil {
		return nil
	}
	dto := &plugin.Account{
		ID:             a.ID,
		Name:           a.Name,
		Platform:       a.Platform,
		Type:           a.Type,
		Status:         a.Status,
		Credentials:    a.Credentials,
		Extra:          a.Extra,
		Priority:       a.Priority,
		Concurrency:    a.Concurrency,
		GroupIDs:       append([]int64(nil), a.GroupIDs...),
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
		RateMultiplier: a.RateMultiplier,
		ExpiresAt:      a.ExpiresAt,
		Schedulable:    a.Schedulable,
		ErrorMessage:   a.ErrorMessage,
	}
	if a.Notes != nil {
		dto.Notes = *a.Notes
	}
	if a.ProxyID != nil {
		dto.ProxyID = *a.ProxyID
	}
	return dto
}

// toAccountDTOs maps a slice of service accounts to DTOs in a single pass.
func toAccountDTOs(accounts []service.Account) []*plugin.Account {
	out := make([]*plugin.Account, 0, len(accounts))
	for i := range accounts {
		out = append(out, toAccountDTO(&accounts[i]))
	}
	return out
}

// unimplementedAccountAPI is returned when the host boots without an
// AccountRepository (e.g. partial wire during early boot or tests).
type unimplementedAccountAPI struct{}

func (unimplementedAccountAPI) Find(context.Context, int64) (*plugin.Account, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedAccountAPI) List(context.Context, plugin.AccountFilter) ([]*plugin.Account, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedAccountAPI) PatchExtra(context.Context, int64, plugin.PatchFunc) error {
	return plugin.ErrNotImplemented
}
func (unimplementedAccountAPI) PatchCredentials(context.Context, int64, plugin.PatchFunc) error {
	return plugin.ErrNotImplemented
}
