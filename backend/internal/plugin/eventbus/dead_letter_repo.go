package eventbus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DeadLetterEntry is one persisted failed-delivery record. Mirrors the
// columns of event_dead_letters (migration 103).
type DeadLetterEntry struct {
	ID            int64
	Topic         string
	Payload       []byte // JSON-encoded payload
	FirstFailedAt time.Time
	LastAttemptAt time.Time
	AttemptCount  int
	LastError     string
	SubscriberTag string
	CorrelationID string
}

// DeadLetterFilter narrows list queries.
type DeadLetterFilter struct {
	Topic         string
	SubscriberTag string
	Limit         int
	Offset        int
}

// DeadLetterRepo persists failed AsyncHook deliveries so operators can
// investigate and retry them. Two concrete implementations are provided:
// a Postgres-backed [SQLDeadLetterRepo] and an [InMemoryDeadLetterRepo] for
// tests.
type DeadLetterRepo interface {
	Record(ctx context.Context, e DeadLetterEntry) error
	List(ctx context.Context, filter DeadLetterFilter) ([]DeadLetterEntry, error)
	// Retry re-enqueues the entry via the bus. Implementations call the
	// provided retrier rather than owning their own job queue handle.
	Retry(ctx context.Context, id int64, retrier DeadLetterRetrier) error
	Delete(ctx context.Context, id int64) error
}

// DeadLetterRetrier is the callback signature Retry invokes; defined here to
// avoid a dependency cycle with the bus itself.
type DeadLetterRetrier func(ctx context.Context, topic string, payload []byte) error

// ErrDeadLetterNotFound is returned by [DeadLetterRepo.Retry] and
// [DeadLetterRepo.Delete] when the id has no matching row.
var ErrDeadLetterNotFound = errors.New("eventbus: dead letter not found")

// SQLDeadLetterRepo stores entries in Postgres. Uses *sql.DB directly (not
// ent) to keep the eventbus package free of generated-code dependencies.
type SQLDeadLetterRepo struct {
	db *sql.DB
}

// NewSQLDeadLetterRepo wires the repository to the given database handle.
func NewSQLDeadLetterRepo(db *sql.DB) *SQLDeadLetterRepo {
	return &SQLDeadLetterRepo{db: db}
}

// Record inserts a new dead-letter row.
func (r *SQLDeadLetterRepo) Record(ctx context.Context, e DeadLetterEntry) error {
	const stmt = `
INSERT INTO event_dead_letters
    (topic, payload, first_failed_at, last_attempt_at, attempt_count,
     last_error, subscriber_tag, correlation_id)
VALUES ($1, $2, COALESCE($3, now()), COALESCE($4, now()), $5, $6, $7, $8)
RETURNING id`
	var id int64
	err := r.db.QueryRowContext(ctx, stmt,
		e.Topic, e.Payload,
		nullableTime(e.FirstFailedAt), nullableTime(e.LastAttemptAt),
		e.AttemptCount, e.LastError, e.SubscriberTag, e.CorrelationID,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("eventbus: record dead letter: %w", err)
	}
	return nil
}

// List returns the most recently failed entries matching the filter.
func (r *SQLDeadLetterRepo) List(ctx context.Context, filter DeadLetterFilter) ([]DeadLetterEntry, error) {
	q, args := buildListQuery(filter)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("eventbus: list dead letters: %w", err)
	}
	defer rows.Close()
	return scanDeadLetterRows(rows)
}

// Retry looks up the row and hands it to the retrier callback; on success
// the row is deleted, on failure its attempt_count/last_error update.
func (r *SQLDeadLetterRepo) Retry(ctx context.Context, id int64, retrier DeadLetterRetrier) error {
	entry, err := r.fetchOne(ctx, id)
	if err != nil {
		return err
	}
	if err := retrier(ctx, entry.Topic, entry.Payload); err != nil {
		return r.recordRetryFailure(ctx, id, err)
	}
	return r.Delete(ctx, id)
}

// Delete removes the row.
func (r *SQLDeadLetterRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM event_dead_letters WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("eventbus: delete dead letter: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrDeadLetterNotFound
	}
	return nil
}

func (r *SQLDeadLetterRepo) fetchOne(ctx context.Context, id int64) (DeadLetterEntry, error) {
	const stmt = `
SELECT id, topic, payload, first_failed_at, last_attempt_at, attempt_count,
       COALESCE(last_error,''), COALESCE(subscriber_tag,''), COALESCE(correlation_id,'')
FROM event_dead_letters WHERE id = $1`
	var e DeadLetterEntry
	err := r.db.QueryRowContext(ctx, stmt, id).Scan(
		&e.ID, &e.Topic, &e.Payload, &e.FirstFailedAt, &e.LastAttemptAt,
		&e.AttemptCount, &e.LastError, &e.SubscriberTag, &e.CorrelationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DeadLetterEntry{}, ErrDeadLetterNotFound
	}
	if err != nil {
		return DeadLetterEntry{}, fmt.Errorf("eventbus: fetch dead letter: %w", err)
	}
	return e, nil
}

func (r *SQLDeadLetterRepo) recordRetryFailure(ctx context.Context, id int64, cause error) error {
	const stmt = `
UPDATE event_dead_letters
SET attempt_count = attempt_count + 1,
    last_attempt_at = now(),
    last_error = $1
WHERE id = $2`
	if _, err := r.db.ExecContext(ctx, stmt, cause.Error(), id); err != nil {
		return fmt.Errorf("eventbus: update dead letter retry: %w", err)
	}
	return fmt.Errorf("eventbus: retry failed (id=%d): %w", id, cause)
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func buildListQuery(f DeadLetterFilter) (string, []any) {
	const defaultLimit = 100
	q := `
SELECT id, topic, payload, first_failed_at, last_attempt_at, attempt_count,
       COALESCE(last_error,''), COALESCE(subscriber_tag,''), COALESCE(correlation_id,'')
FROM event_dead_letters
WHERE 1=1`
	args := []any{}
	if f.Topic != "" {
		args = append(args, f.Topic)
		q += fmt.Sprintf(" AND topic = $%d", len(args))
	}
	if f.SubscriberTag != "" {
		args = append(args, f.SubscriberTag)
		q += fmt.Sprintf(" AND subscriber_tag = $%d", len(args))
	}
	limit := f.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY first_failed_at DESC LIMIT $%d", len(args))
	if f.Offset > 0 {
		args = append(args, f.Offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	return q, args
}

func scanDeadLetterRows(rows *sql.Rows) ([]DeadLetterEntry, error) {
	var out []DeadLetterEntry
	for rows.Next() {
		var e DeadLetterEntry
		if err := rows.Scan(&e.ID, &e.Topic, &e.Payload, &e.FirstFailedAt,
			&e.LastAttemptAt, &e.AttemptCount, &e.LastError,
			&e.SubscriberTag, &e.CorrelationID); err != nil {
			return nil, fmt.Errorf("eventbus: scan dead letter: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
