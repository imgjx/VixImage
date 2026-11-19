package skills

import (
	"strings"
	"testing"
)

func TestWrapHTML(t *testing.T) {
	// 片段 HTML：应被包成完整文档并注入 reset。
	frag := wrapHTML("<h1>Hi</h1>", 1080, 1920)
	if !strings.Contains(frag, "<html") || !strings.Contains(frag, "<h1>Hi</h1>") {
		t.Fatalf("片段未被包裹: %s", frag[:80])
	}
	if !strings.Contains(frag, "width:1080px") || !strings.Contains(frag, "min-height:1920px") {
		t.Fatalf("reset 尺寸未注入: %s", frag[:200])
	}

	// 完整文档：只在 </head> 后注入 reset，不重复包 <html>。
	full := "<!doctype html><html><head></head><body>BODY</body></html>"
	got := wrapHTML(full, 800, 600)
	if strings.Count(strings.ToLower(got), "<html") != 1 {
		t.Fatalf("完整文档被重复包裹: %s", got[:120])
	}
	iHead, iReset := strings.Index(strings.ToLower(got), "</head>"), strings.Index(got, "<style>")
	if iReset < 0 || iReset > iHead {
		t.Fatalf("reset 未插入 </head> 前: %s", got[:160])
	}
}

func TestFindChrome(t *testing.T) {
	// 显式路径：即使不存在，LookPath/Stat 都不应 panic；环境装了则应能找到。
	got := findChrome("")
	t.Logf("findChrome(\"\") = %q", got)
	// 不强制断言非空（CI 环境可能没有浏览器），只验证函数可调用且不异常。
}
