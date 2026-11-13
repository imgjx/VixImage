package client

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"sort"
	"strconv"
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
	// Stream 开启流式（当前对 openai_chat 生效）；增量经 OnDelta 逐段回调，
	// 便于规划过程「每一步都能看到」。
	Stream  bool
	OnDelta func(string)
}

// Client performs HTTP calls to upstream image/chat models in OpenAI / Gemini / Stability / Anthropic shapes.
type Client struct {
	http *http.Client
}

func New() *Client {
	// 统一注入自定义 User-Agent：部分上游（如 Cloudflare 防护）会拦截
	// Go 默认 UA "Go-http-client/2.0"，统一身份避免被盾 403。
	tp := &uaTransport{base: http.DefaultTransport}
	return &Client{http: &http.Client{Timeout: 10 * time.Minute, Transport: tp}}
}

// upstreamUA 是所有上游请求统一的 User-Agent。
// 实测部分上游（如 Cloudflare 防护）会 tarpit/拦截非浏览器 UA（含 Go 默认与
// 自定义产品 UA），只有浏览器形态 UA 能稳定通过，故统一伪装为 Chrome。
const upstreamUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

type uaTransport struct{ base http.RoundTripper }

func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", upstreamUA)
	}
	return t.base.RoundTrip(req)
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
		body["image"] = ref // OpenAI 兼容 i2i 字段
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
	case model.FormatGemini:
		return c.geminiImage(req)
	case model.FormatStability:
		return c.stabilityImage(req)
	default: // openai
		endpoint = imagesEndpoint(req.Model.BaseURL, "/v1/images/generations")
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
// already ends with "/v1" so we never emit "/v1/v1/...".
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
// already includes the "/v1" suffix, avoiding "/v1/v1/...".
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
	// 上下文限制（context_limit）：在所有协议分支之前统一按字符预算裁剪输入，
	// 防止复杂/多轮需求把上下文撑爆。只裁文本，保留图片与最新用户要求。
	if lim := req.Model.ContextLimit; lim > 0 {
		req.Messages = trimContext(req.Messages, lim)
	}
	// Google Gemini / Anthropic 有自己的原生协议，单独适配。
	switch req.Model.Format {
	case model.FormatGemini:
		return c.geminiChat(req)
	case model.FormatAnthropic:
		return c.anthropicChat(req)
	}
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
	if req.Stream && req.OnDelta != nil {
		return c.chatOpenAIStream(endpoint, req, body)
	}
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
//   - 兼容:    data[].url / 顶层 url
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

// ---------------------------------------------------------------------------
// Google Gemini 适配（chat + image 均走 generateContent）
// ---------------------------------------------------------------------------

// geminiEndpoint 拼出 {base}/v1beta/models/{model}:generateContent，容忍 base 已带 /v1beta。
func geminiEndpoint(base, modelName, action string) string {
	b := strings.TrimRight(base, "/")
	if !strings.HasSuffix(b, "/v1beta") {
		b += "/v1beta"
	}
	return b + "/models/" + modelName + action
}

// geminiParts 把文本与可选参考图转成 Gemini parts；data URI 拆成 inline_data。
func geminiParts(text, refImage string) []map[string]any {
	parts := []map[string]any{{"text": text}}
	if mime, b64, ok := parseDataURI(refImage); ok {
		parts = append(parts, map[string]any{
			"inline_data": map[string]any{"mime_type": mime, "data": b64},
		})
	} else if strings.HasPrefix(refImage, "http") {
		parts = append(parts, map[string]any{
			"file_data": map[string]any{"mime_type": "image/png", "file_uri": refImage},
		})
	}
	return parts
}

