package skills

import (
	"strings"
	"testing"
)

func TestWrapSVG(t *testing.T) {
	frag := `<svg width="800" height="600" xmlns="http://www.w3.org/2000/svg"><rect width="800" height="600" fill="#0a0e1a"/></svg>`
	doc := wrapSVG(frag, 800, 600)
	for _, want := range []string{"<!doctype html>", "width:800px", "height:600px", "<svg"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("wrapSVG 缺少 %q: %s", want, doc)
		}
	}

	full := `<html><head><title>x</title></head><body>` + frag + `</body></html>`
	doc2 := wrapSVG(full, 800, 600)
	if !strings.Contains(doc2, "</head>") || !strings.Contains(doc2, "width:800px") {
		t.Fatalf("完整 HTML 文档应注入 reset 到 head: %s", doc2)
	}
	if strings.Count(doc2, "<html") != 1 {
		t.Fatalf("不应重复包 html: %s", doc2)
	}
}
