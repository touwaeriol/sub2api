package plugin

import "context"

// EventKind classifies an event topic's delivery semantics. The publisher
// picks the kind when declaring the topic; subscribers MUST match.
type EventKind int

// EventKind values.
const (
	// EventKindSyncHook — the publisher runs all handlers synchronously
	// inside its transaction. Any handler returning non-nil aborts the
	// publish and rolls back the surrounding tx. Use for "veto" points
	// (e.g. before_delete, before_forward).
	EventKindSyncHook EventKind = iota

	// EventKindAsyncHook — enqueued for reliable delivery. Handlers retry
	// on failure and dead-letter after exhaustion. Use for side-effects
	// that MUST eventually complete (billing, external syncs).
	EventKindAsyncHook

	// EventKindNotify — best-effort fire-and-forget notification. No
	// retries, no durable queue. Use for metrics and UI-push fan-out.
	EventKindNotify
)

// String returns a stable label for the kind, used in logs and errors.
func (k EventKind) String() string {
	switch k {
	case EventKindSyncHook:
		return "sync_hook"
	case EventKindAsyncHook:
		return "async_hook"
	case EventKindNotify:
		return "notify"
	default:
		return "unknown"
	}
}

// EventSchema describes the payload shape of a topic. The event bus uses it
// to validate payloads at publish time.
type EventSchema struct {
	// Topic is the fully-qualified event name.
	Topic string
	// Kind is the delivery class this topic adheres to.
	Kind EventKind
	// PayloadExample is a zero-value of the payload struct; the bus checks
	// publish payloads against its type via reflection.
	PayloadExample any
	// Description documents what triggers the event.
	Description string
}

// EventDecl is the publisher-side manifest entry: a topic plus its schema.
// Listed in Meta.Publishes.
type EventDecl = EventSchema

// EventSubscription is a subscriber-side handler registration listed in
// Meta.Subscribes. The event bus rejects subscriptions whose Kind disagrees
// with the publisher's declared [EventKind].
type EventSubscription struct {
	// Topic is the event to listen to.
	Topic string
	// Kind must match the publisher's [EventSchema.Kind]. Mismatch causes
	// registration to fail with [ErrEventKindMismatch] — the mismatch is
	// always a programming error.
	Kind EventKind
	// Priority orders SyncHook handlers (lower runs earlier). Ignored for
	// AsyncHook and Notify kinds. Default 0; ties are broken by insertion
	// order (not guaranteed stable across restarts).
	Priority int
	// SubscriberTag is an optional identifier (usually plugin id + handler
	// name) surfaced in logs and dead-letter rows; helps pinpoint which
	// subscriber failed without reflecting on function pointers.
	SubscriberTag string
	// Handler receives the event payload. For SyncHook events, returning
	// an error cancels the publish. For AsyncHook events, a non-nil error
	// triggers retry. For Notify events, errors are logged and dropped.
	Handler func(ctx context.Context, payload any) error
}

// EventBus is the contract the host implementation exposes to plugins via
// CoreAPI. The concrete implementation lives outside this SDK.
//
// Implementations route by the topic's declared [EventKind]. The three
// dedicated PublishX methods make the call-site's intent explicit; the
// generic [EventBus.Publish] dispatches based on the schema and exists for
// legacy callers.
type EventBus interface {
	// Publish dispatches an event. The effective behaviour (sync, async
	// queued, fire-and-forget) comes from the topic's declared Kind.
	// For SyncHook topics, Publish runs handlers inline and returns the
	// first handler error. For AsyncHook/Notify topics, Publish returns
	// quickly after enqueueing / scheduling.
	Publish(ctx context.Context, topic string, payload any) error

	// PublishSyncHook dispatches a SyncHook event. Returns the first
	// handler error so the caller can roll back the surrounding tx.
	// Errors when the topic is not SyncHook.
	PublishSyncHook(ctx context.Context, topic string, payload any) error

	// PublishAsyncHook enqueues an AsyncHook event and returns once the
	// payload is persisted (or accepted by the queue). Errors when the
	// topic is not AsyncHook.
	PublishAsyncHook(ctx context.Context, topic string, payload any) error

	// PublishNotify fans out a Notify event. Non-blocking; delivery is
	// best-effort.
	PublishNotify(ctx context.Context, topic string, payload any)

	// Subscribe registers a handler. Normally called by the host from
	// Meta.Subscribes during plugin Init; exposed here for plugins that
	// want to subscribe dynamically.
	Subscribe(sub EventSubscription) error
}

// syncHookTxKey is the context key the bus uses to expose the enclosing
// *sql.Tx to SyncHook subscribers. Use [WithSyncHookTx] and [SyncHookTxFrom]
// rather than reading the key directly.
type syncHookTxKey struct{}

// WithSyncHookTx returns ctx enriched with the given transaction handle.
// Publishers of SyncHook events call this so subscribers can execute side-
// effect writes in the same transaction.
//
// tx is typed as any so this package remains free of a database/sql
// dependency; subscribers cast to *sql.Tx (or their preferred wrapper) on
// retrieval.
func WithSyncHookTx(ctx context.Context, tx any) context.Context {
	return context.WithValue(ctx, syncHookTxKey{}, tx)
}

// SyncHookTxFrom retrieves the transaction handle previously attached via
// [WithSyncHookTx]. Second return is false when no tx is present (the event
// is running outside a transaction).
func SyncHookTxFrom(ctx context.Context) (any, bool) {
	v := ctx.Value(syncHookTxKey{})
	if v == nil {
		return nil, false
	}
	return v, true
}

// Built-in event topics. These are the contract the host offers to plugins.
// Payload shapes are defined in this package (types.go) so plugins can type-
// assert without importing internal.
const (
	// TopicAccountBeforeDelete — SyncHook. Payload: *Account.
	// Plugins clean up side-tables tied to the account; return an error to
	// veto the delete (caller should surface "cannot delete: ...").
	TopicAccountBeforeDelete = "account.before_delete"

	// TopicAccountCreated — Notify. Payload: *Account.
	TopicAccountCreated = "account.created"

	// TopicAccountStateChanged — Notify. Payload: AccountStateChanged.
	TopicAccountStateChanged = "account.state_changed"

	// TopicRequestBeforeForward — SyncHook. Payload: *ForwardRequest.
	// Plugins may mutate the request in-place (rewrite headers, pick model)
	// or veto with an error.
	TopicRequestBeforeForward = "request.before_forward"

	// TopicRequestAfterForward — Notify. Payload: *ForwardResult.
	// Used for usage reporting / audit.
	TopicRequestAfterForward = "request.after_forward"

	// TopicOrderPaid — SyncHook. Payload: OrderEvent.
	// Runs inside the payment transaction; return an error to fail the
	// "mark paid" step and trigger refund.
	TopicOrderPaid = "order.paid"

	// TopicOrderFulfilled — Notify. Payload: OrderEvent.
	TopicOrderFulfilled = "order.fulfilled"

	// TopicPluginSettingsChanged — Notify. Payload: SettingsChanged.
	TopicPluginSettingsChanged = "plugin.settings_changed"
)
