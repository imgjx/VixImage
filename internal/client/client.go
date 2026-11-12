package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"viximage/internal/model"
)

// ImageRequest is a request to an upstream image-generation model.
type ImageRequest struct {
	Model    model.UpstreamModel
	Prompt   string
	N        int
	Size     string
	InputURL string // optional reference image (image-to-image); local path or url
	// ResponseFormat overrides the requested wire format ("url" | "b64_json").
	ResponseFormat string
	// Extra carries extra provider-specific body fields.
	Extra map[string]any
}

// ImageResult holds the generated image URLs.
type ImageResult struct {
	URLs []string
}

// ChatMessage is a single chat turn. Content may be plain text or, for vision
// models, a list of parts.
type ChatMessage struct {
	Role    string
	Content any // string | []ChatPart
}

type ChatPart struct {
	Type     string // "text" | "image_url"
	Text     string
	ImageURL string
}

type ChatRequest struct {
	Model    model.UpstreamModel
	Messages []ChatMessage
	Temp     float64
	// Extra carries additional provider-specific fields (e.g. max_tokens).
	Extra map[string]any
}

// Client performs HTTP calls to upstream models in OpenAI / Agnes compatible shapes.
type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 10 * time.Minute}}
}

// GenerateImage calls the upstream image model and returns the produced URLs.
func (c *Client) GenerateImage(req ImageRequest) (*ImageResult, error) {
	n := req.N
	if n <= 0 {
		n = req.Model.N
	}
	if n <= 0 {
		n = 1
	}
	size := req.Size
	if size == "" {
		size = req.Model.Size
	}
	if size == "" {
		size = "1024x1024"
	}

	body := map[string]any{
		"model":  req.Model.ModelName,
		"prompt": req.Prompt,
		"n":      n,
		"size":   size,
	}
	// "完美格式支持"：默认请求 b64_json（部分网关不支持 url 回传），
	// 但显式指定了 ResponseFormat 时以调用方为准。
	if rf := req.ResponseFormat; rf == "url" || rf == "b64_json" {
		body["response_format"] = rf
	} else {
		body["response_format"] = "b64_json"
	}
	if req.InputURL != "" {
		// 支持本地路径参考图：自动转为 data URI。
		ref := req.InputURL
		if !strings.HasPrefix(ref, "http") && !strings.HasPrefix(ref, "data:") {
			if uri, err := fileToDataURI(ref); err == nil {
				ref = uri
			}
		}
		body["image"] = ref // OpenAI/Agnes 兼容 i2i 字段
	}
	// 反向提示词（图片模型专属）：留空则不发送该字段，避免上游因未知字段报错。
	if np := strings.TrimSpace(req.Model.NegativePrompt); np != "" {
		body["negative_prompt"] = np
	}
	for k, v := range req.Extra {
		body[k] = v
	}

	var endpoint string
	switch req.Model.Format {
	case model.FormatAgnes:
		endpoint = imagesEndpoint(req.Model.BaseURL, "/v1/images/generations")
	default: // openai
		endpoint = imagesEndpoint(req.Model.BaseURL, "/images/generations")
	}

	data, err := c.postJSON(endpoint, req.Model.APIKey, body)
	if err != nil {
		return nil, err
	}
	urls, err := extractImageURLs(data)
	if err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("upstream returned no image urls: %s", truncate(jstr(data), 300))
	}
	return &ImageResult{URLs: urls}, nil
}

// imagesEndpoint builds the image generations URL, tolerating a base URL that
// already ends with "/v1" (Agnes style) so we never emit "/v1/v1/...".
func imagesEndpoint(base, suffix string) string {
	b := strings.TrimRight(base, "/")
	if strings.HasSuffix(b, "/images/generations") {
		return b
	}
	if strings.HasSuffix(b, "/v1") && strings.HasPrefix(suffix, "/v1") {
		return b + strings.TrimPrefix(suffix, "/v1")
	}
	return b + suffix
}

// encodeContent converts a message content value into the OpenAI wire shape.
// Plain strings pass through; a []ChatPart becomes the multi-part array form
// used for vision requests.
func encodeContent(content any) any {
	switch v := content.(type) {
	case string:
		return v
	case []ChatPart:
		parts := make([]map[string]any, 0, len(v))
		for _, p := range v {
			switch p.Type {
			case "image_url":
				parts = append(parts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": p.ImageURL},
				})
			default:
				parts = append(parts, map[string]any{
					"type": "text",
					"text": p.Text,
				})
			}
		}
		return parts
	case []map[string]any:
		return v
	default:
		return content
	}
}

// chatEndpoint builds the chat completions URL. It tolerates a base URL that
// already includes the "/v1" suffix (Agnes style), avoiding "/v1/v1/...".
func chatEndpoint(base string) string {
	b := strings.TrimRight(base, "/")
	if strings.HasSuffix(b, "/chat/completions") {
		return b
	}
	if strings.HasSuffix(b, "/v1") {
		return b + "/chat/completions"
	}
	return b + "/v1/chat/completions"
}

