package cache

import (
	"sync"
	"time"
)

// FallbackCache wraps a primary cache (e.g. Redis) with a secondary local cache
// (File). When the primary backend is unreachable, reads and writes transparently
// degrade to the secondary one, so the system keeps serving requests instead of
// failing authentication outright.
type FallbackCache struct {
	primary Cache
	backup  Cache
	// downUntil, when set, marks the primary backend as known-unavailable so
	// requests skip it entirely instead of paying a connection timeout each time.
	downUntil time.Time
	mu        sync.Mutex
}

// NewFallback returns a cache that prefers primary and falls back to backup.
func NewFallback(primary, backup Cache) Cache {
	if primary == nil {
		return backup
	}
	if backup == nil {
		return primary
	}
	return &FallbackCache{primary: primary, backup: backup}
}

// healthy reports whether the primary backend can be used right now.
func (c *FallbackCache) healthy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.downUntil) {
		return false
	}
	type healthChecker interface{ Healthy() bool }
	if h, ok := c.primary.(healthChecker); ok {
		if !h.Healthy() {
			// Mark it down for a short while to avoid hammering a dead server.
			c.downUntil = time.Now().Add(10 * time.Second)
			return false
		}
	}
	return true
}

// markDown records that the primary backend failed.
func (c *FallbackCache) markDown() {
	c.mu.Lock()
	c.downUntil = time.Now().Add(10 * time.Second)
	c.mu.Unlock()
}

func (c *FallbackCache) Set(key, value string, ttl time.Duration) error {
	if c.healthy() {
		if err := c.primary.Set(key, value, ttl); err == nil {
			return nil
		}
		c.markDown()
	}
	return c.backup.Set(key, value, ttl)
}

func (c *FallbackCache) Get(key string) (string, bool) {
	if c.healthy() {
		if v, ok := c.primary.Get(key); ok {
			return v, true
		}
	}
	return c.backup.Get(key)
}

func (c *FallbackCache) Del(key string) error {
	e1 := c.primary.Del(key)
	e2 := c.backup.Del(key)
	if e1 != nil {
		return e1
	}
	return e2
}

func (c *FallbackCache) Incr(key string) (int64, error) {
	if c.healthy() {
		if v, err := c.primary.Incr(key); err == nil {
			return v, nil
		}
		c.markDown()
	}
	return c.backup.Incr(key)
}

func (c *FallbackCache) Exists(key string) bool {
	if c.primary.Exists(key) {
		return true
	}
	return c.backup.Exists(key)
}

func (c *FallbackCache) Close() error {
	_ = c.primary.Close()
	return c.backup.Close()
}
