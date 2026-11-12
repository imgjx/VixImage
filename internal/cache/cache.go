package cache

import (
	"context"
	"time"

	"viximage/internal/config"
)

// Cache is the caching abstraction. Two implementations: File (zero-config) and
// Redis. Selected by config.CacheMode.
type Cache interface {
	Set(key, value string, ttl time.Duration) error
	Get(key string) (string, bool)
	Del(key string) error
	Incr(key string) (int64, error)
	Exists(key string) bool
	Close() error
}

// Open returns the configured cache backend.
// Redis is preferred when selected, but it is always wrapped with a File
// fallback: if Redis is unreachable (not installed / down), sessions and
// counters transparently degrade to the local file cache instead of
// breaking login and API calls.
func Open(cfg *config.Config) Cache {
	local := NewFile(cfg.CacheFileDir)
	if cfg.CacheMode == "redis" {
		return NewFallback(NewRedis(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB), local)
	}
	return local
}

// ctx is a shared background context for redis ops.
var ctx = context.Background()
