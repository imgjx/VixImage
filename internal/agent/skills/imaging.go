package skills

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
)

// LoadImage decodes an image file into an RGBA image.
func LoadImage(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)
	return out, nil
}

// FitMode controls how a source image fills its target box.
type FitMode int

const (
	// FitCover crops the source to fill the box completely (no letterboxing).
	FitCover FitMode = iota
	// FitContain fits the whole source inside the box, leaving padding.
	FitContain
)

// Resize scales an image to exactly (w, h) using the given fit mode.
func Resize(src *image.RGBA, w, h int, mode FitMode) *image.RGBA {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw == 0 || sh == 0 {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	var scale float64
	if mode == FitCover {
		scale = math.Max(float64(w)/float64(sw), float64(h)/float64(sh))
	} else {
		scale = math.Min(float64(w)/float64(sw), float64(h)/float64(sh))
	}
	nw := int(math.Round(float64(sw) * scale))
	nh := int(math.Round(float64(sh) * scale))
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	// Nearest-neighbour-free quality: box-average downscale, bilinear upscale.
	scaled := scaleImage(src, nw, nh)
	if mode == FitContain {
		out := image.NewRGBA(image.Rect(0, 0, w, h))
		offX := (w - nw) / 2
		offY := (h - nh) / 2
		draw.Draw(out, image.Rect(offX, offY, offX+nw, offY+nh), scaled, scaled.Bounds().Min, draw.Over)
		return out
	}
	// cover → centre-crop
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	offX := (nw - w) / 2
	offY := (nh - h) / 2
	draw.Draw(out, out.Bounds(), scaled, image.Pt(offX, offY), draw.Over)
	return out
}

// scaleImage resizes with a box filter (good for downscale) and bilinear
// interpolation (good for upscale).
func scaleImage(src *image.RGBA, nw, nh int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	out := image.NewRGBA(image.Rect(0, 0, nw, nh))
	if sw == 0 || sh == 0 {
		return out
	}
	xRatio := float64(sw) / float64(nw)
	yRatio := float64(sh) / float64(nh)
	for y := 0; y < nh; y++ {
		sy0 := int(float64(y) * yRatio)
		sy1 := int(float64(y+1) * yRatio)
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		if sy1 > sh {
			sy1 = sh
		}
		for x := 0; x < nw; x++ {
			sx0 := int(float64(x) * xRatio)
			sx1 := int(float64(x+1) * xRatio)
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sx1 > sw {
				sx1 = sw
			}
			var r, g, b, a, n uint32
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					c := src.RGBAAt(sb.Min.X+sx, sb.Min.Y+sy)
					r += uint32(c.R)
					g += uint32(c.G)
					b += uint32(c.B)
					a += uint32(c.A)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			out.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
		}
	}
	return out
}

// RoundedMask builds an anti-aliased rounded-rectangle alpha mask.
func RoundedMask(w, h, radius int) *image.Alpha {
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	if radius <= 0 {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				m.SetAlpha(x, y, color.Alpha{A: 255})
			}
		}
		return m
	}
	if radius*2 > w {
		radius = w / 2
	}
	if radius*2 > h {
		radius = h / 2
	}
	// supersample 4x for smooth corners
	const ss = 4
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cov := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					py := float64(y) + (float64(sy)+0.5)/ss
					if inRoundedRect(px, py, float64(w), float64(h), float64(radius)) {
						cov++
					}
				}
			}
			m.SetAlpha(x, y, color.Alpha{A: uint8(cov * 255 / (ss * ss))})
		}
	}
	return m
}

func inRoundedRect(px, py, w, h, r float64) bool {
	if px < 0 || py < 0 || px > w || py > h {
		return false
	}
	cx := math.Min(math.Max(px, r), w-r)
	cy := math.Min(math.Max(py, r), h-r)
	dx := px - cx
	dy := py - cy
	return dx*dx+dy*dy <= r*r
}

// ApplyMask multiplies the alpha channel of img by the given mask.
func ApplyMask(img *image.RGBA, mask *image.Alpha) {
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			a := mask.AlphaAt(x, y).A
			c.A = uint8(uint32(c.A) * uint32(a) / 255)
			img.SetRGBA(b.Min.X+x, b.Min.Y+y, c)
		}
	}
}

