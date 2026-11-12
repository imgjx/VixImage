package cache

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache wraps go-redis. Falls back gracefully: connection errors surface on
// Get as a cache miss, so the system keeps working even if Redis is unavailable.
type RedisCache struct {
	cli *redis.Client
}

func NewRedis(addr, pass string, db int) *RedisCache {
	cli := redis.NewClient(&redis.Options{Addr: addr, Password: pass, DB: db})
	return &RedisCache{cli: cli}
}

func (c *RedisCache) Set(key, value string, ttl time.Duration) error {
	return c.cli.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Get(key string) (string, bool) {
	v, err := c.cli.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	return v, true
}

// Healthy reports whether the Redis server is reachable.
func (c *RedisCache) Healthy() bool {
	return c.cli.Ping(ctx).Err() == nil
}

func (c *RedisCache) Del(key string) error {
	return c.cli.Del(ctx, key).Err()
}

func (c *RedisCache) Exists(key string) bool {
	n, err := c.cli.Exists(ctx, key).Result()
	return err == nil && n > 0
}

func (c *RedisCache) Incr(key string) (int64, error) {
	return c.cli.Incr(ctx, key).Result()
}

func (c *RedisCache) Close() error { return c.cli.Close() }
