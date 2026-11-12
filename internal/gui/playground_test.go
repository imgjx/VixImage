package gui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"viximage/internal/agent"
	"viximage/internal/agentmodel"
	"viximage/internal/cache"
	"viximage/internal/config"
	"viximage/internal/model"
	"viximage/internal/storage"
	"viximage/internal/upstream"
)

// fakeUpstream 模拟一个 OpenAI 兼容上游：
//   - /v1/chat/completions 返回规划好的 Agent 轮次 JSON
//   - /images/generations 返回一张 8x8 纯色 PNG（base64）
type fakeUpstream struct {
	srv      *httptest.Server
	chatCall int32
}

func newFakeUpstream(t *testing.T) *fakeUpstream {
	t.Helper()
	f := &fakeUpstream{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&f.chatCall, 1)
		// 第 1 轮：建画布 + 用参考图生图 + 贴图 + finish；之后：直接 finish。
		turn := `{"think":"排版","tools":[{"t":"canvas","w":600,"h":800,"bg":"#101020"},
			{"t":"gen","id":"g1","p":"把参考图融进海报","ref":0},
			{"t":"add","layer":{"k":"image","src":"g1","x":50,"y":100,"w":500,"h":300}},
			{"t":"finish"}]}`
		if n > 1 {
			turn = `{"think":"确认交付","tools":[{"t":"finish"}]}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, turn)
	})
	mux.HandleFunc("/images/generations", func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for x := 0; x < 8; x++ {
			for y := 0; y < 8; y++ {
				img.Set(x, y, color.RGBA{R: 200, G: 80, B: 40, A: 255})
			}
		}
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString(buf.Bytes()))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// TestPlaygroundAgentRunsMyModel 端到端验证：Playground 选「我的模型」能跑完整 Agent 链路并拿到成片。
func TestPlaygroundAgentRunsMyModel(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.SQLitePath = filepath.Join(dir, "t.db")
	cfg.CacheFileDir = filepath.Join(dir, "cache")
	cfg.GUIUser = "admin"
	h, _ := config.HashPassword("admin123")
	cfg.GUIPassHash = h

	store, err := storage.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	fu := newFakeUpstream(t)
	up := upstream.New(store)
	// 1) 注册上游 LLM（多模态）与图片模型。
	llm := &model.UpstreamModel{
		Name: "fake-llm", Type: model.TypeLLM, Format: model.FormatOpenAIChat,
		BaseURL: fu.srv.URL, APIKey: "k", ModelName: "fake-chat", Multimodal: true, Enabled: true,
	}
	if err := up.Save(llm); err != nil {
		t.Fatal(err)
	}
	img := &model.UpstreamModel{
		Name: "fake-img", Type: model.TypeImage, Format: model.FormatOpenAI,
		BaseURL: fu.srv.URL, APIKey: "k", ModelName: "fake-image",
		Size: "512x512", N: 1, Enabled: true,
	}
	if err := up.Save(img); err != nil {
		t.Fatal(err)
	}
	// 2) 创建「我的模型」。
	ams := agentmodel.New(store, up)
	am := &model.AgentModel{
		Name: "海报师", CallName: "poster", BaseModelID: llm.ID, MaxSteps: 10,
		ImageModels: []model.ImageModelUse{{UpstreamID: img.ID, Steps: 2}},
		Enabled:     true,
	}
	if err := ams.Save(am); err != nil {
		t.Fatal(err)
	}

	rt := agent.New(cfg, store, up, cache.Open(cfg))
	g := New(cfg, store, up, ams, rt, cache.Open(cfg))
	srv := httptest.NewServer(g.Handler())
	defer srv.Close()

	// 3) 登录拿会话。
	jar := login(t, srv.URL)
	// 4) 调 Playground（multipart，带一张参考图）。
	refImg := makePNG(t, 16, 16)
	body, ct := playgroundBody(t, am.ID, "做一张海报", [][]byte{refImg})
	req, _ := http.NewRequest("POST", srv.URL+"/api/playground/agent", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(jar)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		OK      bool     `json:"ok"`
		Error   string   `json:"error"`
		Images  []string `json:"images"`
		Log     []string `json:"log"`
		Steps   int      `json:"steps_used"`
		Status  string   `json:"status"`
		Trace   []model.TraceStep `json:"trace"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatalf("Playground 运行失败: %s ｜ 日志: %v", out.Error, out.Log)
	}
	if len(out.Images) == 0 {
		t.Fatalf("未产出图片 ｜ 日志: %v", out.Log)
	}
	if out.Status == "" {
		t.Fatal("缺少 status")
	}
	// 5) 成片文件必须真实存在。
	name := strings.TrimPrefix(out.Images[0], "/files/")
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "outputs", name)); err != nil {
		t.Fatalf("成片文件不存在: %v", err)
	}
	// 6) 轨迹里应有步数记录。
	if len(out.Trace) == 0 {
		t.Fatal("缺少 trace 轨迹")
	}
}

// login 走原生表单登录，返回会话 cookie。
func login(t *testing.T, base string) *http.Cookie {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	cl := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := cl.PostForm(base+"/login", url.Values{"username": {"admin"}, "password": {"admin123"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 302 {
		t.Fatalf("登录 = %d，期望 302", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "vix_sid" {
			return c
		}
	}
	t.Fatal("未拿到会话 cookie")
	return nil
}

// makePNG 生成一张测试用 PNG。
func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 30, G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// playgroundBody 构造 Playground 的 multipart 请求体。
func playgroundBody(t *testing.T, agentID int64, prompt string, refs [][]byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	_ = w.WriteField("agent_id", fmt.Sprintf("%d", agentID))
	_ = w.WriteField("prompt", prompt)
	for i, r := range refs {
		fw, err := w.CreateFormFile("refs", fmt.Sprintf("ref%d.png", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf, w.FormDataContentType()
}
