package skills

import (
	"os"
	"testing"
)

func TestRenderHTMLE2E(t *testing.T) {
	if findChrome("") == "" {
		t.Skip("环境无 Chrome，跳过端到端渲染")
	}
	html := `<!doctype html><html><head><meta charset="utf-8"></head><body style="background:#0a0e1a;color:#fff;font-family:sans-serif"><h1 style="font-size:72px">HTML 渲染测试</h1></body></html>`
	ws := t.TempDir()
	p, err := RenderHTML(html, "", ws, 800, 600)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	st, err := os.Stat(p)
	if err != nil || st.Size() == 0 {
		t.Fatalf("产出文件不存在或为空: %v", err)
	}
	t.Logf("渲染成功: %s (%d bytes)", p, st.Size())
}
