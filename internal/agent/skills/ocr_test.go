package skills

import (
	"encoding/json"
	"testing"
)

func TestOcrResultParse(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantTxt string
		wantErr string
	}{
		{"正常", `{"lines":["你好","世界"],"text":"你好\n世界"}`, "你好\n世界", ""},
		{"未安装", `{"error":"PaddleOCR 未安装，请先执行：pip install paddleocr paddlepaddle"}`, "", "PaddleOCR 未安装"},
		{"无文字", `{"lines":[],"text":""}`, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var res ocrResult
			if err := json.Unmarshal([]byte(c.raw), &res); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if c.wantErr != "" && res.Error != c.wantErr && !contains(res.Error, c.wantErr) {
				t.Fatalf("error=%q 期望包含 %q", res.Error, c.wantErr)
			}
			if res.Text != c.wantTxt {
				t.Fatalf("text=%q 期望 %q", res.Text, c.wantTxt)
			}
		})
	}
}

// TestOcrScriptEmbedded 确保 Python 脚本已内嵌（非空且含关键 API）。
func TestOcrScriptEmbedded(t *testing.T) {
	if len(ocrScript) == 0 {
		t.Fatal("OCR 脚本未内嵌")
	}
	for _, kw := range []string{"PaddleOCR", "predict", "rec_texts", "argparse"} {
		if !contains(ocrScript, kw) {
			t.Fatalf("OCR 脚本缺少关键内容 %q", kw)
		}
	}
}

func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
