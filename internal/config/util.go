package config

import (
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// SafeFileName 把用户上传的原始文件名清洗成各平台都合法的文件名。
//
// 上传文件名可能带路径分隔符（Windows 上是 '\'）或 Windows 保留字符
// （: * ? " < > |），直接拼进路径会导致写入失败或写到意外位置。
func SafeFileName(name string) string {
	// 去掉目录部分，只保留基名。
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	// 逐字符过滤非法字符。
	var b strings.Builder
	for _, r := range name {
		switch r {
		case ':', '*', '?', '"', '<', '>', '|', '/', '\\':
			b.WriteRune('_')
		default:
			if r < 32 {
				continue
			}
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == "." || out == ".." {
		return "upload.bin"
	}
	return out
}

// hashDefault is a tiny alias kept for readability (default-admin bootstrap).
func hashDefault(p string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(h)
}

// HashPassword hashes a plaintext password with bcrypt.
func HashPassword(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
func CheckPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
