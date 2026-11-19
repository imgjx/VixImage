package skills

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const htmlRenderUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// findChrome 定位可用的 Chromium/Chrome 可执行文件。
// 优先用显式配置的路径，其次按常见命令名顺序探测。
func findChrome(explicit string) string {
	candidates := []string{}
	if s := strings.TrimSpace(explicit); s != "" {
		candidates = append(candidates, s)
	}
	candidates = append(candidates,
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome",
		"/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	)
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil && p != "" {
			return p
		}
		if strings.ContainsAny(c, `/\\`) {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c
			}
		}
	}
	return ""
}

// RenderHTML 把 HTML 字符串用无头 Chrome 渲染成 PNG，返回输出文件路径。
// chromeBin 为空时自动探测；w/h 为期望画布尺寸（200~4096）。
// 渲染失败一律报错，不返回空图或占位图（无兜底）。
func RenderHTML(html, chromeBin, workDir string, w, h int) (string, error) {
	if strings.TrimSpace(html) == "" {
		return "", fmt.Errorf("HTML 内容为空")
	}
	bin := findChrome(chromeBin)
	if bin == "" {
		return "", fmt.Errorf("未找到 Chrome/Chromium，请安装浏览器或在配置里指定 chrome_bin 路径")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", err
	}
	if w <= 0 {
		w = 1080
	}
	if h <= 0 {
		h = 1920
	}

	// 包裹一层 reset：抹掉默认外边距、锁定视口尺寸，避免白边/滚动条让成片错位。
	doc := wrapHTML(html, w, h)
	htmlPath := filepath.Join(workDir, fmt.Sprintf("render_%d.html", time.Now().UnixNano()))
	if err := os.WriteFile(htmlPath, []byte(doc), 0o644); err != nil {
		return "", fmt.Errorf("写入 HTML 失败: %w", err)
	}
	outPath := filepath.Join(workDir, fmt.Sprintf("render_%d.png", time.Now().UnixNano()))

	args := []string{
		"--headless=new", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
		"--hide-scrollbars", "--force-device-scale-factor=1", "--user-agent=" + htmlRenderUA,
		fmt.Sprintf("--window-size=%d,%d", w, h),
		"--screenshot=" + outPath,
		"file://" + filepath.ToSlash(htmlPath),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("HTML 渲染超时（60s）")
		}
		// 老版本 Chrome 不认 --headless=new，退回 --headless 再试一次。
		args[0] = "--headless"
		cmd2 := exec.CommandContext(context.Background(), bin, args...)
		output2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return "", fmt.Errorf("HTML 渲染失败: %w (%s)", err, truncate(strings.TrimSpace(string(output)+string(output2)), 300))
		}
	}
	if st, statErr := os.Stat(outPath); statErr != nil || st.Size() == 0 {
		return "", fmt.Errorf("HTML 渲染未产出图片: %s", truncate(strings.TrimSpace(string(output)), 200))
	}
	return outPath, nil
}

// wrapHTML 把用户 HTML 包成适合定尺寸截图的文档，并注入 reset 样式。
// 已是完整文档（含 <html>）时只注入 style，不重复包一层。
func wrapHTML(html string, w, h int) string {
	reset := fmt.Sprintf(`<style>html,body{margin:0;padding:0;width:%dpx;min-height:%dpx;}body{overflow:hidden;}*{box-sizing:border-box;}</style>`, w, h)
	lower := strings.ToLower(html)
	if strings.Contains(lower, "<html") {
		if i := strings.Index(lower, "</head>"); i >= 0 {
			return html[:i] + reset + html[i:]
		}
		if i := strings.Index(lower, "<body"); i >= 0 {
			if gt := strings.IndexByte(html[i:], '>'); gt >= 0 {
				p := i + gt + 1
				return html[:p] + reset + html[p:]
			}
		}
		return reset + html
	}
	return "<!doctype html><html><head><meta charset=\"utf-8\">" + reset + "</head><body>" + html + "</body></html>"
}
