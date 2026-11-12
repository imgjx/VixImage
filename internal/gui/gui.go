package gui

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"viximage/internal/agentmodel"
	"viximage/internal/client"
	"viximage/internal/config"
	"viximage/internal/httplog"
	"viximage/internal/model"
	"viximage/internal/storage"
	"viximage/internal/upstream"
)

// Server is the web console (GUI). It runs on its own port, protected by a
// simple account/password login (stored in config, editable via CLI/GUI).
type Server struct {
	cfg    *config.Config
	store  storage.Storage
	up     *upstream.Service
	agents *agentmodel.Service
	cache  Cache
	pages  fs.FS
	// sessions 是内存会话表。GUI 会话只属于本进程，用内存最稳：
	// 不依赖磁盘缓存目录可写、也不受平台文件名规则限制
	// （Windows 文件名不允许 ':'，缓存键曾因此写入失败，导致登录后被弹回登录页）。
	sessions *sessionStore
}

// Cache is the minimal interface the GUI needs (session store).
type Cache interface {
	Set(key, value string, ttl time.Duration) error
	Get(key string) (string, bool)
	Del(key string) error
}

func New(cfg *config.Config, store storage.Storage, up *upstream.Service, ag *agentmodel.Service, cch cacheLike) *Server {
	return &Server{cfg: cfg, store: store, up: up, agents: ag, cache: cch, sessions: newSessionStore()}
}

// cacheLike is satisfied by the cache.Cache implementation.
type cacheLike = interface {
	Set(key, value string, ttl time.Duration) error
	Get(key string) (string, bool)
	Del(key string) error
}

//go:embed all:web
var webFS embed.FS

// loginTmpl 登录页模板（服务端渲染；纯 HTML 表单，不依赖 JavaScript）。
var loginTmpl = template.Must(template.ParseFS(webFS, "web/login.html"))

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// pages
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/session.js", s.page("web/session.js"))
	mux.HandleFunc("/", s.auth(s.page("web/index.html")))
	mux.HandleFunc("/overview", s.auth(s.page("web/overview.html")))
	mux.HandleFunc("/mymodels", s.auth(s.page("web/mymodels.html")))
	mux.HandleFunc("/upstream", s.auth(s.page("web/upstream.html")))
	mux.HandleFunc("/config", s.auth(s.page("web/config.html")))
	mux.HandleFunc("/playground", s.auth(s.page("web/playground.html")))
	// api
	mux.HandleFunc("/api/login", s.apiLogin)
	mux.HandleFunc("/api/session", s.apiSession)
	mux.HandleFunc("/api/logout", s.auth(s.apiLogout))
	mux.HandleFunc("/api/state", s.auth(s.apiState))
	mux.HandleFunc("/api/overview", s.auth(s.apiOverview))
	mux.HandleFunc("/api/upstream", s.auth(s.apiUpstream))
	mux.HandleFunc("/api/upstream/", s.auth(s.apiUpstreamByID))
	mux.HandleFunc("/api/agents", s.auth(s.apiAgents))
	mux.HandleFunc("/api/agents/", s.auth(s.apiAgentsByID))
	mux.HandleFunc("/api/config", s.auth(s.apiConfig))
	mux.HandleFunc("/api/user", s.auth(s.apiUser))
	// playground
	mux.HandleFunc("/api/playground", s.auth(s.apiPlayground))
	mux.HandleFunc("/files/", s.auth(s.serveFile))
	return mux
}

func (s *Server) Start() error {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.GUIPort),
		Handler:      httplog.WithAccessLog("GUI", s.Handler()),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	// 定期清理过期会话，防止长期运行时内存增长。
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			s.sessions.cleanup()
		}
	}()
	return srv.ListenAndServe()
}

func (s *Server) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := webFS.ReadFile(name)
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		ct := "text/html; charset=utf-8"
		if strings.HasSuffix(name, ".css") {
			ct = "text/css"
		} else if strings.HasSuffix(name, ".js") {
			ct = "application/javascript"
		}
		w.Header().Set("Content-Type", ct)
		w.Write(b)
	}
}

