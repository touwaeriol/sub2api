package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	signaturePoolKeyPrefix = "sig_pool:"
	signaturePoolMaxSize   = 100
	signaturePoolTTL       = 24 * time.Hour
)

type signaturePoolCache struct {
	rdb *redis.Client
}

func NewSignaturePoolCache(rdb *redis.Client) service.SignaturePoolCache {
	return &signaturePoolCache{rdb: rdb}
}

func signaturePoolKey(accountID int64) string {
	return fmt.Sprintf("%s%d", signaturePoolKeyPrefix, accountID)
}

func (c *signaturePoolCache) Add(ctx context.Context, accountID int64, signature string) error {
	signature = strings.TrimSpace(signature)
	if c == nil || c.rdb == nil || signature == "" {
		return nil
	}
	key := signaturePoolKey(accountID)
	pipe := c.rdb.Pipeline()
	pipe.SAdd(ctx, key, signature)
	pipe.Expire(ctx, key, signaturePoolTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	size, err := c.rdb.SCard(ctx, key).Result()
	if err != nil {
		return nil
	}
	for size > signaturePoolMaxSize {
		if err := c.rdb.SPop(ctx, key).Err(); err != nil {
			break
		}
		size--
	}
	return nil
}

func (c *signaturePoolCache) RandomGet(ctx context.Context, accountID int64) (string, error) {
	if c == nil || c.rdb == nil {
		return "", nil
	}
	sig, err := c.rdb.SRandMember(ctx, signaturePoolKey(accountID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return strings.TrimSpace(sig), err
}

func (c *signaturePoolCache) Size(ctx context.Context, accountID int64) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	return c.rdb.SCard(ctx, signaturePoolKey(accountID)).Result()
}
