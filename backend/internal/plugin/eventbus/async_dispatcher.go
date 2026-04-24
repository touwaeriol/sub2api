package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// AsyncDispatcher settings. Values are conservative defaults; host wiring
// may override via setters.
const (
	asyncQueueName        = "plugin-eventbus-async"
	defaultMaxAttempts    = 5
	defaultInitialBackoff = 1 * time.Second
	defaultMaxBackoff     = 60 * time.Second
	backoffFactor         = 2
)

// asyncEnvelope is the JSON-serialised job payload the dispatcher puts on
// the queue. Carries enough context for retries and dead-letter rows.
type asyncEnvelope struct {
	Topic         string          `json:"topic"`
	Payload       json.RawMessage `json:"payload"`
	AttemptCount  int             `json:"attempt_count"`
	FirstFailedAt time.Time       `json:"first_failed_at,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
}

// AsyncDispatcher delivers AsyncHook events with retry + dead-letter.
type AsyncDispatcher struct {
	mu             sync.RWMutex
	registry       *Registry
	queue          JobEnqueuer
	handlers       HandlerRegistry
	deadLetter     DeadLetterRepo
	subs           map[string][]plugin.EventSubscription
	maxAttempts    int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	started        bool
}

// NewAsyncDispatcher constructs a dispatcher. The queue and dead-letter repo
// are both required. handlers is typically the same object as queue when the
// concrete implementation embeds both (e.g. InMemoryJobQueue).
func NewAsyncDispatcher(registry *Registry, queue JobEnqueuer, handlers HandlerRegistry, dl DeadLetterRepo) *AsyncDispatcher {
	return &AsyncDispatcher{
		registry:       registry,
		queue:          queue,
		handlers:       handlers,
		deadLetter:     dl,
		subs:           make(map[string][]plugin.EventSubscription),
		maxAttempts:    defaultMaxAttempts,
		initialBackoff: defaultInitialBackoff,
		maxBackoff:     defaultMaxBackoff,
	}
}

// SetRetryPolicy overrides the default retry parameters.
func (d *AsyncDispatcher) SetRetryPolicy(maxAttempts int, initial, max time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if maxAttempts > 0 {
		d.maxAttempts = maxAttempts
	}
	if initial > 0 {
		d.initialBackoff = initial
	}
	if max > 0 {
		d.maxBackoff = max
	}
}

// Register adds a subscription.
func (d *AsyncDispatcher) Register(sub plugin.EventSubscription) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.subs[sub.Topic] = append(d.subs[sub.Topic], sub)
}

// Start registers the queue handler. Must be called once before Publish.
func (d *AsyncDispatcher) Start(_ context.Context) error {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return nil
	}
	d.started = true
	d.mu.Unlock()

	if err := d.handlers.RegisterHandler(asyncQueueName, d.consume); err != nil {
		return fmt.Errorf("eventbus: register async handler: %w", err)
	}
	return nil
}

// Stop is a no-op placeholder kept for symmetry; the underlying queue owns
// worker lifecycle. Host code should stop the queue itself.
func (d *AsyncDispatcher) Stop(_ context.Context) error { return nil }

// Publish enqueues the initial delivery attempt.
func (d *AsyncDispatcher) Publish(ctx context.Context, topic string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("eventbus: marshal async payload topic=%s: %w", topic, err)
	}
	env := asyncEnvelope{
		Topic:        topic,
		Payload:      raw,
		AttemptCount: 0,
	}
	return d.enqueueEnvelope(ctx, env, 0)
}

func (d *AsyncDispatcher) enqueueEnvelope(ctx context.Context, env asyncEnvelope, delay time.Duration) error {
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("eventbus: marshal envelope: %w", err)
	}
	opts := EnqueueOptions{Delay: delay, MaxAttempts: d.maxAttempts}
	if err := d.queue.Enqueue(ctx, asyncQueueName, data, opts); err != nil {
		return fmt.Errorf("eventbus: enqueue async job: %w", err)
	}
	return nil
}

// consume is invoked by the job queue for each dequeued envelope. It runs
// every subscriber; any subscriber error triggers a retry (or dead-letter).
func (d *AsyncDispatcher) consume(ctx context.Context, raw []byte) error {
	var env asyncEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		slog.Error("eventbus: decode async envelope", "error", err)
		return nil // invalid envelope can't retry productively
	}

	payload, payloadErr := d.decodePayload(env)
	if payloadErr != nil {
		slog.Error("eventbus: decode async payload",
			"topic", env.Topic, "error", payloadErr)
		return nil
	}

	d.mu.RLock()
	subs := cloneSubs(d.subs[env.Topic])
	d.mu.RUnlock()

	for _, sub := range subs {
		d.runSubscriber(ctx, env, payload, sub)
	}
	return nil
}

func (d *AsyncDispatcher) decodePayload(env asyncEnvelope) (any, error) {
	schema, ok := d.registry.Get(env.Topic)
	if !ok {
		return nil, fmt.Errorf("%w: %s", plugin.ErrEventTopicUnknown, env.Topic)
	}
	return decodeIntoPayloadType(schema.PayloadExample, env.Payload)
}

// runSubscriber invokes one handler and handles retry/dead-letter on error.
// Split into its own function to keep the loop body <= 30 lines.
func (d *AsyncDispatcher) runSubscriber(ctx context.Context, env asyncEnvelope, payload any, sub plugin.EventSubscription) {
	cctx, cancel := context.WithTimeout(ctx, defaultJobTimeout)
	defer cancel()

	err := sub.Handler(cctx, payload)
	if err == nil {
		return
	}
	d.handleSubscriberFailure(ctx, env, sub, err)
}

func (d *AsyncDispatcher) handleSubscriberFailure(ctx context.Context, env asyncEnvelope, sub plugin.EventSubscription, cause error) {
	nextAttempt := env.AttemptCount + 1
	slog.Warn("eventbus: async hook handler failed",
		"topic", env.Topic,
		"subscriber_tag", sub.SubscriberTag,
		"attempt", nextAttempt,
		"error", cause,
	)

	if nextAttempt >= d.maxAttempts {
		d.recordDeadLetter(ctx, env, sub, cause)
		return
	}
	d.scheduleRetry(ctx, env, sub, cause, nextAttempt)
}

func (d *AsyncDispatcher) scheduleRetry(ctx context.Context, env asyncEnvelope, sub plugin.EventSubscription, cause error, nextAttempt int) {
	delay := backoffFor(nextAttempt, d.initialBackoff, d.maxBackoff)
	retry := env
	retry.AttemptCount = nextAttempt
	if retry.FirstFailedAt.IsZero() {
		retry.FirstFailedAt = time.Now()
	}
	if err := d.enqueueEnvelope(ctx, retry, delay); err != nil {
		slog.Error("eventbus: schedule retry failed; falling back to dead letter",
			"topic", env.Topic, "error", err, "cause", cause)
		d.recordDeadLetter(ctx, retry, sub, cause)
	}
}

func (d *AsyncDispatcher) recordDeadLetter(ctx context.Context, env asyncEnvelope, sub plugin.EventSubscription, cause error) {
	firstFailed := env.FirstFailedAt
	if firstFailed.IsZero() {
		firstFailed = time.Now()
	}
	entry := DeadLetterEntry{
		Topic:         env.Topic,
		Payload:       env.Payload,
		FirstFailedAt: firstFailed,
		LastAttemptAt: time.Now(),
		AttemptCount:  env.AttemptCount + 1,
		LastError:     cause.Error(),
		SubscriberTag: sub.SubscriberTag,
		CorrelationID: env.CorrelationID,
	}
	if err := d.deadLetter.Record(ctx, entry); err != nil {
		slog.Error("eventbus: persist dead letter failed",
			"topic", env.Topic, "error", err, "cause", cause)
	}
}

// backoffFor returns the exponential backoff duration for the given attempt
// index (1-based). Capped at maxBackoff.
func backoffFor(attempt int, initial, max time.Duration) time.Duration {
	d := initial
	for i := 1; i < attempt; i++ {
		d *= backoffFactor
		if d >= max {
			return max
		}
	}
	if d > max {
		return max
	}
	return d
}