// ---------------- auth ----------------

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authed(w, r) {
			next(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, 401, map[string]any{"error": "unauthorized"})
			return
		}
		// 未登录：302 跳到登录页。
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// authed 校验登录态：Cookie 优先，其次 X-Vix-Sid 请求头，再次 URL 参数 t。
// 三种来源都支持，是为了兼容浏览器不保存 / 不发送 Cookie 的情况——
// 此时靠本地令牌（localStorage）维持登录，不再出现「登录成功又被弹回登录页」。
func (s *Server) authed(w http.ResponseWriter, r *http.Request) bool {
	tok := sessionToken(r)
	if tok == "" {
		return false
	}
	// 内存会话为准；缓存中的会话（多实例/重启后仍有效）作为兜底。
	if _, ok := s.sessions.get(tok); !ok {
		if _, ok := s.cache.Get("gui:sid:" + tok); !ok {
			return false
		}
	}
	// 本次不是靠 Cookie 进来的（例如走了请求头/URL 参数），补发一份 Cookie。
	if c, err := r.Cookie("vix_sid"); err != nil || c.Value != tok {
		setSessionCookie(w, tok)
	}
	return true
}

// sessionToken 依次从 Cookie、X-Vix-Sid 头、URL 参数 t 中取会话令牌。
func sessionToken(r *http.Request) string {
	if c, err := r.Cookie("vix_sid"); err == nil && c.Value != "" {
		return c.Value
	}
	if v := strings.TrimSpace(r.Header.Get("X-Vix-Sid")); v != "" {
		return v
	}
	return strings.TrimSpace(r.URL.Query().Get("t"))
}

// setSessionCookie 下发会话 Cookie。
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: "vix_sid", Value: token, Path: "/", HttpOnly: true, MaxAge: 86400,
		SameSite: http.SameSiteLaxMode,
	})
}

// serveLogin 渲染登录页（错误提示由服务端注入）。
func (s *Server) serveLogin(w http.ResponseWriter, r *http.Request) {
	s.renderLogin(w, r.URL.Query().Get("err"))
}

// renderLogin 输出登录页 HTML。
func (s *Server) renderLogin(w http.ResponseWriter, errMsg string) {
	var buf bytes.Buffer
	if err := loginTmpl.Execute(&buf, map[string]string{"Err": errMsg}); err != nil {
		http.Error(w, "login page error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

// handleLogin 处理登录：GET 渲染登录页，POST 校验账号密码。
// 采用原生表单提交（不依赖 JavaScript），老旧浏览器同样可以登录；
// 成功后 302 回到来源页（默认首页）。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.serveLogin(w, r)
		return
	case http.MethodPost:
	default:
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, "表单解析失败")
		return
	}
	user := strings.TrimSpace(r.PostFormValue("username"))
	pass := r.PostFormValue("password")
	if user == "" || user != s.cfg.GUIUser || !config.CheckPassword(s.cfg.GUIPassHash, pass) {
		s.renderLogin(w, "账号或密码错误")
		return
	}
	token := s.sessions.newToken()
	s.sessions.put(token, user, 24*time.Hour)
	// 同步到缓存（失败不影响登录，内存会话才是准的）。
	if err := s.cache.Set("gui:sid:"+token, user, 24*time.Hour); err != nil {
		log.Printf("警告: 会话写入缓存失败(不影响登录): %v", err)
	}
	setSessionCookie(w, token)
	// 登录后回到来源页（仅允许站内相对路径）。
	target := "/"
	if next := r.PostFormValue("next"); strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		target = next
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	var b struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	if b.Username != s.cfg.GUIUser || !config.CheckPassword(s.cfg.GUIPassHash, b.Password) {
		writeJSON(w, 401, map[string]any{"error": "账号或密码错误"})
		return
	}
	token := s.sessions.newToken()
	s.sessions.put(token, b.Username, 24*time.Hour)
	if err := s.cache.Set("gui:sid:"+token, b.Username, 24*time.Hour); err != nil {
		log.Printf("警告: 会话写入缓存失败(不影响登录): %v", err)
	}
	setSessionCookie(w, token)
	writeJSON(w, 200, map[string]any{"ok": true, "username": b.Username, "token": token})
}

