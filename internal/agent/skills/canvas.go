package skills

import (
	"image"
	"image/color"
	"math"
)

// ---------------------------------------------------------------------------
// 颜色与插值
// ---------------------------------------------------------------------------

// ParseHex parses "#RGB", "#RRGGBB", "#RRGGBBAA" into an RGBA value.
func ParseHex(s string) color.RGBA {
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	hexToByte := func(c byte) uint8 {
		switch {
		case c >= '0' && c <= '9':
			return c - '0'
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10
		case c >= 'A' && c <= 'F':
			return c - 'A' + 10
		}
		return 0
	}
	switch len(s) {
	case 3:
		r := hexToByte(s[0])
		g := hexToByte(s[1])
		b := hexToByte(s[2])
		return color.RGBA{r * 17, g * 17, b * 17, 255}
	case 6:
		return color.RGBA{
			hexToByte(s[0])<<4 | hexToByte(s[1]),
			hexToByte(s[2])<<4 | hexToByte(s[3]),
			hexToByte(s[4])<<4 | hexToByte(s[5]),
			255,
		}
	case 8:
		return color.RGBA{
			hexToByte(s[0])<<4 | hexToByte(s[1]),
			hexToByte(s[2])<<4 | hexToByte(s[3]),
			hexToByte(s[4])<<4 | hexToByte(s[5]),
			hexToByte(s[6])<<4 | hexToByte(s[7]),
		}
	}
	return color.RGBA{0, 0, 0, 255}
}

func lerp(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

func lerpFloat(a, b float64, t float64) float64 { return a + (b-a)*t }

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// clamp 把 v 限制在 [lo, hi] 区间（用于 0~255 量级的颜色通道）。
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func smoothstep(t float64) float64 {
	t = clamp01(t)
	return t * t * (3 - 2*t)
}

// ---------------------------------------------------------------------------
// 画布
// ---------------------------------------------------------------------------

// Canvas is an RGBA drawing surface with premultiplied-alpha compositing
// helpers used by the Go-native image editing skill.
type Canvas struct {
	Img *image.RGBA
}

// NewCanvas allocates a transparent canvas.
func NewCanvas(w, h int) *Canvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Canvas{Img: image.NewRGBA(image.Rect(0, 0, w, h))}
}

// Width returns canvas width in pixels.
func (c *Canvas) Width() int { return c.Img.Bounds().Dx() }

// Height returns canvas height in pixels.
func (c *Canvas) Height() int { return c.Img.Bounds().Dy() }

// At returns the pixel colour at (x, y).
func (c *Canvas) At(x, y int) color.RGBA {
	return c.Img.RGBAAt(x, y)
}

// Set writes a pixel, alpha-blending it over the existing content.
func (c *Canvas) Set(x, y int, col color.RGBA) {
	if x < 0 || y < 0 || x >= c.Width() || y >= c.Height() {
		return
	}
	c.Img.SetRGBA(x, y, alphaBlend(c.At(x, y), col))
}

// alphaBlend composites src over dst using straight (non-premultiplied) alpha.
func alphaBlend(dst, src color.RGBA) color.RGBA {
	if src.A == 0 {
		return dst
	}
	if src.A == 255 {
		return src
	}
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return color.RGBA{}
	}
	return color.RGBA{
		R: uint8((float64(src.R)*sa + float64(dst.R)*da*(1-sa)) / outA),
		G: uint8((float64(src.G)*sa + float64(dst.G)*da*(1-sa)) / outA),
		B: uint8((float64(src.B)*sa + float64(dst.B)*da*(1-sa)) / outA),
		A: uint8(outA * 255),
	}
}

