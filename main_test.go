package main

import (
	"testing"

	"viximage/internal/model"
)

// fakeStore 仅供 CLI 测试，实现 storage.Storage 的最小子集。
type fakeStore struct {
	byKey map[string]model.APIKey
}

func (f *fakeStore) GetAPIKey(key string) (*model.APIKey, error) {
	if k, ok := f.byKey[key]; ok {
		return &k, nil
	}
	return nil, nil
}

func TestAtoi64Safe(t *testing.T) {
	cases := map[string]int64{"": 0, "0": 0, "12": 12, "007": 7, "1a": 0, "x": 0}
	for in, want := range cases {
		if got := atoi64Safe(in); got != want {
			t.Fatalf("atoi64Safe(%q)=%d want %d", in, got, want)
		}
	}
}

func TestResolveKeyID(t *testing.T) {
	fs := &fakeStore{byKey: map[string]model.APIKey{
		"vix-sk-real": {ID: 7, Key: "vix-sk-real"},
	}}
	// 纯数字按 id 解析。
	if id, _ := resolveKeyID(fs, "3"); id != 3 {
		t.Fatalf("纯数字解析失败: %d", id)
	}
	// 完整密钥按 key 查询。
	id, name := resolveKeyID(fs, "vix-sk-real")
	if id != 7 {
		t.Fatalf("密钥解析失败: %d", id)
	}
	if name == "" {
		t.Fatalf("应返回掩码名，实际为空")
	}
	// 不存在的密钥返回 0。
	if id, _ := resolveKeyID(fs, "nope"); id != 0 {
		t.Fatalf("不存在应返回 0，实际 %d", id)
	}
}

func TestClip(t *testing.T) {
	if clip("abcdef", 3) != "abc" {
		t.Fatalf("clip 截断失败")
	}
	if clip("ab", 5) != "ab" {
		t.Fatalf("clip 短串应原样")
	}
}