// apiSession 用本地保存的令牌换取新的会话 Cookie。
// 浏览器不保存 / 不发送 Cookie 时，前端凭 localStorage 中的令牌调用本接口，
// 服务端校验通过即重新下发 Cookie，从而把「登录成功又被弹回登录页」自动修复。
func (s *Server) apiSession(w http.ResponseWriter, r *http.Request) {
	tok := strings.TrimSpace(r.Header.Get("X-Vix-Sid"))
	if tok == "" {
		if c, err := r.Cookie("vix_sid"); err == nil {
			tok = c.Value
		}
	}
	if tok == "" {
		writeJSON(w, 401, map[string]any{"error": "no session"})
		return
	}
	if _, ok := s.sessions.get(tok); !ok {
		if _, ok := s.cache.Get("gui:sid:" + tok); !ok {
			writeJSON(w, 401, map[string]any{"error": "invalid session"})
			return
		}
	}
	setSessionCookie(w, tok)
	writeJSON(w, 200, map[string]any{"ok": true, "username": s.cfg.GUIUser})
}

func (s *Server) apiLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("vix_sid"); err == nil {
		s.sessions.del(c.Value)
		_ = s.cache.Del("gui:sid:" + c.Value)
	}
	if v := strings.TrimSpace(r.Header.Get("X-Vix-Sid")); v != "" {
		s.sessions.del(v)
		_ = s.cache.Del("gui:sid:" + v)
	}
	http.SetCookie(w, &http.Cookie{Name: "vix_sid", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) apiState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"username": s.cfg.GUIUser,
		"gui_port": s.cfg.GUIPort,
		"api_port": s.cfg.APIPort,
		"storage":  s.cfg.StorageMode,
		"cache":    s.cfg.CacheMode,
		"data_dir": s.cfg.DataDir,
	})
}

// ---------------- 调用概述 ----------------

func (s *Server) apiOverview(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.CallStats()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	// 分页：page(从 1 起) / size(每页条数)。
	page := int(atoi64(r.URL.Query().Get("page")))
	size := int(atoi64(r.URL.Query().Get("size")))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	logs, total, err := s.store.ListCallLogsPaged((page-1)*size, size)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	pages := (total + int64(size) - 1) / int64(size)
	writeJSON(w, 200, map[string]any{
		"stats": stats, "logs": logs,
		"page": page, "size": size, "total": total, "pages": pages,
	})
}

// ---------------- 上游模型 ----------------

func (s *Server) apiUpstream(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListUpstream()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		out := make([]model.UpstreamModel, 0, len(list))
		for _, m := range list {
			out = append(out, m.MaskKey())
		}
		writeJSON(w, 200, map[string]any{"upstream": out})
	case http.MethodPost:
		var m model.UpstreamModel
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			writeJSON(w, 400, map[string]any{"error": "bad json"})
			return
		}
		if err := s.up.Save(&m); err != nil {
			writeJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "id": m.ID})
	default:
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) apiUpstreamByID(w http.ResponseWriter, r *http.Request) {
	id := atoi64(strings.TrimPrefix(r.URL.Path, "/api/upstream/"))
	if r.Method == http.MethodDelete {
		if err := s.up.Delete(id); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	writeJSON(w, 405, map[string]any{"error": "method not allowed"})
}

// ---------------- 我的模型 ----------------

func (s *Server) apiAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.agents.List()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"agents": list})
	case http.MethodPost:
		var m model.AgentModel
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			writeJSON(w, 400, map[string]any{"error": "bad json"})
			return
		}
		if err := s.agents.Save(&m); err != nil {
			writeJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "id": m.ID})
	default:
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) apiAgentsByID(w http.ResponseWriter, r *http.Request) {
	id := atoi64(strings.TrimPrefix(r.URL.Path, "/api/agents/"))
	switch r.Method {
	case http.MethodDelete:
		if err := s.agents.Delete(id); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	case http.MethodGet:
		m, err := s.agents.Get(id)
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		if m == nil {
			writeJSON(w, 404, map[string]any{"error": "not found"})
			return
		}
		writeJSON(w, 200, m)
	case http.MethodPut, http.MethodPatch:
		// Partial update: only the fields present in the body are changed.
		var patch model.AgentModel
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeJSON(w, 400, map[string]any{"error": "bad json"})
			return
		}
		m, err := s.agents.Update(id, &patch)
		if err != nil {
			writeJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "agent": m})
	default:
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
	}
}

