package client

import (
	"strings"
	"testing"
)

func TestTrimContext(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "system", Content: "SYS"},
		{Role: "user", Content: strings.Repeat("旧", 100)}, // 最旧，应被丢弃
		{Role: "assistant", Content: strings.Repeat("答", 20)},
		{Role: "user", Content: strings.Repeat("新", 30)}, // 最新，必须保留
	}

	// 预算足够时不改动。
	if got := trimContext(msgs, 1000); len(got) != 4 {
		t.Fatalf("预算充足不应裁剪，got %d", len(got))
	}

	// 预算 55：从旧到新保留后缀，system 永远保留。
	got := trimContext(msgs, 55)
	if got[0].Role != "system" {
		t.Fatal("system 必须保留")
	}
	last := got[len(got)-1].Content.(string)
	if last != strings.Repeat("新", 30) {
		t.Fatal("最新用户要求必须完整保留")
	}
	for _, m := range got {
		if s, ok := m.Content.(string); ok && s == strings.Repeat("旧", 100) {
			t.Fatal("最旧超预算消息应被丢弃")
		}
	}

	// 图片不计入预算、不被裁剪。
	withImg := []ChatMessage{
		{Role: "user", Content: []ChatPart{
			{Type: "text", Text: strings.Repeat("文", 200)},
			{Type: "image_url", ImageURL: "data:image/png;base64,AAAA"},
		}},
	}
	gotImg := trimContext(withImg, 50)
	parts := gotImg[0].Content.([]ChatPart)
	if len(parts) != 2 || parts[1].Type != "image_url" {
		t.Fatal("图片部分必须保留")
	}
	if strings.Contains(parts[0].Text, "截断") == false {
		t.Fatal("超长单条文本应被截断并加标记")
	}
	if !strings.HasSuffix(parts[0].Text, strings.Repeat("文", 50)) {
		t.Fatal("截断应保留尾部用户要求")
	}
}
