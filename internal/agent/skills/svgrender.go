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

// RenderSVG 把 SVG 字符串用无头 Chrome 渲染成 PNG，返回输出文件路径。
// chromeBin 为空时自动探测；w/h 为期望画布尺寸（200~4096）。
// 渲染失败一律报错，不返回空图或占位图（无兜底）。
//
// 与 htmlrender 的区别：SVG 是纯矢量图形，适合精确几何图形、图标、图表、
// 文字路径等；包成 HTML 文档后由 Chrome 光栅化，缩放不失真。
func RenderSVG(svg, chromeBin, workDir string, w, h int) (string, error) {
	s := strings.TrimSpace(svg)
	if s == "" {
		return "", fmt.Errorf("SVG 内容为空")
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

	doc := wrapSVG(s, w, h)
	svgPath := filepath.Join(workDir, fmt.Sprintf("svg_%d.html", time.Now().UnixNano()))
	if err := os.WriteFile(svgPath, []byte(doc), 0o644); err != nil {
		return "", fmt.Errorf("写入 SVG 失败: %w", err)
	}
	outPath := filepath.Join(workDir, fmt.Sprintf("svg_%d.png", time.Now().UnixNano()))

	args := []string{
		"--headless=new", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
		"--hide-scrollbars", "--force-device-scale-factor=1", "--user-agent=" + htmlRenderUA,
		fmt.Sprintf("--window-size=%d,%d", w, h),
		"--screenshot=" + outPath,
		"file://" + filepath.ToSlash(svgPath),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("SVG 渲染超时（60s）")
		}
		// 老版本 Chrome 不认 --headless=new，退回 --headless 再试一次。
		args[0] = "--headless"
		cmd2 := exec.CommandContext(context.Background(), bin, args...)
		output2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return "", fmt.Errorf("SVG 渲染失败: %w (%s)", err, truncate(strings.TrimSpace(string(output)+string(output2)), 300))
		}
	}
	if st, statErr := os.Stat(outPath); statErr != nil || st.Size() == 0 {
		return "", fmt.Errorf("SVG 渲染未产出图片: %s", truncate(strings.TrimSpace(string(output)), 200))
	}
	return outPath, nil
}

// wrapSVG 把 SVG 包成定尺寸文档。若输入已是完整 HTML 文档则只注入 reset 样式；
// 否则按 SVG 片段处理，并确保根节点带 xmlns（内联到 HTML 时可省略，但保留更稳）。
func wrapSVG(svg string, w, h int) string {
	reset := fmt.Sprintf(`<style>html,body{margin:0;padding:0;width:%dpx;height:%dpx;overflow:hidden;}body{line-height:0;}svg{display:block;}</style>`, w, h)
	lower := strings.ToLower(svg)
	if strings.Contains(lower, "<html") {
		if i := strings.Index(lower, "</head>"); i >= 0 {
			return svg[:i] + reset + svg[i:]
		}
		return reset + svg
	}
	if !strings.Contains(lower, "<svg") {
		// 明确报错由调用前的校验负责；这里保守地当作片段交给渲染器。
	}
	return "<!doctype html><html><head><meta charset=\"utf-8\">" + reset + "</head><body>" + svg + "</body></html>"
}