// ---------------- 系统配置 ----------------

func (s *Server) apiConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]any{
			"gui_port":       s.cfg.GUIPort,
			"api_port":       s.cfg.APIPort,
			"storage_mode":   s.cfg.StorageMode,
			"sqlite_path":    s.cfg.SQLitePath,
			"mysql_dsn":      s.cfg.MySQLDSN,
			"cache_mode":     s.cfg.CacheMode,
			"redis_addr":     s.cfg.RedisAddr,
			"redis_pass":     s.cfg.RedisPass,
			"redis_db":       s.cfg.RedisDB,
			"api_keys":       maskKeys(s.cfg.APIKeys),
			"gui_user":       s.cfg.GUIUser,
			"data_dir":       s.cfg.DataDir,
			"python_bin":     s.cfg.PythonBin,
			"image_engine":   s.cfg.ImageEngine,
			"delete_outputs": s.cfg.DeleteOutputs,
		})
		return
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		var b map[string]any
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			writeJSON(w, 400, map[string]any{"error": "bad json"})
			return
		}
		if v, ok := b["storage_mode"].(string); ok && (v == "sqlite" || v == "mysql") {
			s.cfg.StorageMode = v
		}
		if v, ok := b["cache_mode"].(string); ok && (v == "file" || v == "redis") {
			s.cfg.CacheMode = v
		}
		if v, ok := b["redis_addr"].(string); ok {
			s.cfg.RedisAddr = v
		}
		if v, ok := b["redis_pass"].(string); ok {
			s.cfg.RedisPass = v
		}
		if v, ok := b["mysql_dsn"].(string); ok {
			s.cfg.MySQLDSN = v
		}
		if v, ok := b["image_engine"].(string); ok && (v == "go" || v == "python") {
			s.cfg.ImageEngine = v
		}
		if v, ok := b["api_keys"].([]any); ok {
			ks := []string{}
			for _, x := range v {
				if s, ok := x.(string); ok && s != "" {
					ks = append(ks, s)
				}
			}
			if len(ks) > 0 {
				s.cfg.APIKeys = ks
			}
		}
		if v, ok := b["python_bin"].(string); ok && v != "" {
			s.cfg.PythonBin = v
		}
		// 生产完成并提取后是否删除产物（中间产物永远 100% 删除）。
		if v, ok := b["delete_outputs"].(bool); ok {
			s.cfg.DeleteOutputs = v
		}
		if err := s.cfg.Save(); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "note": "部分配置(存储/缓存模式)重启后生效"})
		return
	}
	writeJSON(w, 405, map[string]any{"error": "method not allowed"})
}

// ---------------- 修改 GUI 账号密码 ----------------

func (s *Server) apiUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	var b struct {
		Username    string `json:"username"`
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	// Allow changing username.
	if b.Username != "" && b.Username != s.cfg.GUIUser {
		s.cfg.GUIUser = b.Username
	}
	// Changing password requires the old password (unless none set yet).
	if b.NewPassword != "" {
		if s.cfg.GUIPassHash != "" && !config.CheckPassword(s.cfg.GUIPassHash, b.OldPassword) {
			writeJSON(w, 400, map[string]any{"error": "原密码错误"})
			return
		}
		h, err := config.HashPassword(b.NewPassword)
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		s.cfg.GUIPassHash = h
	}
	if err := s.cfg.Save(); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "username": s.cfg.GUIUser})
}