func (c *Client) geminiChat(req ChatRequest) (string, error) {
	contents := make([]map[string]any, 0, len(req.Messages))
	sys := ""
	for _, m := range req.Messages {
		if m.Role == "system" {
			if s, ok := m.Content.(string); ok {
				sys = s
			}
			continue
		}
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		text := ""
		var img string
		if s, ok := m.Content.(string); ok {
			text = s
		} else if arr, ok := m.Content.([]ChatPart); ok {
			var sb strings.Builder
			for _, p := range arr {
				if p.Type == "text" {
					sb.WriteString(p.Text)
				} else if p.Type == "image_url" {
					img = p.ImageURL
				}
			}
			text = sb.String()
		}
		contents = append(contents, map[string]any{"role": role, "parts": geminiParts(text, img)})
	}
	body := map[string]any{"contents": contents}
	if sys != "" {
		body["systemInstruction"] = map[string]any{"parts": []map[string]any{{"text": sys}}}
	}
	gc := map[string]any{"temperature": orF(req.Temp, 0.7)}
	if mt := req.Model.MaxTokens; mt > 0 {
		gc["maxOutputTokens"] = mt
	}
	if tp := req.Model.TopP; tp > 0 {
		gc["topP"] = tp
	}
	body["generationConfig"] = gc
	endpoint := geminiEndpoint(req.Model.BaseURL, req.Model.ModelName, ":generateContent")
	data, err := c.postJSONH(endpoint, req.Model.APIKey,
		map[string]string{"x-goog-api-key": req.Model.APIKey}, body)
	if err != nil {
		return "", err
	}
	if t := geminiText(data); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("unexpected gemini response: %s", truncate(jstr(data), 300))
}

func (c *Client) geminiImage(req ImageRequest) (*ImageResult, error) {
	parts := geminiParts(req.Prompt, req.InputURL)
	gc := map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}
	if ar := aspectRatio(req.Size); ar != "" {
		gc["imageConfig"] = map[string]any{"aspectRatio": ar}
	}
	body := map[string]any{
		"contents":         []map[string]any{{"parts": parts}},
		"generationConfig": gc,
	}
	endpoint := geminiEndpoint(req.Model.BaseURL, req.Model.ModelName, ":generateContent")
	data, err := c.postJSONH(endpoint, req.Model.APIKey,
		map[string]string{"x-goog-api-key": req.Model.APIKey}, body)
	if err != nil {
		return nil, err
	}
	var urls []string
	if cands, ok := data["candidates"].([]any); ok {
		for _, cd := range cands {
			content, _ := cd.(map[string]any)["content"].(map[string]any)
			ps, _ := content["parts"].([]any)
			for _, p := range ps {
				m, ok := p.(map[string]any)
				if !ok {
					continue
				}
				for _, key := range []string{"inlineData", "inline_data"} {
					if in, ok := m[key].(map[string]any); ok {
						if d, _ := in["data"].(string); d != "" {
							mime, _ := in["mimeType"].(string)
							if mime == "" {
								mime, _ = in["mime_type"].(string)
							}
							if mime == "" {
								mime = "image/png"
							}
							urls = append(urls, "data:"+mime+";base64,"+d)
						}
					}
				}
			}
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("gemini returned no image: %s", truncate(jstr(data), 300))
	}
	return &ImageResult{URLs: urls}, nil
}

