package skills

import (
	"fmt"
	"image"
	"image/color"
	"bytes"
	"os"

	skipqrcode "github.com/skip2/go-qrcode"

	gozxing "github.com/makiuchi-d/gozxing"
	gozxingqr "github.com/makiuchi-d/gozxing/qrcode"
)

// GenerateQR 生成二维码 PNG。text 为要编码的内容；
// size 为输出边长（像素，0 取默认 512）；fg/bg 为 "#rrggbb" 前景/背景色（空取黑/白）。
// 容错等级固定最高，方便海报上小幅印刷后仍可扫出。
func GenerateQR(text string, size int, fg, bg string) ([]byte, error) {
	if size <= 0 {
		size = 512
	}
	if size < 64 {
		size = 64
	}
	if size > 2048 {
		size = 2048
	}
	q, err := skipqrcode.New(text, skipqrcode.Highest)
	if err != nil {
		return nil, err
	}
	q.DisableBorder = false
	q.ForegroundColor = hexColor(fg, color.RGBA{0, 0, 0, 255})
	q.BackgroundColor = hexColor(bg, color.RGBA{255, 255, 255, 255})
	return q.PNG(size)
}

func hexColor(v string, def color.RGBA) color.RGBA {
	if v == "" {
		return def
	}
	c := ParseHex(v)
	c.A = 255
	return c
}

// DecodeQR 从本地图片文件解析二维码，返回解码文本。找不到二维码时报错（无兜底）。
func DecodeQR(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if res, err := decodeQRImage(img); err == nil {
		return res, nil
	}
	// 放大后重试一次：小尺寸二维码常因采样不足识别失败。
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	big := scaleImage(rgba, rgba.Bounds().Dx()*3, rgba.Bounds().Dy()*3)
	if res, err := decodeQRImage(big); err == nil {
		return res, nil
	}
	return "", fmt.Errorf("未在图片中识别到二维码")
}

func decodeQRImage(img image.Image) (string, error) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}
	res, err := gozxingqr.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		return "", err
	}
	return res.GetText(), nil
}
