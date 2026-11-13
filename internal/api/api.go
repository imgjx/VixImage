package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"viximage/internal/agent"
	"viximage/internal/cache"
	"viximage/internal/config"
	"viximage/internal/httplog"
	"viximage/internal/model"
	"viximage/internal/storage"
	"viximage/internal/upstream"
)

// Server is the API surface. It is protected by apikey auth and exposes the
// "invoke my model -> create an agent that makes images" workflow.
type Server struct {
	cfg     *config.Config
	store   storage.Storage
	up      *upstream.Service
	agent   *agent.Runtime
	cache   cache.Cache
	outputs string
	uploads string
}

func New(cfg *config.Config, store storage.Storage, up *upstream.Service, ag *agent.Runtime, cch cache.Cache) *Server {
	return &Server{
		cfg:     cfg,
		store:   store,
		up:      up,
		agent:   ag,
		cache:   cch,
		outputs: filepath.Join(cfg.DataDir, "outputs"),
		uploads: filepath.Join(cfg.DataDir, "uploads"),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/v1/agents", s.listAgents)
	mux.HandleFunc("/v1/agents/run", s.runAgent)
	mux.HandleFunc("/v1/upload", s.upload)
	mux.HandleFunc("/v1/upstream", s.listUpstream)
	mux.HandleFunc("/v1/files/", s.serveFile)
	return mux
}

func (s *Server) Start() error {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.APIPort),
		Handler:      httplog.WithAccessLog("API", s.handlerWithAuth(s.Handler())),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}
	return srv.ListenAndServe()
}

// handlerWithAuth enforces apikey on every route except /health.
func (s *Server) handlerWithAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		key := apiKeyFrom(r)
		if key == "" || !s.validKey(key) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or missing apikey"})
			return
		}
		// 命中有效密钥：累加该密钥的调用次数与最近调用时间。
		_ = s.store.IncrAPIKeyCalls(key)
		next.ServeHTTP(w, r)
	})
}

// validKey 校验密钥是否有效（存在且启用）。
func (s *Server) validKey(key string) bool {
	k, err := s.store.GetAPIKey(key)
	if err != nil || k == nil {
		return false
	}
	return k.Enabled
}

func apiKeyFrom(r *http.Request) string {
	if h := r.Header.Get("X-API-Key"); h != "" {
		return h
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "service": "viximage-api", "port": s.cfg.APIPort})
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	list, err := s.store.ListAgentModels()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, map[string]any{
			"id": m.ID, "name": m.Name, "call_name": m.CallName,
			"max_steps": m.MaxSteps, "base_model_id": m.BaseModelID,
			"image_models": m.ImageModels, "enabled": m.Enabled,
			"style": m.Style, "prompt": m.Prompt,
		})
	}
	writeJSON(w, 200, map[string]any{"agents": out})
}

func (s *Server) listUpstream(w http.ResponseWriter, r *http.Request) {
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
}

type runBody struct {
	CallName string   `json:"call_name"`
	Prompt   string   `json:"prompt"`
	Images   []string `json:"images"` // uploaded file ids or urls（兼作参考图）
	Webhook  string   `json:"webhook"`
	// DeepThink forces the deep-thinking planning mode for this call,
	// overriding the agent model's own setting.
	DeepThink bool `json:"deep_think"`
	// Size 强制本次请求尺寸（如 "1280x720"），覆盖生图画幅映射。
	Size string `json:"size"`
	// Trace 是 VixImage 专属参数：为 true 时响应附带每一步的短 ThinkFlag
	// （只含 step + think，不暴露该步用了哪些技能）。
	Trace bool `json:"trace"`
}

func (s *Server) runAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	var b runBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	if b.CallName == "" || b.Prompt == "" {
		writeJSON(w, 400, map[string]any{"error": "call_name 与 prompt 必填"})
		return
	}
	inputs := s.resolveInputs(b.Images)
	res, err := s.agent.Run(agent.RunRequest{
		AgentCallName: b.CallName,
		Prompt:        b.Prompt,
		InputImages:   inputs,
		APIKeyMask:    maskKey(apiKeyFrom(r)),
		Webhook:       b.Webhook,
		DeepThink:     b.DeepThink,
		Size:          b.Size,
		Trace:         b.Trace,
	})
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error(), "log": res})
		return
	}
	// rewrite local paths to downloadable urls
	urls := make([]string, 0, len(res.OutputImages))
	for _, p := range res.OutputImages {
		urls = append(urls, "/v1/files/"+filepath.Base(p))
	}
	out := map[string]any{
		"status":        res.Status,
		"output_images": urls,
		"steps_used":    res.StepsUsed,
		"llm_calls":     res.LLMCalls,
		"log":           res.Log,
		"theme":         res.Theme,
		"deep_think":    res.DeepThink,
		"style":         res.Style,
		"workspace":     res.Workspace,
		"exhausted":     res.Exhausted,
		"partial":       res.Partial,
	}
	// 专属参数 trace：附带每一步的短 ThinkFlag，不含具体技能。
	if b.Trace {
		steps := make([]map[string]any, 0, len(res.Trace))
		for _, t := range res.Trace {
			steps = append(steps, map[string]any{"step": t.Step, "think": t.Think})
		}
		out["trace"] = steps
		out["reasoning"] = res.Reasoning
	}
	writeJSON(w, 200, out)
}

// resolveInputs turns uploaded ids ("up_<id>") or urls into local file paths.
func (s *Server) resolveInputs(images []string) []string {
	out := []string{}
	for _, id := range images {
		if strings.HasPrefix(id, "up_") {
			p := filepath.Join(s.uploads, strings.TrimPrefix(id, "up_"))
			if _, err := os.Stat(p); err == nil {
				out = append(out, p)
			}
			continue
		}
		// treat as url and download locally
		if strings.HasPrefix(id, "http") {
			dst := filepath.Join(s.uploads, fmt.Sprintf("dl_%d", time.Now().UnixNano()))
			if downloadTo(id, dst) == nil {
				out = append(out, dst)
			}
		}
	}
	return out
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	_ = r.ParseMultipartForm(32 << 20)
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeJSON(w, 400, map[string]any{"error": "no file"})
		return
	}
	_ = os.MkdirAll(s.uploads, 0o755)
	res := []string{}
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			continue
		}
		dst := filepath.Join(s.uploads, fmt.Sprintf("up_%d_%s", time.Now().UnixNano(), config.SafeFileName(fh.Filename)))
		out, err := os.Create(dst)
		if err != nil {
			f.Close()
			continue
		}
		io.Copy(out, f)
		out.Close()
		f.Close()
		res = append(res, "up_"+filepath.Base(dst))
	}
	writeJSON(w, 200, map[string]any{"uploaded": res})
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v1/files/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	p := filepath.Join(s.outputs, name)
	http.ServeFile(w, r, p)
}

// ---------------- helpers ----------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func maskKey(k string) string {
	if len(k) <= 8 {
		return "******"
	}
	return k[:4] + "****" + k[len(k)-4:]
}

func downloadTo(u, dst string) error {
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
