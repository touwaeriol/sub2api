package eventbus

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// Bus is the concrete implementation of [plugin.EventBus]. It owns one
// dispatcher per EventKind and routes calls based on the topic's registered
// schema.
type Bus struct {
	registry *Registry
	sync     *SyncDispatcher
	async    *AsyncDispatcher
	notify   *NotifyDispatcher
}

// BusOptions bundle optional settings for [NewBus].
type BusOptions struct {
	// Registry is the schema catalogue. If nil, a fresh registry is created
	// with only the core topics registered.
	Registry *Registry
	// JobQueue backs the AsyncDispatcher. Required for async hooks.
	JobQueue JobEnqueuer
	// HandlerRegistry is typically the same object as JobQueue (both
	// methods on the same concrete type, e.g. InMemoryJobQueue).
	HandlerRegistry HandlerRegistry
	// DeadLetterRepo stores entries for async hooks that exhausted retries.
	DeadLetterRepo DeadLetterRepo
}

// NewBus wires the sub-dispatchers. Returns an error only if the registry
// cannot load the built-in core schemas.
func NewBus(opts BusOptions) (*Bus, error) {
	registry := opts.Registry
	if registry == nil {
		registry = NewRegistry()
		if err := RegisterCoreSchemas(registry); err != nil {
			return nil, fmt.Errorf("eventbus: register core schemas: %w", err)
		}
	}
	b := &Bus{
		registry: registry,
		sync:     NewSyncDispatcher(),
		notify:   NewNotifyDispatcher(),
	}
	if opts.JobQueue != nil && opts.HandlerRegistry != nil && opts.DeadLetterRepo != nil {
		b.async = NewAsyncDispatcher(registry, opts.JobQueue, opts.HandlerRegistry, opts.DeadLetterRepo)
	}
	return b, nil
}

// Registry exposes the underlying schema catalogue (for admin tooling).
func (b *Bus) Registry() *Registry { return b.registry }

// SyncDispatcher exposes the sync dispatcher (tests / advanced wiring).
func (b *Bus) SyncDispatcher() *SyncDispatcher { return b.sync }

// AsyncDispatcher returns nil when the bus was constructed without async
// infrastructure (tests that only exercise sync/notify).
func (b *Bus) AsyncDispatcher() *AsyncDispatcher { return b.async }

// NotifyDispatcher exposes the notify dispatcher.
func (b *Bus) NotifyDispatcher() *NotifyDispatcher { return b.notify }

// Start brings up background workers (currently only the async
// dispatcher's queue handler). Safe to call multiple times; the async
// dispatcher itself guards against double registration. Returns an error
// when async wiring is present but the queue rejects the handler —
// callers typically log and proceed because sync/notify still work.
func (b *Bus) Start(ctx context.Context) error {
	if b.async == nil {
		return nil
	}
	return b.async.Start(ctx)
}

// Stop is the symmetric teardown hook. The underlying in-memory job queue
// owns worker lifecycle so this is intentionally a no-op today; kept for
// forward compatibility with a SQL-backed queue.
func (b *Bus) Stop(ctx context.Context) error {
	if b.async == nil {
		return nil
	}
	return b.async.Stop(ctx)
}

// Publish routes to the dispatcher matching the topic's Kind.
func (b *Bus) Publish(ctx context.Context, topic string, payload any) error {
	schema, ok := b.registry.Get(topic)
	if !ok {
		return fmt.Errorf("%w: %s", plugin.ErrEventTopicUnknown, topic)
	}
	if !payloadMatchesSchema(payload, schema.PayloadExample) {
		return fmt.Errorf("%w: topic=%s", plugin.ErrEventHandlerSignature, topic)
	}
	switch schema.Kind {
	case plugin.EventKindSyncHook:
		return b.sync.Dispatch(ctx, topic, payload)
	case plugin.EventKindAsyncHook:
		return b.publishAsync(ctx, topic, payload)
	case plugin.EventKindNotify:
		b.notify.Dispatch(ctx, topic, payload)
		return nil
	default:
		return fmt.Errorf("%w: unknown kind %d", plugin.ErrEventSchemaInvalid, schema.Kind)
	}
}

// PublishSyncHook is the explicit entry point for SyncHook topics.
func (b *Bus) PublishSyncHook(ctx context.Context, topic string, payload any) error {
	if err := b.expectKind(topic, plugin.EventKindSyncHook, payload); err != nil {
		return err
	}
	return b.sync.Dispatch(ctx, topic, payload)
}

// PublishAsyncHook is the explicit entry point for AsyncHook topics.
func (b *Bus) PublishAsyncHook(ctx context.Context, topic string, payload any) error {
	if err := b.expectKind(topic, plugin.EventKindAsyncHook, payload); err != nil {
		return err
	}
	return b.publishAsync(ctx, topic, payload)
}

// PublishNotify is the explicit entry point for Notify topics. Errors are
// logged (not returned) to match the fire-and-forget contract.
func (b *Bus) PublishNotify(ctx context.Context, topic string, payload any) {
	if err := b.expectKind(topic, plugin.EventKindNotify, payload); err != nil {
		// Intentional: Notify callers don't check errors. Log and move on.
		logInvalidNotify(topic, err)
		return
	}
	b.notify.Dispatch(ctx, topic, payload)
}

// Subscribe validates the subscription and routes it to the right dispatcher.
func (b *Bus) Subscribe(sub plugin.EventSubscription) error {
	schema, ok := b.registry.Get(sub.Topic)
	if !ok {
		return fmt.Errorf("%w: %s", plugin.ErrEventTopicUnknown, sub.Topic)
	}
	if sub.Kind != schema.Kind {
		return fmt.Errorf("%w: topic=%s want=%s got=%s",
			plugin.ErrEventKindMismatch, sub.Topic, schema.Kind, sub.Kind)
	}
	if sub.Handler == nil {
		return fmt.Errorf("%w: topic=%s handler is nil",
			plugin.ErrEventHandlerSignature, sub.Topic)
	}
	b.routeSubscription(sub)
	return nil
}

func (b *Bus) routeSubscription(sub plugin.EventSubscription) {
	switch sub.Kind {
	case plugin.EventKindSyncHook:
		b.sync.Register(sub)
	case plugin.EventKindAsyncHook:
		if b.async != nil {
			b.async.Register(sub)
		}
	case plugin.EventKindNotify:
		b.notify.Register(sub)
	}
}

// expectKind verifies the topic is registered with the expected Kind and the
// payload type matches. Shared validation for the three explicit publishers.
func (b *Bus) expectKind(topic string, want plugin.EventKind, payload any) error {
	schema, ok := b.registry.Get(topic)
	if !ok {
		return fmt.Errorf("%w: %s", plugin.ErrEventTopicUnknown, topic)
	}
	if schema.Kind != want {
		return fmt.Errorf("%w: topic=%s schema=%s requested=%s",
			plugin.ErrEventKindMismatch, topic, schema.Kind, want)
	}
	if !payloadMatchesSchema(payload, schema.PayloadExample) {
		return fmt.Errorf("%w: topic=%s", plugin.ErrEventHandlerSignature, topic)
	}
	return nil
}

func (b *Bus) publishAsync(ctx context.Context, topic string, payload any) error {
	if b.async == nil {
		return fmt.Errorf("eventbus: async dispatcher not configured (topic=%s)", topic)
	}
	return b.async.Publish(ctx, topic, payload)
}

// logInvalidNotify is split out so the signature of PublishNotify stays flat.
func logInvalidNotify(topic string, err error) {
	slogErrorf("eventbus: invalid notify publish ignored", "topic", topic, "error", err)
}
