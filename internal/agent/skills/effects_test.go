package skills

import (
	"image/png"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	FillSolidRect(img, c)
	return img
}

func avgAlpha(img *image.RGBA) float64 {
	b := img.Bounds()
	var sum uint64
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			sum += uint64(img.RGBAAt(x, y).A)
		}
	}
	return float64(sum) / float64(uint64(b.Dx())*uint64(b.Dy()))
}

// TestRemoveBackground：白底红圆抠图后，边缘应透明、中心保留。
func TestRemoveBackground(t *testing.T) {
	src := solid(120, 120, color.RGBA{255, 255, 255, 255})
	for y := 40; y < 80; y++ {
		for x := 40; x < 80; x++ {
			src.SetRGBA(x, y, color.RGBA{200, 30, 30, 255})
		}
	}
	out := RemoveBackground(src, 60)
	if a := out.RGBAAt(2, 2).A; a != 0 {
		t.Fatalf("角落应被抠掉，alpha=%d", a)
	}
	if a := out.RGBAAt(60, 60).A; a != 255 {
		t.Fatalf("中心主体应保留，alpha=%d", a)
	}
}

// TestBlur：虚化后对比度应下降（边缘不再锋利）。
func TestBlur(t *testing.T) {
	src := solid(64, 64, color.RGBA{0, 0, 0, 255})
	for y := 0; y < 20; y++ {
		for x := 0; x < 64; x++ {
			src.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	out := Blur(src, 6)
	// 原图边界处是 0/255 硬跳变，模糊后应为中间值
	v := out.RGBAAt(32, 20).R
	if v < 40 || v > 215 {
		t.Fatalf("虚化后边界像素应为中间值，实际 %d", v)
	}
}

// TestRotate：旋转 30 度后画布尺寸不变（expand=false），中心内容仍近似原色。
func TestRotate(t *testing.T) {
	src := solid(100, 100, color.RGBA{10, 200, 30, 255})
	out := Rotate(src, 30, false, nil)
	if out.Bounds().Dx() != 100 || out.Bounds().Dy() != 100 {
		t.Fatalf("expand=false 时尺寸应不变，实际 %dx%d", out.Bounds().Dx(), out.Bounds().Dy())
	}
	c := out.RGBAAt(50, 50)
	if c.G < 150 {
		t.Fatalf("旋转后中心应仍为绿色，实际 %+v", c)
	}
}

// TestRotateExpand：expand=true 时画布应扩大。
func TestRotateExpand(t *testing.T) {
	src := solid(100, 60, color.RGBA{255, 0, 0, 255})
	out := Rotate(src, 45, true, nil)
	if out.Bounds().Dx() < 100 || out.Bounds().Dy() < 60 {
		t.Fatalf("expand=true 画布应扩大，实际 %dx%d", out.Bounds().Dx(), out.Bounds().Dy())
	}
}

// TestShearLayer：斜切后画布加宽、顶部内容右移。
func TestShearLayer(t *testing.T) {
	src := solid(50, 50, color.RGBA{255, 255, 255, 255})
	out := shearLayer(src, 0.2)
	if out.Bounds().Dx() <= 50 {
		t.Fatalf("斜切后画布应加宽，实际 %d", out.Bounds().Dx())
	}
	// 顶部第 10 行应有内容且比底部第 40 行更靠右
	topX := -1
	botX := -1
	for x := out.Bounds().Dx() - 1; x >= 0; x-- {
		if topX < 0 && out.RGBAAt(x, 10).A > 0 {
			topX = x
		}
		if botX < 0 && out.RGBAAt(x, 40).A > 0 {
			botX = x
		}
	}
	if topX < 0 || botX < 0 {
		t.Fatalf("斜切后内容丢失 topX=%d botX=%d", topX, botX)
	}
	if topX <= botX {
		t.Fatalf("斜体顶部应比底部更靠右 top=%d bottom=%d", topX, botX)
	}
}

// TestItalicSpec：斜体文字整图渲染应成功输出文件。
func TestItalicSpec(t *testing.T) {
	dir := t.TempDir()
	spec := &Spec{
		Width: 600, Height: 300,
		BG: &BGSpec{Type: "solid", Color: "#101828"},
		Items: []SpecItem{
			{Type: "text", Text: "斜体标题测试", X: 40, Y: 60, Size: 56, Color: "#ffffff", Weight: "bold", Italic: true, Glow: "#3d8bff", GlowSize: 12},
		},
		Output: filepath.Join(dir, "italic.png"),
	}
	out, err := RenderSpecToFile(spec)
	if err != nil {
		t.Fatalf("斜体渲染失败: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("输出文件不存在: %v", err)
	}
}

// TestProcessSpecEffects：整条渲染管线应接受 blur/rotate/remove_bg 的组合。
func TestProcessSpecEffects(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.png")
	src := solid(200, 200, color.RGBA{255, 255, 255, 255})
	for y := 60; y < 140; y++ {
		for x := 60; x < 140; x++ {
			src.SetRGBA(x, y, color.RGBA{30, 90, 220, 255})
		}
	}
	f, _ := os.Create(in)
	// 用 png 编码保存
	encodePNG(t, f, src)
	f.Close()

	spec := &Spec{
		Width: 400, Height: 300,
		BG: &BGSpec{Type: "gradient", Colors: []string{"#050912", "#0d1a30"}, Angle: 90},
		Items: []SpecItem{
			{Type: "image", Src: in, X: 100, Y: 30, W: 200, H: 200, RemoveBG: true, RemoveTol: 70, Blur: 2, Rotate: 12},
		},
		Output: filepath.Join(dir, "out.png"),
	}
	out, err := RenderSpecToFile(spec)
	if err != nil {
		t.Fatalf("组合效果渲染失败: %v", err)
	}
	img, err := LoadImage(out)
	if err != nil {
		t.Fatalf("读取输出失败: %v", err)
	}
	// 画布角落应是背景渐变色而非透明
	if img.RGBAAt(5, 5).A != 255 {
		t.Fatalf("背景不应透明")
	}
}

func encodePNG(t *testing.T, f *os.File, img *image.RGBA) {
	if err := pngEncode(f, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
}

func pngEncode(f *os.File, img *image.RGBA) error {
	return png.Encode(f, img)
}
