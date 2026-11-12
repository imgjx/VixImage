package gui

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// sessionStore 是进程内的会话表。
//
// 设计原因：GUI 登录态是单进程状态，没必要落盘。落盘会引入两类致命问题：
//  1. 缓存目录不可写（Windows 上装在 Program Files、或权限受限）时写入静默失败；
//  2. 缓存键含 ':'，Windows 不允许文件名带冒号，写入必然失败。
//
// 两者都会表现为「登录成功 → 立刻被弹回登录页」，而错误被 `_ = Set(...)` 吞掉。
//
// 会话令牌用 crypto/rand 生成，避免时间戳被猜到。
type sessionStore struct {
	mu sync.Mutex
	m  map[string]sessionEntry
}

type sessionEntry struct {
	user    string
	expires time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{m: make(map[string]sessionEntry)}
}

// newToken 生成一个随机会话令牌。
func (s *sessionStore) newToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		// rand 失败时退化为时间戳+进程内计数，仍保证进程内唯一。
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// put 写入/刷新一个会话。
func (s *sessionStore) put(token, user string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string]sessionEntry)
	}
	s.m[token] = sessionEntry{user: user, expires: time.Now().Add(ttl)}
}

// get 查询会话，返回用户名与会话是否有效。
func (s *sessionStore) get(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[token]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expires) {
		delete(s.m, token)
		return "", false
	}
	return e.user, true
}

// del 删除会话。
func (s *sessionStore) del(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, token)
}

// cleanup 清理过期会话（定时调用，防止长期运行内存增长）。
func (s *sessionStore) cleanup() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.m {
		if now.After(e.expires) {
			delete(s.m, k)
		}
	}
}
