//go:build unit

package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

func TestCacheStore_KeyPrefix(t *testing.T) {
	c := &cacheStore{keyScope: "plugin:demo:"}
	got := c.key("foo")
	want := "plugin:demo:foo"
	if got != want {
		t.Fatalf("key prefix wrong: got=%q want=%q", got, want)
	}
}

func TestCacheStore_MissingRedisFallsBackToUnimplemented(t *testing.T) {
	factory := NewCoreAPIFactory(Dependencies{})
	core := factory.For("demo", []plugin.Permission{})
	cache := core.Cache()
	if _, err := cache.Get(context.Background(), "x"); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
	if err := cache.Set(context.Background(), "x", []byte("v"), time.Second); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented on Set, got %v", err)
	}
	if err := cache.Del(context.Background(), "x"); !errors.Is(err, plugin.ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented on Del, got %v", err)
	}
}