// geminiText 合并 candidates[0].content.parts 中的所有 text。
func geminiText(data map[string]any) string {
	cands, _ := data["candidates"].([]any)
	if len(cands) == 0 {
		return ""
	}
	content, _ := cands[0].(map[string]any)["content"].(map[string]any)
	ps, _ := content["parts"].([]any)
	var sb strings.Builder
	for _, p := range ps {
		if m, ok := p.(map[string]any); ok {
			if t, ok := m["text"].(string); ok {
				sb.WriteString(t)
			}
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Anthropic Claude 适配（chat: /v1/messages）
// ---------------------------------------------------------------------------

func anthropicEndpoint(base string) string {
	b := strings.TrimRight(base, "/")
	if strings.HasSuffix(b, "/messages") {
		return b
	}
	if strings.HasSuffix(b, "/v1") {
		return b + "/messages"
	}
	return b + "/v1/messages"
}

func (c *Client) anthropicChat(req ChatRequest) (string, error) {
	sys := ""
	msgs := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			if s, ok := m.Content.(string); ok {
				sys = s
			}
			continue
		}
		switch v := m.Content.(type) {
		case string:
			msgs = append(msgs, map[string]any{"role": m.Role, "content": v})
		case []ChatPart:
			var blocks []map[string]any
			for _, p := range v {
				switch p.Type {
				case "image_url":
					if mime, b64, ok := parseDataURI(p.ImageURL); ok {
						blocks = append(blocks, map[string]any{
							"type":   "image",
							"source": map[string]any{"type": "base64", "media_type": mime, "data": b64},
						})
					}
				default:
					blocks = append(blocks, map[string]any{"type": "text", "text": p.Text})
				}
			}
			if blocks != nil {
				msgs = append(msgs, map[string]any{"role": m.Role, "content": blocks})
			}
		}
	}
	mt := req.Model.MaxTokens
	if mt <= 0 {
		mt = 4096
	}
	body := map[string]any{
		"model": req.Model.ModelName, "max_tokens": mt, "messages": msgs,
		"temperature": orF(req.Temp, 0.7),
	}
	if sys != "" {
		body["system"] = sys
	}
	endpoint := anthropicEndpoint(req.Model.BaseURL)
	data, err := c.postJSONH(endpoint, req.Model.APIKey, map[string]string{
		"x-api-key":         req.Model.APIKey,
		"anthropic-version": "2023-06-01",
	}, body)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if arr, ok := data["content"].([]any); ok {
		for _, it := range arr {
			if m, ok := it.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
	}
	if t := sb.String(); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("unexpected anthropic response: %s", truncate(jstr(data), 300))
}

// ---------------------------------------------------------------------------
// Stability AI 适配（image: /v2beta/stable-image/generate/core，JSON 响应）
// ---------------------------------------------------------------------------

func (c *Client) stabilityImage(req ImageRequest) (*ImageResult, error) {
	b := strings.TrimRight(req.Model.BaseURL, "/")
	if !strings.HasSuffix(b, "/v2beta") {
		b += "/v2beta"
	}
	endpoint := b + "/stable-image/generate/core"
	form := map[string]string{
		"prompt":        req.Prompt,
		"output_format": "png",
	}
	if ar := aspectRatio(req.Size); ar != "" {
		form["aspect_ratio"] = ar
	}
	data, err := c.postFormJSON(endpoint, req.Model.APIKey, form)
	if err != nil {
		return nil, err
	}
	var urls []string
	if v := firstStr(data, "image", "b64_json"); v != "" {
		urls = append(urls, "data:image/png;base64,"+v)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("stability returned no image: %s", truncate(jstr(data), 300))
	}
	return &ImageResult{URLs: urls}, nil
}

// ---------------------------------------------------------------------------
// 流式（openai_chat SSE）
// ---------------------------------------------------------------------------

// chatOpenAIStream 发起 stream:true 的 OpenAI 兼容请求，逐段回调 delta，
// 返回拼接后的完整文本。
func (c *Client) chatOpenAIStream(endpoint string, req ChatRequest, body map[string]any) (string, error) {
	body["stream"] = true
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Model.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upstream http %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}
	var sb strings.Builder
	rd := bufio.NewReaderSize(resp.Body, 64*1024)
	for {
		line, rerr := rd.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				break
			} else if payload != "" {
				var ev map[string]any
				if e := json.Unmarshal([]byte(payload), &ev); e == nil {
					if ch, ok := ev["choices"].([]any); ok && len(ch) > 0 {
						if m, ok := ch[0].(map[string]any)["delta"].(map[string]any); ok {
							if d, ok := m["content"].(string); ok && d != "" {
								sb.WriteString(d)
								if req.OnDelta != nil {
									req.OnDelta(d)
								}
							}
						}
					}
				}
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return "", fmt.Errorf("read stream: %w", rerr)
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("empty stream from %s", endpoint)
	}
	return sb.String(), nil
}

// ---------------------------------------------------------------------------
// 通用小工具
// ---------------------------------------------------------------------------

// postJSONH 在 postJSON 基础上支持额外请求头（Gemini / Anthropic 用）。
func (c *Client) postJSONH(url, apiKey string, headers map[string]string, body map[string]any) (map[string]any, error) {
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
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
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

// postFormJSON 发送 multipart 表单并解析 JSON 响应（Stability 用）。
func (c *Client) postFormJSON(url, apiKey string, form map[string]string) (map[string]any, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_ = w.WriteField(k, form[k])
	}
	_ = w.Close()
	httpReq, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Accept", "application/json")
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
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		return map[string]any{"image": base64.StdEncoding.EncodeToString(raw)}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode: %w (%s)", err, truncate(string(raw), 200))
	}
	return out, nil
}

// parseDataURI 解析 data URI，返回 (mime, base64)。
func parseDataURI(s string) (string, string, bool) {
	if !strings.HasPrefix(s, "data:") {
		return "", "", false
	}
	rest := s[len("data:"):]
	i := strings.Index(rest, ",")
	if i < 0 {
		return "", "", false
	}
	mime := strings.TrimSuffix(rest[:i], ";base64")
	if mime == "" {
		mime = "image/png"
	}
	return mime, rest[i+1:], true
}

