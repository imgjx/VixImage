package skills

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"viximage/internal/client"
	"viximage/internal/model"
)

// Produce is the 图像生产技能: it asks an upstream image model to generate images
// from a prompt and downloads the results locally. This mirrors the poster
// workflow of generating hero assets via the image API.
//
// sizeOverride lets the caller request a specific canvas (e.g. "16:9") so the
// generated asset already matches the target layout instead of being square.
func (c *Context) Produce(model model.UpstreamModel, prompt, inputURL string, sizeOverride string) ([]string, error) {
	c.logf("[生产] 调用上游图片模型 %s 生成: %s", model.Name, truncate(prompt, 60))
	size := model.Size
	if sizeOverride != "" {
		size = sizeOverride
	}
	res, err := c.Client.GenerateImage(client.ImageRequest{
		Model:    model,
		Prompt:   prompt,
		N:        model.N,
		Size:     size,
		InputURL: inputURL,
	})
	if err != nil {
		return nil, fmt.Errorf("生产技能失败: %w", err)
	}
	var out []string
	for i, u := range res.URLs {
		ext := "png"
		if strings.Contains(u, ".jpg") || strings.Contains(u, ".jpeg") {
			ext = "jpg"
		}
		dst := filepath.Join(c.OutDir, fmt.Sprintf("prod_%d_%d.%s", time.Now().UnixNano(), i, ext))
		if err := download(u, dst); err != nil {
			return nil, err
		}
		out = append(out, dst)
		c.logf("[生产] 已下载 %s", dst)
	}
	return out, nil
}

func download(u, dst string) error {
	if strings.HasPrefix(u, "data:") {
		// data:image/png;base64,....
		idx := strings.Index(u, ",")
		if idx < 0 {
			return fmt.Errorf("bad data url")
		}
		b64 := u[idx+1:]
		raw, err := base64.StdEncoding.DecodeString(b64)
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

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "..."
}

var _ = bytes.MinRead
