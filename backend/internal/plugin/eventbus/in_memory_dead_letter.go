package eventbus

import (
	"context"
	"sort"
	"sync"
	"time"
)

// InMemoryDeadLetterRepo is a non-persistent DeadLetterRepo used by tests
// and by Phase 0 before the Postgres-backed repo is wired up.
type InMemoryDeadLetterRepo struct {
	mu      sync.Mutex
	nextID  int64
	entries map[int64]DeadLetterEntry
}

// NewInMemoryDeadLetterRepo returns an empty repo.
func NewInMemoryDeadLetterRepo() *InMemoryDeadLetterRepo {
	return &InMemoryDeadLetterRepo{entries: make(map[int64]DeadLetterEntry)}
}

// Record stores the entry and assigns an id.
func (r *InMemoryDeadLetterRepo) Record(_ context.Context, e DeadLetterEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	e.ID = r.nextID
	if e.FirstFailedAt.IsZero() {
		e.FirstFailedAt = time.Now()
	}
	if e.LastAttemptAt.IsZero() {
		e.LastAttemptAt = time.Now()
	}
	r.entries[e.ID] = e
	return nil
}

// List returns matching entries sorted by first_failed_at desc.
func (r *InMemoryDeadLetterRepo) List(_ context.Context, f DeadLetterFilter) ([]DeadLetterEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]DeadLetterEntry, 0)
	for _, e := range r.entries {
		if !matchesFilter(e, f) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FirstFailedAt.After(out[j].FirstFailedAt)
	})
	return applyPagination(out, f), nil
}

// Retry invokes the retrier; removes the entry on success, bumps its
// attempt_count on failure.
func (r *InMemoryDeadLetterRepo) Retry(ctx context.Context, id int64, retrier DeadLetterRetrier) error {
	r.mu.Lock()
	entry, ok := r.entries[id]
	r.mu.Unlock()
	if !ok {
		return ErrDeadLetterNotFound
	}
	if err := retrier(ctx, entry.Topic, entry.Payload); err != nil {
		r.mu.Lock()
		entry.AttemptCount++
		entry.LastAttemptAt = time.Now()
		entry.LastError = err.Error()
		r.entries[id] = entry
		r.mu.Unlock()
		return err
	}
	return r.Delete(ctx, id)
}

// Delete removes an entry.
func (r *InMemoryDeadLetterRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[id]; !ok {
		return ErrDeadLetterNotFound
	}
	delete(r.entries, id)
	return nil
}

// Len exposes the current entry count to tests.
func (r *InMemoryDeadLetterRepo) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

func matchesFilter(e DeadLetterEntry, f DeadLetterFilter) bool {
	if f.Topic != "" && e.Topic != f.Topic {
		return false
	}
	if f.SubscriberTag != "" && e.SubscriberTag != f.SubscriberTag {
		return false
	}
	return true
}

func applyPagination(in []DeadLetterEntry, f DeadLetterFilter) []DeadLetterEntry {
	if f.Offset > 0 {
		if f.Offset >= len(in) {
			return nil
		}
		in = in[f.Offset:]
	}
	if f.Limit > 0 && f.Limit < len(in) {
		in = in[:f.Limit]
	}
	return in
}
