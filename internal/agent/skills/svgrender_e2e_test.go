package skills

import (
	"os"
	"testing"
)

func TestRenderSVGE2E(t *testing.T) {
	if findChrome("") == "" {
		t.Skip("环境无 Chrome，跳过端到端 SVG 渲染")
	}
	svg := `<svg width="800" height="600" xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#0a0e1a"/><stop offset="1" stop-color="#1b2b4a"/></linearGradient></defs><rect width="800" height="600" fill="url(#g)"/><circle cx="400" cy="300" r="160" fill="#7ee2a8"/><text x="400" y="320" font-size="48" fill="#fff" text-anchor="middle">SVG</text></svg>`
	ws := t.TempDir()
	p, err := RenderSVG(svg, "", ws, 800, 600)
	if err != nil {
		t.Fatalf("SVG 渲染失败: %v", err)
	}
	st, err := os.Stat(p)
	if err != nil || st.Size() == 0 {
		t.Fatalf("产出文件不存在或为空: %v", err)
	}
	t.Logf("SVG 渲染成功: %s (%d bytes)", p, st.Size())
}
