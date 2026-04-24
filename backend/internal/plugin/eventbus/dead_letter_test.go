//go:build unit

package eventbus

import (
	"context"
	"errors"
	"testing"
)

func TestInMemoryDeadLetterRecordListDelete(t *testing.T) {
	repo := NewInMemoryDeadLetterRepo()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := repo.Record(ctx, DeadLetterEntry{
			Topic:         "topic.a",
			Payload:       []byte(`{}`),
			SubscriberTag: "sub",
			LastError:     "oops",
		}); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	got, err := repo.List(ctx, DeadLetterFilter{Topic: "topic.a"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}

	if err := repo.Delete(ctx, got[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.Delete(ctx, got[0].ID); !errors.Is(err, ErrDeadLetterNotFound) {
		t.Fatalf("expected ErrDeadLetterNotFound, got %v", err)
	}
}

func TestInMemoryDeadLetterRetryFailureUpdatesAttempt(t *testing.T) {
	repo := NewInMemoryDeadLetterRepo()
	ctx := context.Background()
	if err := repo.Record(ctx, DeadLetterEntry{Topic: "t", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	entries, _ := repo.List(ctx, DeadLetterFilter{})
	id := entries[0].ID

	wantErr := errors.New("still broken")
	err := repo.Retry(ctx, id, func(ctx context.Context, topic string, payload []byte) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped failure, got %v", err)
	}
	after, _ := repo.List(ctx, DeadLetterFilter{})
	if after[0].AttemptCount != 1 {
		t.Fatalf("expected attempt_count=1, got %d", after[0].AttemptCount)
	}
	if after[0].LastError != wantErr.Error() {
		t.Fatalf("last_error not updated: %q", after[0].LastError)
	}
}

func TestInMemoryDeadLetterRetrySuccessDeletes(t *testing.T) {
	repo := NewInMemoryDeadLetterRepo()
	ctx := context.Background()
	_ = repo.Record(ctx, DeadLetterEntry{Topic: "t", Payload: []byte(`{}`)})
	entries, _ := repo.List(ctx, DeadLetterFilter{})
	id := entries[0].ID

	err := repo.Retry(ctx, id, func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})
	if err != nil {
		t.Fatalf("retry success: %v", err)
	}
	if repo.Len() != 0 {
		t.Fatalf("entry not deleted after successful retry")
	}
}
