package skills

import (
	"image"
	"image/color"
	"math"
)

// ---------------------------------------------------------------------------
// 图片处理技能（Go 原生实现）
// 提供：虚化、旋转/斜放、抠图（去背景）、斜体字等效果。
// ---------------------------------------------------------------------------

// Blur 虚化：对整张图做高斯近似（盒滤波）模糊，radius 越大越糊。
func Blur(src *image.RGBA, radius int) *image.RGBA {
	return blurRGBA(src, radius)
}

// BlurRegion 只虚化指定矩形区域（常用于打码背景、突出主体）。
func BlurRegion(src *image.RGBA, x, y, w, h, radius int) *image.RGBA {
	b := src.Bounds()
	clip := image.Rect(x, y, x+w, y+h).Intersect(b)
	if clip.Empty() || radius < 1 {
		return src
	}
	// 提取区域 → 模糊 → 贴回
	sub := image.NewRGBA(image.Rect(0, 0, clip.Dx(), clip.Dy()))
	drawCopy(sub, src, clip.Min)
	blurred := blurRGBA(sub, radius)
	out := cloneRGBA(src)
	for yy := 0; yy < clip.Dy(); yy++ {
		for xx := 0; xx < clip.Dx(); xx++ {
			out.SetRGBA(clip.Min.X+xx, clip.Min.Y+yy, blurred.RGBAAt(xx, yy))
		}
	}
	return out
}

// Rotate 旋转/斜放图片。angle 单位为度，正值顺时针。
// expand=true 时扩张画布以容纳全部像素（不裁切）；false 时保持原尺寸，
// 超出部分透明，便于直接作为版式里的斜贴图层。
// bg 非空时用该颜色填充空隙（否则透明）。
func Rotate(src *image.RGBA, angle float64, expand bool, bg *color.RGBA) *image.RGBA {
	if angle == 0 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	rad := angle * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)

	ow, oh := w, h
	if expand {
		ow = int(math.Ceil(math.Abs(float64(w)*cos) + math.Abs(float64(h)*sin)))
		oh = int(math.Ceil(math.Abs(float64(w)*sin) + math.Abs(float64(h)*cos)))
	}
	out := image.NewRGBA(image.Rect(0, 0, maxInt(ow, 1), maxInt(oh, 1)))
	if bg != nil {
		FillSolidRect(out, *bg)
	}
	cx, cy := float64(w)/2, float64(h)/2
	ocx, ocy := float64(ow)/2, float64(oh)/2
	// 逆向映射：对输出每个像素，找源图中对应位置并双线性采样。
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			dx := float64(x) - ocx
			dy := float64(y) - ocy
			// 逆旋转
			sx := cos*dx + sin*dy + cx
			sy := -sin*dx + cos*dy + cy
			if sx < 0 || sy < 0 || sx >= float64(w)-1 || sy >= float64(h)-1 {
				continue
			}
			out.SetRGBA(x, y, bilinear(src, sx, sy))
		}
	}
	return out
}

// bilinear 双线性插值采样。
func bilinear(src *image.RGBA, fx, fy float64) color.RGBA {
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	dx, dy := fx-float64(x0), fy-float64(y0)
	x1, y1 := minInt(x0+1, src.Bounds().Dx()-1), minInt(y0+1, src.Bounds().Dy()-1)
	x0 = maxInt(x0, 0)
	y0 = maxInt(y0, 0)
	c00 := src.RGBAAt(x0, y0)
	c10 := src.RGBAAt(x1, y0)
	c01 := src.RGBAAt(x0, y1)
	c11 := src.RGBAAt(x1, y1)
	mixX := func(a, b uint8) uint8 { return uint8(float64(a)*(1-dx) + float64(b)*dx) }
	mixY := func(a, b uint8) uint8 { return uint8(float64(a)*(1-dy) + float64(b)*dy) }
	top := color.RGBA{mixX(c00.R, c10.R), mixX(c00.G, c10.G), mixX(c00.B, c10.B), mixX(c00.A, c10.A)}
	bot := color.RGBA{mixX(c01.R, c11.R), mixX(c01.G, c11.G), mixX(c01.B, c11.B), mixX(c01.A, c11.A)}
	return color.RGBA{
		mixY(top.R, bot.R), mixY(top.G, bot.G), mixY(top.B, bot.B), mixY(top.A, bot.A),
	}
}

