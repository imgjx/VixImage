package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Windows 不允许文件名包含冒号等字符，缓存键（含 ':' 分隔的命名空间）
// 直接拼进文件名会导致 os.WriteFile 失败——而会话缓存正是用 "gui:sid:xxx"，
// 失败会让登录态永远读不到，表现为「登录成功后又被弹回登录页」。
func TestFileCacheKeyWithColonIsWritable(t *testing.T) {
	dir := t.TempDir()
	c := NewFile(dir)
	key := "gui:sid:1755000000000000000"
	if err := c.Set(key, "admin", time.Hour); err != nil {
		t.Fatalf("Set(%q) 失败: %v", key, err)
	}
	if v, ok := c.Get(key); !ok || v != "admin" {
		t.Fatalf("Get(%q) = (%q,%v)，期望 (\"admin\",true)", key, v, ok)
	}
	// 文件名必须是各平台合法的文件名。
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.ContainsAny(e.Name(), `:\/:*?"<>|`) {
			t.Fatalf("缓存文件名含非法字符 %q", e.Name())
		}
	}
	_ = filepath.Join(dir, "x")
}

// 不同的键不能因为转义而互相覆盖。
func TestFileCacheKeysDoNotCollide(t *testing.T) {
	dir := t.TempDir()
	c := NewFile(dir)
	c.Set("a:b", "1", time.Hour)
	c.Set("a-b", "2", time.Hour)
	v1, ok1 := c.Get("a:b")
	v2, ok2 := c.Get("a-b")
	if !ok1 || v1 != "1" {
		t.Fatalf("a:b = (%q,%v)", v1, ok1)
	}
	if !ok2 || v2 != "2" {
		t.Fatalf("a-b = (%q,%v)", v2, ok2)
	}
}
