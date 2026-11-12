package config

import (
	"strings"
	"testing"
)

// 上传文件名可能含 Windows 保留字符或路径分隔符，
// 直接拼进路径会写入失败或写到意外位置。
func TestSafeFileName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"photo.png", "photo.png"},
		{"C:\\Users\\me\\pic.jpg", "pic.jpg"},
		{"../../etc/passwd", "passwd"},
		{"a:b*c?d.e", "a_b_c_d.e"},
		{"", "upload.bin"},
		{"..", "upload.bin"},
	}
	for _, c := range cases {
		got := SafeFileName(c.in)
		if got != c.want {
			t.Errorf("SafeFileName(%q) = %q，期望 %q", c.in, got, c.want)
		}
		if strings.ContainsAny(got, `:\/:*?"<>|`) {
			t.Errorf("SafeFileName(%q) = %q 仍含非法字符", c.in, got)
		}
	}
}