// PlaceOptions controls how an image layer is composited onto the canvas.
type PlaceOptions struct {
	X, Y       int
	W, H       int
	Radius     int
	Fit        FitMode
	Shadow     bool
	ShadowBlur int
	ShadowDX   int
	ShadowDY   int
	ShadowA    float64
	Border     string
	BorderW    int
	Overlay    float64
	OverlayCol string
	// Blur 虚化强度（缩放后应用，radius 像素）
	Blur int
	// Rotate 旋转角度（度），用于斜放贴图效果
	Rotate float64
	// Persp / Shear 透视梯形变换：把平面贴图映射成"斜放表面"（如斜放的屏幕、
	// 地面、墙面），比单纯旋转更贴合三维透视。Persp=顶边相对底边的宽度收缩比例
	// (0=矩形, 0.5=顶边半宽)；Shear=顶边相对底边的水平平移(宽度比例, 正=顶边右移)，
	// 用于左右倾斜的屏幕。梯形之外区域透明。
	Persp float64
	Shear float64
	// Opacity 图层整体不透明度 0~1（≤0 或 ≥1 视为不透明）
	Opacity float64
}

// clampPersp 把透视收缩比例限制在 [0, 0.9]，避免顶边收缩到 0 或反向。
func clampPersp(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 0.9 {
		return 0.9
	}
	return p
}

// WarpTrapezoid 把图像映射成一个梯形（上窄下宽或由 Shear 控制的斜梯形），
// 用于把画面内容"贴"到透视倾斜的平面（如斜放的屏幕、地面、墙面），比单纯旋转更贴合。
// 返回大小仍为 w×h，梯形之外的区域透明（露出底下的画布）。采用双线性采样保边。
func WarpTrapezoid(src *image.RGBA, w, h int, persp, shear float64) *image.RGBA {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	if sw <= 0 || sh <= 0 {
		return out
	}
	persp = clampPersp(persp)
	topW := float64(w) * (1 - persp)
	topCx := float64(w)/2 + shear*float64(w)/2
	xT0 := topCx - topW/2
	xT1 := topCx + topW/2
	xB0, xB1 := 0.0, float64(w)
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		// 顶(y=0)窄、底(y=h)宽：顶部用窄上边，底部用全宽下边。
		leftX := xT0 + (xB0-xT0)*t
		rightX := xT1 + (xB1-xT1)*t
		span := rightX - leftX
		if span <= 0 {
			continue
		}
		for x := 0; x < w; x++ {
			u := (float64(x) - leftX) / span
			if u < 0 || u > 1 {
				continue // 梯形外 → 透明
			}
			out.Set(x, y, sampleBilinear(src, u*(sw-1), t*(sh-1)))
		}
	}
	return out
}

// sampleBilinear 对源图像做双线性采样（坐标基于源像素空间）。
func sampleBilinear(src *image.RGBA, fx, fy float64) color.RGBA {
	b := src.Bounds()
	if fx < 0 {
		fx = 0
	}
	if fy < 0 {
		fy = 0
	}
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	x1 := x0 + 1
	y1 := y0 + 1
	if x1 >= b.Dx() {
		x1 = b.Dx() - 1
	}
	if y1 >= b.Dy() {
		y1 = b.Dy() - 1
	}
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	dx := fx - float64(x0)
	dy := fy - float64(y0)
	c00 := src.RGBAAt(b.Min.X+x0, b.Min.Y+y0)
	c10 := src.RGBAAt(b.Min.X+x1, b.Min.Y+y0)
	c01 := src.RGBAAt(b.Min.X+x0, b.Min.Y+y1)
	c11 := src.RGBAAt(b.Min.X+x1, b.Min.Y+y1)
	return bilinear4(c00, c10, c01, c11, dx, dy)
}

// bilinear4 四角双线性插值；alpha 通道用预乘方式插值，避免半透明边缘发暗。
func bilinear4(c00, c10, c01, c11 color.RGBA, dx, dy float64) color.RGBA {
	// 预乘后的 RGB（按 alpha 权重），分别与 alpha 一起插值。
	pm := func(c color.RGBA) (float64, float64, float64, float64) {
		a := float64(c.A) / 255
		return float64(c.R) * a, float64(c.G) * a, float64(c.B) * a, a
	}
	r00, g00, b00, a00 := pm(c00)
	r10, g10, b10, a10 := pm(c10)
	r01, g01, b01, a01 := pm(c01)
	r11, g11, b11, a11 := pm(c11)
	lerp := func(a, b, d float64) float64 { return a*(1-d) + b*d }
	// 顶 / 底 边插值
	rt0, gt0, bt0, at0 := lerp(r00, r10, dx), lerp(g00, g10, dx), lerp(b00, b10, dx), lerp(a00, a10, dx)
	rt1, gt1, bt1, at1 := lerp(r01, r11, dx), lerp(g01, g11, dx), lerp(b01, b11, dx), lerp(a01, a11, dx)
	R, G, B, A := lerp(rt0, rt1, dy), lerp(gt0, gt1, dy), lerp(bt0, bt1, dy), lerp(at0, at1, dy)
	if A <= 0.0001 {
		return color.RGBA{A: 0}
	}
	// 反预乘回 0~255 的非预乘色（R/G/B/A 此时在 0~255 量级，按 255 夹取，
	// 不能用 clamp01——否则会把 >1 的颜色通道压成 255 全白）。
	return color.RGBA{
		R: uint8(clamp(R/A, 0, 255)),
		G: uint8(clamp(G/A, 0, 255)),
		B: uint8(clamp(B/A, 0, 255)),
		A: uint8(clamp(A*255, 0, 255)),
	}
}

