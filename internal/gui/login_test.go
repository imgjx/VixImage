package gui

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"viximage/internal/config"
)

// testConfig 构造一个默认账号 admin/admin123 的配置。
func testConfig() *config.Config {
	cfg := config.Default()
	cfg.GUIUser = "admin"
	h, _ := config.HashPassword("admin123")
	cfg.GUIPassHash = h
	return cfg
}

// newJar 返回一个空 cookie jar。
func newJar() *cookiejar.Jar {
	j, _ := cookiejar.New(nil)
	return j
}

// brokenCache 模拟「缓存写入失败」的环境：
// Windows 上缓存目录不可写、或缓存键含 ':' 导致文件名非法时，
// 磁盘缓存会全线失效。GUI 会话必须仍然可用。
type brokenCache struct{}

func (brokenCache) Set(string, string, time.Duration) error { return http.ErrServerClosed }
func (brokenCache) Get(string) (string, bool)               { return "", false }
func (brokenCache) Del(string) error                        { return nil }

// 测试：磁盘缓存完全不可用时，登录成功后仍能进入控制台（不会被弹回登录页）。
func TestLoginWorksWhenCacheBroken(t *testing.T) {
	cfg := testConfig()
	s := New(cfg, nil, nil, nil, brokenCache{})
	s.sessions = newSessionStore()
	h := s.Handler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	// 1) 未登录访问首页 → 302 到 /login
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := cl.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 302 {
		t.Fatalf("未登录访问首页 = %d，期望 302", resp.StatusCode)
	}

	// 2) 原生表单登录（无 JS 路径），不自动跟随跳转以便观察 302 与 Set-Cookie
	jar, _ := cookiejar.New(nil)
	noredir := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	form := strings.NewReader("username=admin&password=admin123")
	resp, err = noredir.Post(srv.URL+"/login", "application/x-www-form-urlencoded", form)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 302 {
		t.Fatalf("登录 = %d，期望 302", resp.StatusCode)
	}
	// 拿到会话 cookie
	var sid string
	for _, c := range resp.Cookies() {
		if c.Name == "vix_sid" {
			sid = c.Value
		}
	}
	if sid == "" {
		t.Fatal("登录后未下发 vix_sid cookie")
	}

	// 3) 带 cookie 访问首页 → 必须 200 且是控制台页面（不再被弹回登录页）
	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: "vix_sid", Value: sid})
	resp, err = cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("登录后访问首页 = %d，期望 200（不应被弹回登录页）", resp.StatusCode)
	}

	// 4) 带 cookie 访问受保护 API → 200
	req, _ = http.NewRequest("GET", srv.URL+"/api/state", nil)
	req.AddCookie(&http.Cookie{Name: "vix_Sid", Value: sid})
	req.AddCookie(&http.Cookie{Name: "vix_sid", Value: sid})
	resp, err = cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("登录后调用 /api/state = %d，期望 200", resp.StatusCode)
	}
}

// 会话令牌必须是随机且不含路径分隔符/非法字符。
func TestSessionTokenIsSafe(t *testing.T) {
	s := newSessionStore()
	tok := s.newToken()
	if tok == "" {
		t.Fatal("令牌为空")
	}
	if strings.ContainsAny(tok, `:\/:*?"<>| `) {
		t.Fatalf("令牌含非法字符: %q", tok)
	}
	// 令牌之间不应重复（随机性基本保证）。
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tok2 := s.newToken()
		if seen[tok2] {
			t.Fatalf("令牌重复: %s", tok2)
		}
		seen[tok2] = true
	}
}

// 会话过期后必须失效。
func TestSessionExpiry(t *testing.T) {
	s := newSessionStore()
	tok := s.newToken()
	s.put(tok, "admin", -time.Second)
	if _, ok := s.get(tok); ok {
		t.Fatal("过期会话仍被视为有效")
	}
}
