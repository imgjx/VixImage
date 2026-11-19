package skills

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed ocr_image.py
var ocrScript string

// ocrResult 是 Python 脚本写回的结果文件结构。
type ocrResult struct {
	Lines []string `json:"lines"`
	Text  string   `json:"text"`
	Error string   `json:"error"`
}

// RecognizeText 用 PaddleOCR 识别本地图片中的文字，返回按行拼接的文本。
// pythonBin 为 Python 解释器路径（复用 config.PythonBin），lang 为识别语言（默认 ch）。
// 未安装 PaddleOCR / 识别失败 / 图片无文字都返回明确错误——绝不返回臆造文字。
func RecognizeText(pythonBin, lang, imagePath, workDir string) (string, error) {
	if _, err := os.Stat(imagePath); err != nil {
		return "", fmt.Errorf("OCR 图片不存在: %w", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", err
	}
	if lang == "" {
		lang = "ch"
	}

	scriptPath := filepath.Join(workDir, "ocr_image.py")
	if err := os.WriteFile(scriptPath, []byte(ocrScript), 0o644); err != nil {
		return "", fmt.Errorf("写入 OCR 脚本失败: %w", err)
	}
	resultPath := filepath.Join(workDir, fmt.Sprintf("ocr_result_%d.json", os.Getpid()))
	_ = os.Remove(resultPath) // 避免上一次残留被误读

	cmd := exec.Command(pythonBin, scriptPath, "--image", imagePath, "--out", resultPath, "--lang", lang)
	output, err := cmd.CombinedOutput()
	if _, statErr := os.Stat(resultPath); statErr != nil {
		// 连结果文件都没写出来：Python 本身起不来 / 参数错误。
		return "", fmt.Errorf("OCR 执行失败（%v）: %s", err, truncate(string(output), 300))
	}
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		return "", fmt.Errorf("读取 OCR 结果失败: %w", err)
	}
	var res ocrResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("解析 OCR 结果失败: %w (%s)", err, truncate(string(raw), 200))
	}
	if res.Error != "" {
		return "", fmt.Errorf("OCR 失败: %s", res.Error)
	}
	if err != nil && res.Text == "" {
		// 脚本非零退出（如"识别成功但无文字"），无结果文本。
		return "", fmt.Errorf("OCR 未识别到文字（退出码 %v）", err)
	}
	if res.Text == "" {
		return "", fmt.Errorf("OCR 未识别到文字")
	}
	return res.Text, nil
}