// aspectRatio 把 "WxH" 尺寸映射到最近的常见宽高比（Gemini / Stability 用）。
func aspectRatio(size string) string {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(size)), "x", 2)
	if len(parts) != 2 {
		return ""
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return ""
	}
	r := float64(w) / float64(h)
	best, bestDiff := "", 1e9
	for _, c := range []struct {
		name string
		v    float64
	}{{"1:1", 1}, {"4:5", 0.8}, {"5:4", 1.25}, {"2:3", 2.0 / 3}, {"3:2", 1.5},
		{"3:4", 0.75}, {"4:3", 4.0 / 3}, {"9:16", 9.0 / 16}, {"16:9", 16.0 / 9}, {"21:9", 21.0 / 9}} {
		d := r - c.v
		if d < 0 {
			d = -d
		}
		if d < bestDiff {
			best, bestDiff = c.name, d
		}
	}
	return best
}

// orF 返回 v；v<=0 时返回缺省值 def。
func orF(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	return v
}

// trimContext 把输入消息裁剪到指定的字符预算内，防止复杂/多轮需求把上下文撑爆。
//
// 规则（与 max_tokens 相对，一个管输入、一个管输出）：
//   - 只裁文本内容；图片（image_url / base64）一律保留，不计入预算。
//   - system 消息永远保留（承载工具说明，删了模型就不会干活）。
//   - 从最旧的非 system 消息开始整条丢弃，直到总量落入预算。
//   - 若仅剩最后一条且文本仍超预算，则截掉其【头部】、保留【尾部】
//     （用户的最新要求、步骤约束通常写在尾部），并加省略标记。
//
// 用 rune 计数，中文按字计，避免按字节裁一半导致乱码。
func trimContext(msgs []ChatMessage, limit int) []ChatMessage {
	if limit <= 0 || len(msgs) == 0 {
		return msgs
	}
	textOf := func(c any) (string, bool) {
		switch v := c.(type) {
		case string:
			return v, true
		case []ChatPart:
			var sb strings.Builder
			for _, p := range v {
				if p.Type == "text" {
					sb.WriteString(p.Text)
				}
			}
			return sb.String(), true
		}
		return "", false
	}
	runes := func(s string) []rune { return []rune(s) }
	total := 0
	for _, m := range msgs {
		if s, ok := textOf(m.Content); ok && m.Role != "system" {
			total += len(runes(s))
		}
	}
	if total <= limit {
		return msgs
	}

	// 只保留能塞进预算的【最新一段】非 system 消息后缀；system 全留。
	start := -1
	acc := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == "system" {
			continue
		}
		s, _ := textOf(m.Content)
		n := len(runes(s))
		if acc+n > limit {
			break
		}
		acc += n
		start = i
	}
	if start < 0 { // 连最后一条都超预算 → 只保留它，稍后截文本
		start = len(msgs) - 1
	}
	out := make([]ChatMessage, 0, len(msgs))
	for i, m := range msgs {
		if m.Role == "system" {
			out = append(out, m)
		} else if i >= start {
			out = append(out, m)
		}
	}

	// 兜底：若保留下来的文本仍超预算（多为最后一条超长），截其头部保尾部。
	cur := 0
	for _, m := range out {
		if s, ok := textOf(m.Content); ok && m.Role != "system" {
			cur += len(runes(s))
		}
	}
	if cur > limit {
		for i := len(out) - 1; i >= 0; i-- {
			m := &out[i]
			if m.Role == "system" {
				continue
			}
			s, ok := textOf(m.Content)
			if !ok {
				continue
			}
			r := runes(s)
			if len(r) <= limit {
				continue
			}
			clip := "…（上文已按上下文限制截断）…" + string(r[len(r)-limit:])
			switch m.Content.(type) {
			case string:
				m.Content = clip
			case []ChatPart:
				parts := m.Content.([]ChatPart)
				firstText := -1
				for j := range parts {
					if parts[j].Type == "text" && firstText < 0 {
						firstText = j
					}
				}
				if firstText >= 0 {
					parts[firstText].Text = clip
					m.Content = parts
				}
			}
			break
		}
	}
	return out
}
