package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// StockImage 是免版权图库搜索结果的一条。
type StockImage struct {
	URL     string `json:"url"`     // 原图直链（可下载）
	Thumb   string `json:"thumb"`   // 缩略图
	Title   string `json:"title"`   // 标题 / 关键词
	Author  string `json:"author"`  // 作者
	License string `json:"license"` // 授权信息
	Source  string `json:"source"`  // 来源图库
}

// StockConfig 各图库访问配置。Openverse 免密钥；Pexels / Pixabay 需各自 API Key。
type StockConfig struct {
	Pexels        string
	Pixabay       string
	OpenverseBase string // 可选：自定义 Openverse 兼容端点
}

const stockUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// SearchStock 从免版权图库搜索图片。
// site 取值：openverse（默认，CC 聚合、无需密钥）/ pexels / pixabay。
// n 为期望张数（1~8）。返回已过滤掉无地址条目的结果。
func SearchStock(site, query string, n int, cfg StockConfig) ([]StockImage, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("素材搜索关键词为空")
	}
	if n <= 0 {
		n = 1
	}
	if n > 8 {
		n = 8
	}
	site = strings.ToLower(strings.TrimSpace(site))
	if site == "" {
		site = "openverse"
	}

	cli := &http.Client{Timeout: 20 * time.Second}
	var imgs []StockImage
	var err error
	switch site {
	case "openverse", "open", "cc", "openverse.org":
		base := strings.TrimSpace(cfg.OpenverseBase)
		if base == "" {
			base = "https://api.openverse.org"
		}
		u := strings.TrimRight(base, "/") + "/v1/images/?q=" + url.QueryEscape(query) +
			"&page_size=" + fmt.Sprintf("%d", n*2) + "&license_type=commercial"
		var raw []byte
		raw, err = stockGet(cli, u, nil)
		if err == nil {
			imgs, err = parseOpenverse(raw)
		}
	case "pexels":
		if strings.TrimSpace(cfg.Pexels) == "" {
			return nil, fmt.Errorf("未配置 Pexels API Key（请在系统配置里填写）")
		}
		u := "https://api.pexels.com/v1/search?query=" + url.QueryEscape(query) +
			"&per_page=" + fmt.Sprintf("%d", n*2)
		var raw []byte
		raw, err = stockGet(cli, u, map[string]string{"Authorization": strings.TrimSpace(cfg.Pexels)})
		if err == nil {
			imgs, err = parsePexels(raw)
		}
	case "pixabay":
		if strings.TrimSpace(cfg.Pixabay) == "" {
			return nil, fmt.Errorf("未配置 Pixabay API Key（请在系统配置里填写）")
		}
		u := "https://pixabay.com/api/?key=" + url.QueryEscape(strings.TrimSpace(cfg.Pixabay)) +
			"&q=" + url.QueryEscape(query) + "&image_type=photo&per_page=" + fmt.Sprintf("%d", n*2)
		var raw []byte
		raw, err = stockGet(cli, u, nil)
		if err == nil {
			imgs, err = parsePixabay(raw)
		}
	default:
		return nil, fmt.Errorf("未知图库 %q（可选 openverse / pexels / pixabay）", site)
	}
	if err != nil {
		return nil, err
	}
	// 过滤无直链的条目，再截断到期望张数。
	out := make([]StockImage, 0, n)
	for _, im := range imgs {
		if strings.TrimSpace(im.URL) == "" {
			continue
		}
		out = append(out, im)
		if len(out) >= n {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("图库 %q 搜索 %q 没有可用图片", site, query)
	}
	return out, nil
}

func stockGet(cli *http.Client, u string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", stockUA)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("访问图库失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("图库接口 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	return raw, nil
}

// parseOpenverse 解析 Openverse /v1/images 响应。
func parseOpenverse(raw []byte) ([]StockImage, error) {
	var d struct {
		Results []struct {
			URL       string `json:"url"`
			Thumbnail string `json:"thumbnail"`
			Title     string `json:"title"`
			Creator   string `json:"creator"`
			License   string `json:"license"`
			Source    string `json:"source"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("解析 Openverse 响应失败: %w", err)
	}
	out := make([]StockImage, 0, len(d.Results))
	for _, r := range d.Results {
		out = append(out, StockImage{URL: r.URL, Thumb: r.Thumbnail, Title: r.Title,
			Author: r.Creator, License: r.License, Source: orStr(r.Source, "openverse")})
	}
	return out, nil
}

// parsePexels 解析 Pexels /v1/search 响应。
func parsePexels(raw []byte) ([]StockImage, error) {
	var d struct {
		Photos []struct {
			ID           int64  `json:"id"`
			Photographer string `json:"photographer"`
			URL          string `json:"url"`
			Src          struct {
				Original string `json:"original"`
				Medium   string `json:"medium"`
			} `json:"src"`
		} `json:"photos"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("解析 Pexels 响应失败: %w", err)
	}
	out := make([]StockImage, 0, len(d.Photos))
	for _, r := range d.Photos {
		u := r.Src.Original
		if u == "" {
			u = r.Src.Medium
		}
		out = append(out, StockImage{URL: u, Thumb: r.Src.Medium, Title: fmt.Sprintf("pexels-%d", r.ID),
			Author: r.Photographer, License: "Pexels License（免费商用）", Source: "pexels"})
	}
	return out, nil
}

// parsePixabay 解析 Pixabay /api/ 响应。
func parsePixabay(raw []byte) ([]StockImage, error) {
	var d struct {
		Hits []struct {
			WebURL    string `json:"webformatURL"`
			LargeURL  string `json:"largeImageURL"`
			Preview   string `json:"previewURL"`
			User      string `json:"user"`
			Tags      string `json:"tags"`
			ImageType string `json:"type"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("解析 Pixabay 响应失败: %w", err)
	}
	out := make([]StockImage, 0, len(d.Hits))
	for _, r := range d.Hits {
		u := r.LargeURL
		if u == "" {
			u = r.WebURL
		}
		out = append(out, StockImage{URL: u, Thumb: r.Preview, Title: r.Tags,
			Author: r.User, License: "Pixabay License（免费商用）", Source: "pixabay"})
	}
	return out, nil
}

func orStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