// PlaceImage composites an image onto the canvas honouring the options.
func (c *Canvas) PlaceImage(src *image.RGBA, opt PlaceOptions) {
	w, h := opt.W, opt.H
	if w < 1 || h < 1 {
		return
	}
	layer := Resize(src, w, h, opt.Fit)
	if opt.Radius > 0 {
		ApplyMask(layer, RoundedMask(w, h, opt.Radius))
	}
	// 虚化（先于旋转/透视，保证糊面覆盖整个贴图框）
	if opt.Blur > 0 {
		layer = Blur(layer, opt.Blur)
	}
	// 透视梯形：把平面贴图映射成斜放表面（先于旋转，旋转作用在已透视的梯形上）。
	if opt.Persp != 0 || opt.Shear != 0 {
		layer = WarpTrapezoid(layer, w, h, opt.Persp, opt.Shear)
	}
	// 斜放：旋转后空隙保持透明，露出底下的画布
	if opt.Rotate != 0 {
		layer = Rotate(layer, opt.Rotate, false, nil)
	}
	// 图层整体不透明度：在合成前缩放整层 alpha。
	if opt.Opacity > 0 && opt.Opacity < 1 {
		b := layer.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				p := layer.RGBAAt(x, y)
				p.A = uint8(float64(p.A) * opt.Opacity)
				layer.SetRGBA(x, y, p)
			}
		}
	}
	// darkening overlay
	if opt.Overlay > 0 {
		ov := ParseHex(orDefault(opt.OverlayCol, "#000000"))
		ov.A = uint8(clamp01(opt.Overlay) * 255)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				layer.SetRGBA(x, y, alphaBlend(layer.RGBAAt(x, y), ov))
			}
		}
	}
	// drop shadow
	if opt.Shadow {
		blur := opt.ShadowBlur
		if blur <= 0 {
			blur = 18
		}
		sh := blurRGBA(layer, blur)
		a := opt.ShadowA
		if a <= 0 {
			a = 0.45
		}
		// The rounded mask dilutes the shadow's alpha, so it is boosted to
		// keep the drop shadow visible.
		boostAlpha(sh, 2.0)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				p := sh.RGBAAt(x, y)
				p.A = uint8(float64(p.A) * a)
				c.Set(opt.X+x+opt.ShadowDX, opt.Y+y+opt.ShadowDY, p)
			}
		}
	}
	c.DrawLayer(layer, opt.X, opt.Y)
	// border
	if opt.Border != "" && opt.BorderW > 0 {
		bc := ParseHex(opt.Border)
		bw := opt.BorderW
		r := opt.Radius
		for i := 0; i < bw; i++ {
			alpha := uint8(clamp01(1-float64(i)/float64(bw)) * 255)
			col := color.RGBA{bc.R, bc.G, bc.B, alpha}
			strokeRoundedRect(c, opt.X, opt.Y, w, h, r, col, i)
		}
	}
}

// strokeRoundedRect draws one anti-aliased rounded-rect outline at inset i.
func strokeRoundedRect(c *Canvas, x, y, w, h, r int, col color.RGBA, inset int) {
	const ss = 3
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			cov := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					fx := float64(px) + (float64(sx)+0.5)/ss
					fy := float64(py) + (float64(sy)+0.5)/ss
					if inRoundedRect(fx, fy, float64(w), float64(h), float64(r)) {
						cov++
					}
				}
			}
			if cov == 0 {
				continue
			}
			// keep only pixels near the border ring
			inner := inRoundedRect(fx2(px, inset), fy2(py, inset), float64(w-2*inset), float64(h-2*inset), float64(maxInt(r-inset, 0)))
			if inner {
				continue
			}
			c.Set(x+px, y+py, col)
		}
	}
}

func fx2(v, inset int) float64 { return float64(v) + 0.5 }
func fy2(v, inset int) float64 { return float64(v) + 0.5 }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
