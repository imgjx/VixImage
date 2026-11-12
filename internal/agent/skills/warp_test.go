package skills

import (
	"image"
	"image/color"
	"testing"
)

// TestWarpTrapezoidShapeAndColor 回归：透视梯形必须顶窄底宽，且内部像素保留源色
// （不能因反预乘 clamp 错误而变成全白）。
func TestWarpTrapezoidShapeAndColor(t *testing.T) {
	// 构造一张纯红源图（便于检测"变白"回归）。
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			src.SetRGBA(x, y, color.RGBA{220, 30, 30, 255})
		}
	}
	const w, h = 200, 200
	out := WarpTrapezoid(src, w, h, 0.4, 0)
	// 统计顶行与底行不透明像素：顶窄底宽。
	countRow := func(y int) int {
		c := 0
		for x := 0; x < w; x++ {
			if out.RGBAAt(x, y).A > 10 {
				c++
			}
		}
		return c
	}
	top, bot := countRow(0), countRow(h-1)
	if !(top < bot) {
		t.Fatalf("透视梯形方向错误：顶行不透明=%d 应 < 底行=%d（顶窄底宽）", top, bot)
	}
	// 内部像素应保留红色，而非变白。
	mid := out.RGBAAt(w/2, h/2)
	if mid.A < 200 {
		t.Fatalf("梯形内部像素应不透明，实际 A=%d", mid.A)
	}
	if mid.R < 150 || mid.G > 120 || mid.B > 120 {
		t.Fatalf("梯形内部像素应近似红色，实际 RGB=(%d,%d,%d)（白色回归）", mid.R, mid.G, mid.B)
	}
}
