package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// FileCache is a zero-dependency on-disk cache. Each entry is a small JSON file
// holding the value and an expiry timestamp.
type FileCache struct {
	dir string
	mu  sync.RWMutex
}

type fileEntry struct {
	Value   string `json:"v"`
	Expires int64  `json:"e"` // unix ms; 0 = no expiry
}

func NewFile(dir string) *FileCache {
	_ = os.MkdirAll(dir, 0o755)
	return &FileCache{dir: dir}
}

// path 把缓存键映射为合法的文件名。
//
// 注意：键里含有 ':'（如 "gui:sid:123"、"api:calls:total"）。Windows 不允许
// 文件名含冒号，若直接拼进文件名，写入会失败——而会话缓存正是用 "gui:sid:*"，
// 失败会被上层忽略（_ = cache.Set），表现为「登录成功后又被弹回登录页」。
// 因此这里把键整体做 SHA-256，取前 16 位十六进制作为文件名，跨平台安全且
// 不会因转义规则不同而互相覆盖。
func (c *FileCache) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:8])+".cache")
}

func (c *FileCache) Set(key, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var exp int64
	if ttl > 0 {
		exp = time.Now().Add(ttl).UnixMilli()
	}
	b, err := json.Marshal(fileEntry{Value: value, Expires: exp})
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(key), b, 0o644)
}

func (c *FileCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return "", false
	}
	var e fileEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return "", false
	}
	if e.Expires > 0 && time.Now().UnixMilli() > e.Expires {
		_ = os.Remove(c.path(key))
		return "", false
	}
	return e.Value, true
}

func (c *FileCache) Del(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := os.Remove(c.path(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (c *FileCache) Exists(key string) bool {
	_, ok := c.Get(key)
	return ok
}

func (c *FileCache) Incr(key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := int64(0)
	if b, err := os.ReadFile(c.path(key)); err == nil {
		var e fileEntry
		if json.Unmarshal(b, &e) == nil {
			if e.Expires > 0 && time.Now().UnixMilli() > e.Expires {
				v = 0
			} else {
				_ = json.Unmarshal([]byte(e.Value), &v)
			}
		}
	}
	v++
	b, _ := json.Marshal(fileEntry{Value: strconv.FormatInt(v, 10), Expires: 0})
	_ = os.WriteFile(c.path(key), b, 0o644)
	return v, nil
}

func (c *FileCache) Close() error { return nil }