// Chat calls the upstream multimodal LLM and returns the assistant text.
func (c *Client) Chat(req ChatRequest) (string, error) {
	temp := req.Temp
	if temp == 0 {
		temp = 0.7
	}
	msgs := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]any{"role": m.Role, "content": encodeContent(m.Content)})
	}
	body := map[string]any{
		"model":       req.Model.ModelName,
		"messages":    msgs,
		"temperature": temp,
	}
	for k, v := range req.Extra {
		body[k] = v
	}
	// max_tokens 由模型配置提供（小上下文模型尤其需要）。Extra 显式指定时优先。
	if _, set := body["max_tokens"]; !set {
		mt := req.Model.MaxTokens
		if mt <= 0 {
			mt = 2400 // 内置缺省上限，避免小上下文模型被撑爆
		}
		body["max_tokens"] = mt
	}
	// top_p：LLM 专属字段，0 = 不注入（走上游默认）。
	if _, set := body["top_p"]; !set && req.Model.TopP > 0 {
		body["top_p"] = req.Model.TopP
	}
	// system：模型自带的系统提示词前缀（LLM 专属）。
	if _, set := body["system"]; !set && strings.TrimSpace(req.Model.SystemPrompt) != "" {
		body["system"] = req.Model.SystemPrompt
	}
	endpoint := chatEndpoint(req.Model.BaseURL)
	data, err := c.postJSON(endpoint, req.Model.APIKey, body)
	if err != nil {
		return "", err
	}
	// OpenAI-compatible: choices[0].message.content
	if ch, ok := data["choices"].([]any); ok && len(ch) > 0 {
		if m, ok := ch[0].(map[string]any)["message"].(map[string]any); ok {
			if t, ok := m["content"].(string); ok {
				return t, nil
			}
		}
	}
	if errMsg, ok := data["error"].(map[string]any); ok {
		return "", fmt.Errorf("llm error: %v", errMsg["message"])
	}
	return "", fmt.Errorf("unexpected llm response: %s", truncate(jstr(data), 300))
}

func (c *Client) postJSON(url, apiKey string, body map[string]any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("upstream http %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode: %w (%s)", err, truncate(string(raw), 200))
	}
	return out, nil
}

// extractImageURLs 解析各家图片 API 的常见响应格式：
//   - OpenAI:  data[].url / data[].b64_json
//   - Agnes:   data[].url / 顶层 url
//   - 网关变体: images[] / output[].url / data[].image 等
func extractImageURLs(data map[string]any) ([]string, error) {
	var urls []string

	// 标准与变体：data 数组
	if arr, ok := data["data"].([]any); ok {
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				// 数组里直接是字符串 url
				if s, ok := item.(string); ok && strings.HasPrefix(s, "http") {
					urls = append(urls, s)
				}
				continue
			}
			if u := firstStr(m, "url", "image", "image_url", "b64_json"); u != "" {
				urls = append(urls, normalizeImage(m, u))
			}
			// 嵌套 image_url: {url: ...}
			if nested, ok := m["image_url"].(map[string]any); ok {
				if u, ok := nested["url"].(string); ok && u != "" {
					urls = append(urls, u)
				}
			}
		}
	}
	// images 数组（部分网关）
	if len(urls) == 0 {
		if arr, ok := data["images"].([]any); ok {
			for _, item := range arr {
				switch v := item.(type) {
				case string:
					urls = append(urls, v)
				case map[string]any:
					if u := firstStr(v, "url", "image", "b64_json"); u != "" {
						urls = append(urls, normalizeImage(v, u))
					}
				}
			}
		}
	}
	// output 数组（类 gpt-image / dalle3 代理）
	if len(urls) == 0 {
		if arr, ok := data["output"].([]any); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					if u := firstStr(m, "url", "image", "b64_json"); u != "" {
						urls = append(urls, normalizeImage(m, u))
					}
				}
			}
		}
	}
	// 顶层单图
	if len(urls) == 0 {
		if u := firstStr(data, "url", "image", "b64_json"); u != "" {
			urls = append(urls, normalizeImage(data, u))
		}
	}

	// b64 前缀归一化
	for i, u := range urls {
		if !strings.HasPrefix(u, "data:") && !strings.HasPrefix(u, "http") {
			// 大概率是裸 base64
			urls[i] = "data:image/png;base64," + u
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("no image found in response: %s", truncate(jstr(data), 300))
	}
	return urls, nil
}

// normalizeImage 把 b64_json 字段包装成 data URI。
func normalizeImage(m map[string]any, val string) string {
	if _, isB64 := m["b64_json"]; isB64 && !strings.HasPrefix(val, "data:") && !strings.HasPrefix(val, "http") {
		return "data:image/png;base64," + val
	}
	return val
}

// firstStr 返回 keys 中第一个非空字符串字段。
func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// fileToDataURI reads a local image and encodes it as a data URI.
func fileToDataURI(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	mime := "image/png"
	lo := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lo, ".jpg"), strings.HasSuffix(lo, ".jpeg"):
		mime = "image/jpeg"
	case strings.HasSuffix(lo, ".webp"):
		mime = "image/webp"
	case strings.HasSuffix(lo, ".gif"):
		mime = "image/gif"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func jstr(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
