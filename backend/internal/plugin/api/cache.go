package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// cachePrefix is the key prefix every CacheStore operation silently
// prepends. Keeping it a single constant makes Scan / Del glob rewrites
// trivial and prevents plugins from colliding with host-owned keys.
const cachePrefix = "plugin:"

// cacheStore adapts *redis.Client to plugin.CacheStore with automatic
// namespacing. The prefix rule is "plugin:<pluginID>:" — callers use bare
// keys, and the wrapper never exposes raw Redis keys outside the package.
type cacheStore struct {
	guard    *guard
	rdb      *redis.Client
	keyScope string
}

// newCacheStore returns the wrapper (or an ErrNotImplemented stub) bound
// to pluginID's namespace.
func newCacheStore(c *coreAPIImpl) plugin.CacheStore {
	if c.deps.Redis == nil {
		return unimplementedCacheStore{}
	}
	return &cacheStore{
		guard:    c.guard,
		rdb:      c.deps.Redis,
		keyScope: cachePrefix + c.pluginID + ":",
	}
}

// key prepends the plugin namespace to a caller-supplied bare key.
func (c *cacheStore) key(bare string) string { return c.keyScope + bare }

// Get returns the bytes stored under key. A missing key yields
// (nil, redis.Nil) which callers can match with errors.Is.
func (c *cacheStore) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.rdb.Get(ctx, c.key(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, err
		}
		return nil, fmt.Errorf("plugin cache get: %w", err)
	}
	return val, nil
}

// Set stores value under key with the given TTL. A non-positive ttl means
// "no expiry" (equivalent to SET without EX).
func (c *cacheStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, c.key(key), value, ttl).Err(); err != nil {
		return fmt.Errorf("plugin cache set: %w", err)
	}
	return nil
}

// Del removes every named key. An empty list is a no-op.
func (c *cacheStore) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	scoped := make([]string, len(keys))
	for i, k := range keys {
		scoped[i] = c.key(k)
	}
	if err := c.rdb.Del(ctx, scoped...).Err(); err != nil {
		return fmt.Errorf("plugin cache del: %w", err)
	}
	return nil
}

// Scan returns every key matching pattern (after prefixing). Bare keys are
// returned — callers never see the "plugin:<id>:" prefix.
func (c *cacheStore) Scan(ctx context.Context, pattern string) ([]string, error) {
	fullPattern := c.key(pattern)
	iter := c.rdb.Scan(ctx, 0, fullPattern, 0).Iterator()
	var out []string
	for iter.Next(ctx) {
		scoped := iter.Val()
		if len(scoped) < len(c.keyScope) {
			continue
		}
		out = append(out, scoped[len(c.keyScope):])
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("plugin cache scan: %w", err)
	}
	return out, nil
}

// unimplementedCacheStore is returned when Redis is not wired in yet.
type unimplementedCacheStore struct{}

func (unimplementedCacheStore) Get(context.Context, string) ([]byte, error) {
	return nil, plugin.ErrNotImplemented
}
func (unimplementedCacheStore) Set(context.Context, string, []byte, time.Duration) error {
	return plugin.ErrNotImplemented
}
func (unimplementedCacheStore) Del(context.Context, ...string) error {
	return plugin.ErrNotImplemented
}
func (unimplementedCacheStore) Scan(context.Context, string) ([]string, error) {
	return nil, plugin.ErrNotImplemented
}