// FillGradient paints a linear gradient across the canvas.
// angle 0 = left→right, 90 = top→bottom.
func (c *Canvas) FillGradient(c0, c1 color.RGBA, angle float64) {
	w, h := c.Width(), c.Height()
	vertical := math.Abs(math.Mod(angle, 180)) >= 45 && math.Abs(math.Mod(angle, 180)) <= 135
	if vertical {
		for y := 0; y < h; y++ {
			t := 0.0
			if h > 1 {
				t = float64(y) / float64(h-1)
			}
			col := color.RGBA{
				R: lerp(c0.R, c1.R, t),
				G: lerp(c0.G, c1.G, t),
				B: lerp(c0.B, c1.B, t),
				A: lerp(c0.A, c1.A, t),
			}
			for x := 0; x < w; x++ {
				c.Img.SetRGBA(x, y, col)
			}
		}
		return
	}
	for x := 0; x < w; x++ {
		t := 0.0
		if w > 1 {
			t = float64(x) / float64(w-1)
		}
		col := color.RGBA{
			R: lerp(c0.R, c1.R, t),
			G: lerp(c0.G, c1.G, t),
			B: lerp(c0.B, c1.B, t),
			A: lerp(c0.A, c1.A, t),
		}
		for y := 0; y < h; y++ {
			c.Img.SetRGBA(x, y, col)
		}
	}
}

// FillSolid paints the whole canvas with one colour.
func (c *Canvas) FillSolid(col color.RGBA) {
	for y := 0; y < c.Height(); y++ {
		for x := 0; x < c.Width(); x++ {
			c.Img.SetRGBA(x, y, col)
		}
	}
}

// ---------------------------------------------------------------------------
// 蒙版
// ---------------------------------------------------------------------------

// Scrim paints a directional gradient veil over the canvas region.
// direction: "right" (darkest at the left edge), "left" (darkest at the right
// edge), "bottom" (darkest at the bottom), "top" (darkest at the top).
// The veil always fades to fully transparent at its far edge, so no hard
// seam is visible where the scrim region ends.
func (c *Canvas) Scrim(x, y, w, h int, col color.RGBA, direction string, strength float64) {
	if w <= 0 || h <= 0 {
		return
	}
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			var f float64
			switch direction {
			case "left": // darkest at the right edge
				f = smoothstep(float64(xx) / float64(maxInt(w-1, 1)))
			case "bottom": // darkest at the bottom
				f = smoothstep(float64(yy) / float64(maxInt(h-1, 1)))
			case "top": // darkest at the top
				f = 1 - smoothstep(float64(yy)/float64(maxInt(h-1, 1)))
			default: // "right" → darkest at the left edge
				f = 1 - smoothstep(float64(xx)/float64(maxInt(w-1, 1)))
			}
			a := uint8(clamp01(strength*f*f*(3-2*f)) * 255)
			c.Set(x+xx, y+yy, color.RGBA{col.R, col.G, col.B, a})
		}
	}
}

// ---------------------------------------------------------------------------
// 模糊（用于投影与辉光）
// ---------------------------------------------------------------------------

// blurRGBA applies a separable box blur (approximating a Gaussian) to a layer.
func blurRGBA(src *image.RGBA, radius int) *image.RGBA {
	if radius < 1 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tmp := image.NewRGBA(b)
	dst := image.NewRGBA(b)

	// horizontal pass
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, bl, a, n uint32
			for k := -radius; k <= radius; k++ {
				xx := x + k
				if xx < 0 {
					xx = 0
				}
				if xx >= w {
					xx = w - 1
				}
				c := src.RGBAAt(xx, y)
				r += uint32(c.R)
				g += uint32(c.G)
				bl += uint32(c.B)
				a += uint32(c.A)
				n++
			}
			tmp.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	// vertical pass
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, bl, a, n uint32
			for k := -radius; k <= radius; k++ {
				yy := y + k
				if yy < 0 {
					yy = 0
				}
				if yy >= h {
					yy = h - 1
				}
				c := tmp.RGBAAt(x, yy)
				r += uint32(c.R)
				g += uint32(c.G)
				bl += uint32(c.B)
				a += uint32(c.A)
				n++
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	return dst
}

// boostAlpha multiplies the alpha channel of an image, saturating at 255.
func boostAlpha(img *image.RGBA, factor float64) {
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			c.A = uint8(clamp01(float64(c.A)*factor/255) * 255)
			img.SetRGBA(b.Min.X+x, b.Min.Y+y, c)
		}
	}
}

// DrawLayer composites a layer onto the canvas at (x, y).
func (c *Canvas) DrawLayer(layer *image.RGBA, x, y int) {
	b := layer.Bounds()
	for yy := 0; yy < b.Dy(); yy++ {
		for xx := 0; xx < b.Dx(); xx++ {
			c.Set(x+xx, y+yy, layer.RGBAAt(b.Min.X+xx, b.Min.Y+yy))
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
