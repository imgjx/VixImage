// Package httplog 提供 HTTP 访问日志中间件，把每个请求的方法、路径、状态码
// 与耗时打印到标准日志（CLI 实时可见）。
package httplog

import (
	"log"
	"net/http"
	"time"
)

// WithAccessLog 包装一个 handler，记录每个请求的方法、路径、状态码与耗时。
// tag 用于区分日志来源（如 "GUI" / "API"）。
func WithAccessLog(tag string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("[%s] %d %s %s %v", tag, sw.status, r.Method, r.URL.Path, time.Since(start))
	})
}

// statusWriter 记录下游写回的状态码（默认 200）。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
