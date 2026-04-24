package eventbus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// EnqueueOptions tune an async job submission.
type EnqueueOptions struct {
	// Delay pushes the earliest execution time forward. Zero runs ASAP.
	Delay time.Duration
	// MaxAttempts caps total tries (initial + retries). Zero means use the
	// dispatcher's default (see defaultMaxAttempts).
	MaxAttempts int
}

// JobHandler consumes a single job. Returning an error schedules a retry
// (subject to MaxAttempts and the caller's backoff strategy).
type JobHandler func(ctx context.Context, payload []byte) error

// JobEnqueuer is the minimal contract the AsyncDispatcher needs. Production
// wiring will adapt the host's Redis-backed job queue; tests use
// [InMemoryJobQueue].
//
// name is the logical queue / topic label; payload is already serialised.
type JobEnqueuer interface {
	Enqueue(ctx context.Context, name string, payload []byte, opts EnqueueOptions) error
}

// HandlerRegistry lets the dispatcher install a handler under a queue name.
// This is decoupled from [JobEnqueuer] so real production queues that
// register handlers elsewhere can still satisfy [JobEnqueuer] cleanly.
type HandlerRegistry interface {
	RegisterHandler(name string, handler JobHandler) error
}

// ErrQueueClosed is returned by InMemoryJobQueue.Enqueue after Stop.
var ErrQueueClosed = errors.New("eventbus: job queue closed")

// InMemoryJobQueue is a goroutine-backed JobEnqueuer + HandlerRegistry for
// tests and Phase 0 local runs.
//
// Limitations (documented here so nobody treats this as production-ready):
//   - payload is kept only in process memory; restarts lose queued jobs.
//   - retries are performed by the dispatcher re-enqueuing; the queue itself
//     has no persistence guarantee.
//   - Delay is implemented with a timer goroutine; it is approximate and
//     will not survive process death.
//   - A single handler per queue name. Multiple workers consume the same
//     channel — throughput scales with workerCount at construction time.
type InMemoryJobQueue struct {
	mu       sync.RWMutex
	closed   bool
	workers  int
	channels map[string]chan inMemoryJob
	handlers map[string]JobHandler
	wg       sync.WaitGroup
	stopCh   chan struct{}
}

type inMemoryJob struct {
	payload []byte
	runAt   time.Time
}

// NewInMemoryJobQueue returns a queue with the given worker pool size per
// topic. Zero or negative defaults to 1 worker.
func NewInMemoryJobQueue(workers int) *InMemoryJobQueue {
	if workers <= 0 {
		workers = 1
	}
	return &InMemoryJobQueue{
		workers:  workers,
		channels: make(map[string]chan inMemoryJob),
		handlers: make(map[string]JobHandler),
		stopCh:   make(chan struct{}),
	}
}

// Enqueue submits a payload. Delay is honoured by sleeping in the caller's
// goroutine when <= 0 it is dispatched immediately.
func (q *InMemoryJobQueue) Enqueue(_ context.Context, name string, payload []byte, opts EnqueueOptions) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return ErrQueueClosed
	}
	ch, ok := q.channels[name]
	q.mu.RUnlock()
	if !ok {
		return fmt.Errorf("eventbus: no handler registered for queue=%s", name)
	}

	job := inMemoryJob{payload: payload, runAt: time.Now().Add(opts.Delay)}
	if opts.Delay <= 0 {
		ch <- job
		return nil
	}
	// Schedule via a goroutine so the caller returns immediately.
	go func() {
		timer := time.NewTimer(opts.Delay)
		defer timer.Stop()
		select {
		case <-timer.C:
			q.dispatchDelayed(name, job)
		case <-q.stopCh:
		}
	}()
	return nil
}

func (q *InMemoryJobQueue) dispatchDelayed(name string, job inMemoryJob) {
	q.mu.RLock()
	ch, ok := q.channels[name]
	closed := q.closed
	q.mu.RUnlock()
	if closed || !ok {
		return
	}
	select {
	case ch <- job:
	case <-q.stopCh:
	}
}

// RegisterHandler installs a handler and spins up the worker pool.
func (q *InMemoryJobQueue) RegisterHandler(name string, handler JobHandler) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrQueueClosed
	}
	if _, exists := q.handlers[name]; exists {
		return fmt.Errorf("eventbus: handler already registered for queue=%s", name)
	}
	const bufferSize = 256
	ch := make(chan inMemoryJob, bufferSize)
	q.channels[name] = ch
	q.handlers[name] = handler
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.runWorker(name, ch, handler)
	}
	return nil
}

func (q *InMemoryJobQueue) runWorker(name string, ch <-chan inMemoryJob, handler JobHandler) {
	defer q.wg.Done()
	for {
		select {
		case <-q.stopCh:
			return
		case job, ok := <-ch:
			if !ok {
				return
			}
			q.safeRun(name, handler, job)
		}
	}
}

func (q *InMemoryJobQueue) safeRun(name string, handler JobHandler, job inMemoryJob) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("eventbus: in-memory job handler panic",
				"queue", name, "panic", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), defaultJobTimeout)
	defer cancel()
	if err := handler(ctx, job.payload); err != nil {
		slog.Warn("eventbus: in-memory job handler returned error",
			"queue", name, "error", err)
	}
}

// Stop drains the queue and shuts down workers. Idempotent.
func (q *InMemoryJobQueue) Stop() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	close(q.stopCh)
	// Close all channels so any remaining workers exit.
	for _, ch := range q.channels {
		close(ch)
	}
	q.mu.Unlock()
	q.wg.Wait()
}

// defaultJobTimeout bounds each async-hook invocation so a slow subscriber
// cannot wedge a worker forever.
const defaultJobTimeout = 30 * time.Second
