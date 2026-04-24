package eventbus

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// defaultNotifyTimeout bounds each Notify handler so a slow subscriber does
// not leak goroutines indefinitely.
const defaultNotifyTimeout = 10 * time.Second

// NotifyDispatcher fans a Notify event out across all subscribers in their
// own goroutines. Errors and panics are recovered and logged; nothing is
// retried and nothing is persisted.
type NotifyDispatcher struct {
	mu   sync.RWMutex
	subs map[string][]plugin.EventSubscription
}

// NewNotifyDispatcher returns an empty dispatcher.
func NewNotifyDispatcher() *NotifyDispatcher {
	return &NotifyDispatcher{subs: make(map[string][]plugin.EventSubscription)}
}

// Register adds a subscription.
func (d *NotifyDispatcher) Register(sub plugin.EventSubscription) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.subs[sub.Topic] = append(d.subs[sub.Topic], sub)
}

// Dispatch fires one goroutine per subscriber. Returns immediately.
func (d *NotifyDispatcher) Dispatch(_ context.Context, topic string, payload any) {
	d.mu.RLock()
	subs := cloneSubs(d.subs[topic])
	d.mu.RUnlock()

	for _, sub := range subs {
		go d.runSubscriber(sub, topic, payload)
	}
}

// runSubscriber wraps a single handler call in a timeout + panic recovery.
func (d *NotifyDispatcher) runSubscriber(sub plugin.EventSubscription, topic string, payload any) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("eventbus: notify handler panic",
				"topic", topic,
				"subscriber_tag", sub.SubscriberTag,
				"panic", r,
			)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), defaultNotifyTimeout)
	defer cancel()
	if err := sub.Handler(ctx, payload); err != nil {
		slog.Warn("eventbus: notify handler returned error (dropped)",
			"topic", topic,
			"subscriber_tag", sub.SubscriberTag,
			"error", err,
		)
	}
}
