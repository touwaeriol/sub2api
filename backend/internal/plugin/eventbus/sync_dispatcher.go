package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// defaultSyncHandlerTimeout bounds each SyncHook handler. Tunable via
// SyncDispatcher.SetHandlerTimeout.
const defaultSyncHandlerTimeout = 5 * time.Second

// SyncDispatcher runs SyncHook subscribers inline, in priority order,
// aborting on the first error.
type SyncDispatcher struct {
	mu             sync.RWMutex
	subs           map[string][]plugin.EventSubscription
	handlerTimeout time.Duration
}

// NewSyncDispatcher returns a dispatcher with default settings.
func NewSyncDispatcher() *SyncDispatcher {
	return &SyncDispatcher{
		subs:           make(map[string][]plugin.EventSubscription),
		handlerTimeout: defaultSyncHandlerTimeout,
	}
}

// SetHandlerTimeout overrides the per-handler deadline.
func (d *SyncDispatcher) SetHandlerTimeout(dur time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if dur > 0 {
		d.handlerTimeout = dur
	}
}

// Register adds a subscription. The list is kept sorted by Priority (asc).
func (d *SyncDispatcher) Register(sub plugin.EventSubscription) {
	d.mu.Lock()
	defer d.mu.Unlock()
	list := append(d.subs[sub.Topic], sub)
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Priority < list[j].Priority
	})
	d.subs[sub.Topic] = list
}

// Dispatch invokes each subscriber sequentially. The first handler error is
// returned to the publisher (which typically rolls back its transaction).
func (d *SyncDispatcher) Dispatch(ctx context.Context, topic string, payload any) error {
	d.mu.RLock()
	subs := cloneSubs(d.subs[topic])
	timeout := d.handlerTimeout
	d.mu.RUnlock()

	for _, sub := range subs {
		if err := d.runOne(ctx, sub, payload, timeout); err != nil {
			return err
		}
	}
	return nil
}

// runOne wraps a single handler call in a timeout and logs context on error.
func (d *SyncDispatcher) runOne(ctx context.Context, sub plugin.EventSubscription, payload any, timeout time.Duration) error {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := sub.Handler(cctx, payload)
	if err == nil {
		return nil
	}
	slog.Warn("eventbus: sync hook handler failed",
		"topic", sub.Topic,
		"subscriber_tag", sub.SubscriberTag,
		"priority", sub.Priority,
		"error", err,
	)
	return fmt.Errorf("eventbus: sync hook %q (subscriber=%s) failed: %w",
		sub.Topic, sub.SubscriberTag, err)
}

func cloneSubs(in []plugin.EventSubscription) []plugin.EventSubscription {
	out := make([]plugin.EventSubscription, len(in))
	copy(out, in)
	return out
}
