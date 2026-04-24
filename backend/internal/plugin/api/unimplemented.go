package api

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// Unimplemented sub-APIs. Each method returns plugin.ErrNotImplemented (or a
// zero-value snapshot where the signature has no error) so plugins can
// check-and-skip at call time. When a future wave wires one of these, the
// stub is removed and factory.go assigns the real implementation.

type unimplementedUserAPI struct{}

func (unimplementedUserAPI) Find(context.Context, int64) (*plugin.User, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedUserAPI) List(context.Context, plugin.UserFilter) ([]*plugin.User, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedUserAPI) PatchExtra(context.Context, int64, plugin.PatchFunc) error {
	return plugin.ErrNotImplemented
}

type unimplementedOrderAPI struct{}

func (unimplementedOrderAPI) Find(context.Context, string) (*plugin.Order, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedOrderAPI) List(context.Context, plugin.OrderFilter) ([]*plugin.Order, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedOrderAPI) PatchExtra(context.Context, string, plugin.PatchFunc) error {
	return plugin.ErrNotImplemented
}

type unimplementedSubscriptionAPI struct{}

func (unimplementedSubscriptionAPI) Find(context.Context, int64) (*plugin.Subscription, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedSubscriptionAPI) List(context.Context, plugin.SubscriptionFilter) ([]*plugin.Subscription, error) {
	return nil, plugin.ErrNotImplemented
}

type unimplementedRateLimitAPI struct{}

func (unimplementedRateLimitAPI) TryAcquire(context.Context, int64, string) (bool, error) {
	return false, plugin.ErrNotImplemented
}
func (unimplementedRateLimitAPI) MarkLimited(context.Context, int64, string, time.Time) error {
	return plugin.ErrNotImplemented
}
func (unimplementedRateLimitAPI) Remaining(context.Context, int64, string) time.Duration {
	return 0
}

type unimplementedConcurrencyAPI struct{}

func (unimplementedConcurrencyAPI) Acquire(context.Context, string, int) (func(), error) {
	return func() {}, plugin.ErrNotImplemented
}
func (unimplementedConcurrencyAPI) Release(context.Context, string) {}

type unimplementedSchedulerAPI struct{}

func (unimplementedSchedulerAPI) Snapshot(context.Context) (plugin.SchedulerSnapshot, error) {
	return plugin.SchedulerSnapshot{}, plugin.ErrNotImplemented
}

type unimplementedJobQueue struct{}

func (unimplementedJobQueue) Enqueue(context.Context, string, []byte, plugin.JobOptions) error {
	return plugin.ErrNotImplemented
}

type unimplementedI18n struct{}

// T returns key as a best-effort fallback so plugins still render a string
// during Phase 0 even though no translation tables are wired.
func (unimplementedI18n) T(_ string, key string, _ map[string]any) string { return key }

type unimplementedKV struct{}

func (unimplementedKV) Get(context.Context, string) ([]byte, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedKV) Set(context.Context, string, []byte) error {
	return plugin.ErrNotImplemented
}
func (unimplementedKV) Delete(context.Context, string) error {
	return plugin.ErrNotImplemented
}
func (unimplementedKV) Scan(context.Context, string) (map[string][]byte, error) {
	return nil, plugin.ErrNotImplemented
}

type unimplementedEventsLog struct{}

func (unimplementedEventsLog) Append(context.Context, plugin.EventLogEntry) error {
	return plugin.ErrNotImplemented
}
func (unimplementedEventsLog) Query(context.Context, plugin.EventLogFilter) ([]plugin.EventLogEntry, error) {
	return nil, plugin.ErrNotImplemented
}

type unimplementedBus struct{}

func (unimplementedBus) Publish(context.Context, string, any) error {
	return plugin.ErrNotImplemented
}
func (unimplementedBus) PublishSyncHook(context.Context, string, any) error {
	return plugin.ErrNotImplemented
}
func (unimplementedBus) PublishAsyncHook(context.Context, string, any) error {
	return plugin.ErrNotImplemented
}
func (unimplementedBus) PublishNotify(context.Context, string, any) {}
func (unimplementedBus) Subscribe(plugin.EventSubscription) error {
	return plugin.ErrNotImplemented
}

// sdkRegistry resolves peer plugins through the SDK's process-wide registry.
// Keeping the adapter in this file means CoreAPI has no direct dependency on
// the loader package.
type sdkRegistry struct{}

func (sdkRegistry) Lookup(id string) (plugin.Plugin, bool) {
	return plugin.Lookup(id)
}