// ---------------- Playground ----------------

// apiPlayground 直接调用上游图片模型试玩（不经过 agent）。
// 支持 multipart（含参考图文件）与 JSON 两种提交方式。
func (s *Server) apiPlayground(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	var modelID int64
	var prompt, size string
	var n int
	var refPath string

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		_ = r.ParseMultipartForm(64 << 20)
		modelID = atoi64(r.FormValue("model_id"))
		prompt = r.FormValue("prompt")
		size = r.FormValue("size")
		n = int(atoi64(r.FormValue("n")))
		if fhs := r.MultipartForm.File["ref"]; len(fhs) > 0 {
			fh := fhs[0]
			f, err := fh.Open()
			if err == nil {
				defer f.Close()
				_ = os.MkdirAll(filepath.Join(s.cfg.DataDir, "uploads"), 0o755)
				dst := filepath.Join(s.cfg.DataDir, "uploads", fmt.Sprintf("pg_ref_%d_%s", time.Now().UnixNano(), config.SafeFileName(fh.Filename)))
				if out, err := os.Create(dst); err == nil {
					io.Copy(out, f)
					out.Close()
					refPath = dst
				}
			}
		}
	} else {
		var b struct {
			ModelID int64  `json:"model_id"`
			Prompt  string `json:"prompt"`
			Size    string `json:"size"`
			N       int    `json:"n"`
			Ref     string `json:"ref"` // 本地路径 / url / data uri
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			writeJSON(w, 400, map[string]any{"error": "bad json"})
			return
		}
		modelID, prompt, size, n, refPath = b.ModelID, b.Prompt, b.Size, b.N, b.Ref
	}

	if modelID == 0 || prompt == "" {
		writeJSON(w, 400, map[string]any{"error": "model_id 与 prompt 必填"})
		return
	}
	if n <= 0 {
		n = 1
	}
	um, err := s.up.Get(modelID)
	if err != nil || um == nil || um.Type != model.TypeImage {
		writeJSON(w, 400, map[string]any{"error": "图片模型不存在"})
		return
	}
	// 多尺寸支持：显式传了 size 就用它；未配置尺寸限制则放行任意值。
	if size == "" {
		size = um.Size
	}
	res, err := client.New().GenerateImage(client.ImageRequest{
		Model: *um, Prompt: prompt, N: n, Size: size, InputURL: refPath,
	})
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	// 下载到 outputs 并转成可访问的 /files/ URL。
	outDir := filepath.Join(s.cfg.DataDir, "outputs")
	_ = os.MkdirAll(outDir, 0o755)
	urls := []string{}
	for i, u := range res.URLs {
		ext := "png"
		if strings.Contains(u, ".jpg") || strings.Contains(u, ".jpeg") {
			ext = "jpg"
		}
		name := fmt.Sprintf("pg_%d_%d.%s", time.Now().UnixNano(), i, ext)
		dst := filepath.Join(outDir, name)
		if err := downloadToFile(u, dst); err == nil {
			urls = append(urls, "/files/"+name)
		}
	}
	if len(urls) == 0 {
		writeJSON(w, 502, map[string]any{"error": "上游返回了图片但全部下载失败"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "images": urls})
}

// serveFile 提供成品的静态访问。
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/files/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.cfg.DataDir, "outputs", name))
}

func downloadToFile(u, dst string) error {
	if strings.HasPrefix(u, "data:") {
		idx := strings.Index(u, ",")
		if idx < 0 {
			return fmt.Errorf("bad data url")
		}
		raw, err := base64.StdEncoding.DecodeString(u[idx+1:])
		if err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o644)
	}
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("download http %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// ---------------- helpers ----------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func maskKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if len(k) <= 8 {
			out = append(out, "******")
			continue
		}
		out = append(out, k[:4]+"****"+k[len(k)-4:])
	}
	return out
}

func atoi64(s string) int64 {
	var v int64
	fmt.Sscanf(strings.TrimSpace(s), "%d", &v)
	return v
}