// RemoveBackground 抠图：从四边取样背景色，泛洪填充移除与背景相近的连通区域
// （alpha 置 0）。tolerance 为颜色距离阈值（0~441，常用 40~90）。
// 适用于纯色/渐变简单背景的产品图、角色立绘。
func RemoveBackground(src *image.RGBA, tolerance float64) *image.RGBA {
	out := cloneRGBA(src)
	b := out.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return out
	}
	// 背景种子 = 四条边上的像素。
	visited := make([]bool, w*h)
	queue := make([]int, 0, w*h)
	push := func(x, y int) {
		idx := y*w + x
		if !visited[idx] {
			visited[idx] = true
			queue = append(queue, idx)
		}
	}
	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	// 参考背景色 = 边缘像素中位数近似（取四角平均）。
	corners := []color.RGBA{
		out.RGBAAt(0, 0), out.RGBAAt(w-1, 0),
		out.RGBAAt(0, h-1), out.RGBAAt(w-1, h-1),
	}
	var br, bg2, bb uint32
	for _, c := range corners {
		br += uint32(c.R)
		bg2 += uint32(c.G)
		bb += uint32(c.B)
	}
	ref := color.RGBA{uint8(br / 4), uint8(bg2 / 4), uint8(bb / 4), 255}

	dist := func(c color.RGBA) float64 {
		dr := float64(int(c.R) - int(ref.R))
		dg := float64(int(c.G) - int(ref.G))
		db := float64(int(c.B) - int(ref.B))
		return math.Sqrt(dr*dr + dg*dg + db*db)
	}
	// BFS 泛洪：只移除与已移除区域相邻且颜色接近的部分（保留主体内部相近色）。
	for len(queue) > 0 {
		idx := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		x, y := idx%w, idx/w
		c := out.RGBAAt(x, y)
		if c.A == 0 {
			continue // 已透明
		}
		if dist(c) > tolerance {
			continue // 与背景差异过大 → 属于主体
		}
		// 软边：按距离比例降低 alpha，而不是一刀切为 0。
		if dist(c) > tolerance*0.55 {
			ratio := 1 - (dist(c)-tolerance*0.55)/(tolerance*0.45)
			c.A = uint8(float64(c.A) * clamp01(ratio))
			out.SetRGBA(x, y, c)
		} else {
			c.A = 0
			out.SetRGBA(x, y, c)
		}
		if x > 0 {
			push(x-1, y)
		}
		if x < w-1 {
			push(x+1, y)
		}
		if y > 0 {
			push(x, y-1)
		}
		if y < h-1 {
			push(x, y+1)
		}
	}
	return out
}

// Sharpen 简单锐化（拉普拉斯），radius 小、强度弱，用于抠图后提升边缘。
func Sharpen(src *image.RGBA, amount float64) *image.RGBA {
	if amount <= 0 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := cloneRGBA(src)
	blur := blurRGBA(src, 2)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			o := src.RGBAAt(x, y)
			u := blur.RGBAAt(x, y)
			nc := color.RGBA{}
			nc.R = clamp255(float64(o.R) + (float64(o.R)-float64(u.R))*amount)
			nc.G = clamp255(float64(o.G) + (float64(o.G)-float64(u.G))*amount)
			nc.B = clamp255(float64(o.B) + (float64(o.B)-float64(u.B))*amount)
			nc.A = o.A
			out.SetRGBA(x, y, nc)
		}
	}
	return out
}

// FillSolidRect 用纯色填充整张图。
func FillSolidRect(img *image.RGBA, col color.RGBA) {
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			img.SetRGBA(b.Min.X+x, b.Min.Y+y, col)
		}
	}
}

// shearLayer 斜切变换（斜体字原理）：k 为斜切系数，顶部向 k 方向偏移 k*height
// （k>0 向右斜、k<0 向左斜）。画布按偏移范围精确扩张，不裁切内容。
func shearLayer(src *image.RGBA, k float64) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 || k == 0 {
		return src
	}
	// 偏移范围：y=0 时 offset=k*h（极值），y=h-1 时接近 0。
	rightPad := 0
	leftPad := 0
	if k > 0 {
		rightPad = int(math.Ceil(k * float64(h)))
	} else {
		leftPad = int(math.Ceil(-k * float64(h)))
	}
	ow := w + leftPad + rightPad
	out := image.NewRGBA(image.Rect(0, 0, ow, h))
	for y := 0; y < h; y++ {
		offset := k * float64(h-y)
		for x := 0; x < w; x++ {
			c := src.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			nx := int(math.Round(float64(x) + offset)) + leftPad
			if nx < 0 || nx >= ow {
				continue
			}
			out.SetRGBA(nx, y, c)
		}
	}
	return out
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	out := image.NewRGBA(src.Bounds())
	drawCopy(out, src, src.Bounds().Min)
	return out
}

func drawCopy(dst *image.RGBA, src *image.RGBA, srcMin image.Point) {
	b := dst.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.SetRGBA(x, y, src.RGBAAt(srcMin.X+x, srcMin.Y+y))
		}
	}
}

func clamp255(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
