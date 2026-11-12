package skills

import (
	"image"
	"image/color"
	"os"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// ---------------------------------------------------------------------------
// 字体加载（CJK 支持）
// ---------------------------------------------------------------------------

var (
	fontOnce   sync.Once
	cjkBold    *opentype.Collection
	cjkRegular *opentype.Collection
	singleBold *opentype.Font
	singleReg  *opentype.Font
	monoFont   *opentype.Font
)

var boldFontPaths = []string{
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
	"/usr/share/fonts/opentype/noto/NotoSansCJKsc-Bold.otf",
	"/usr/share/fonts/truetype/noto/NotoSansCJKsc-Bold.otf",
	"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/System/Library/Fonts/PingFang.ttc",
}

var regularFontPaths = []string{
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/noto/NotoSansCJKsc-Regular.otf",
	"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
	"/System/Library/Fonts/PingFang.ttc",
}

var monoFontPaths = []string{
	"/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
}

// scIndex is the TTC sub-font index for Simplified Chinese in Noto Sans CJK.
const scIndex = 2

// loadCollection loads the first font collection (.ttc) found in paths.
func loadCollection(paths []string) *opentype.Collection {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if col, err := opentype.ParseCollection(b); err == nil {
			return col
		}
	}
	return nil
}

// loadSingleFont loads the first standalone font file found in paths.
func loadSingleFont(paths []string) *opentype.Font {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if f, err := opentype.Parse(b); err == nil {
			return f
		}
	}
	return nil
}

func loadFonts() {
	fontOnce.Do(func() {
		cjkBold = loadCollection(boldFontPaths)
		cjkRegular = loadCollection(regularFontPaths)
		singleBold = loadSingleFont(boldFontPaths)
		singleReg = loadSingleFont(regularFontPaths)
		for _, p := range monoFontPaths {
			if b, err := os.ReadFile(p); err == nil {
				if f, err := opentype.Parse(b); err == nil {
					monoFont = f
					break
				}
			}
		}
	})
}

// faceFor builds a font face at the requested size/weight.
func faceFor(size int, weight string, mono bool) (font.Face, error) {
	loadFonts()
	opts := &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull}
	if mono && monoFont != nil {
		return opentype.NewFace(monoFont, opts)
	}
	col := cjkRegular
	if weight == "bold" {
		col = cjkBold
	}
	if col != nil {
		f, err := col.Font(scIndex)
		if err == nil {
			return opentype.NewFace(f, opts)
		}
		for i := 0; i < col.NumFonts(); i++ {
			if ff, e := col.Font(i); e == nil {
				return opentype.NewFace(ff, opts)
			}
		}
	}
	// Fall back to a standalone font file.
	single := singleReg
	if weight == "bold" {
		single = singleBold
	}
	if single == nil {
		single = singleBold
	}
	if single == nil {
		single = singleReg
	}
	if single != nil {
		return opentype.NewFace(single, opts)
	}
	return nil, errNoFont
}

type fontError string

func (e fontError) Error() string { return string(e) }

const errNoFont = fontError("no usable CJK font found on this system")

// ---------------------------------------------------------------------------
// 文字绘制
// ---------------------------------------------------------------------------

// TextOptions describes a text run to render.
type TextOptions struct {
	Text        string
	X, Y        int
	Size        int
	Color       color.RGBA
	Weight      string
	Mono        bool
	Anchor      string // "la" (left-ascender), "ma", "ra"
	Spacing     int
	LineHeight  float64
	GlowColor   color.RGBA
	GlowSize    int
	StrokeColor color.RGBA
	StrokeW     int
	Shadow      bool
	ShadowBlur  int
	// Italic 斜体字：渲染后对文字层做斜切变换。
	Italic bool
	// MaxWidth 自动折行宽度（像素）。>0 时把超出该宽度的文本按字测量折成多行，
	// 避免长段落被画布右边界裁切。已有的 \n 手动换行会保留。0=不折行。
	MaxWidth int
}

// DrawText renders text (multi-line supported) onto the canvas.
func (c *Canvas) DrawText(opt TextOptions) {
	if opt.Text == "" {
		return
	}
	if opt.Size <= 0 {
		opt.Size = 30
	}
	if opt.LineHeight <= 0 {
		opt.LineHeight = 1.28
	}
	if opt.Anchor == "" {
		opt.Anchor = "la"
	}
	// The monospace font only covers ASCII; CJK characters would render as
	// empty boxes. Fall back to the CJK face whenever the text has non-ASCII.
	if opt.Mono && hasNonASCII(opt.Text) {
		opt.Mono = false
	}
	face, err := faceFor(opt.Size, opt.Weight, opt.Mono)
	if err != nil {
		return
	}
	defer face.Close()

	measure := func(s string) int {
		d := &font.Drawer{Face: face}
		return d.MeasureString(s).Ceil()
	}

	lines := splitLines(opt.Text)
	if opt.MaxWidth > 0 {
		lines = wrapLines(lines, measure, opt.MaxWidth, opt.Spacing)
	}
	lh := int(float64(opt.Size) * opt.LineHeight)

	// The rendered layer is padded, so its origin must be shifted back by the
	// padding to keep the anchor point exact.
	pad := opt.GlowSize + opt.StrokeW + opt.ShadowBlur + 4
	textWidth := 0
	for _, ln := range lines {
		w := measure(ln)
		if opt.Spacing > 0 {
			w += opt.Spacing * maxInt(len([]rune(ln))-1, 0)
		}
		if w > textWidth {
			textWidth = w
		}
	}
	originX := opt.X - pad
	switch opt.Anchor[0] {
	case 'm':
		originX = opt.X - textWidth/2 - pad
	case 'r':
		originX = opt.X - textWidth - pad
	}

	// shadow
	if opt.Shadow {
		blur := opt.ShadowBlur
		if blur <= 0 {
			blur = 12
		}
		sh := c.renderTextLayer(lines, face, opt, lh, measure, color.RGBA{0, 0, 0, 190})
		sh = blurRGBA(sh, blur)
		c.DrawLayer(sh, originX, opt.Y-pad)
	}

	// glow — rendered as a bright halo around the glyphs. The blurred layer's
	// alpha is heavily diluted, so it is boosted before compositing.
	if opt.GlowSize > 0 {
		gl := c.renderTextLayer(lines, face, opt, lh, measure, color.RGBA{
			opt.GlowColor.R, opt.GlowColor.G, opt.GlowColor.B, 255,
		})
		gl = blurRGBA(gl, opt.GlowSize)
		boostAlpha(gl, 2.2)
		c.DrawLayer(gl, originX, opt.Y-pad)
	}

	// stroke
	if opt.StrokeW > 0 {
		for _, d := range [][2]int{{-opt.StrokeW, 0}, {opt.StrokeW, 0}, {0, -opt.StrokeW}, {0, opt.StrokeW},
			{-opt.StrokeW, -opt.StrokeW}, {opt.StrokeW, -opt.StrokeW}, {-opt.StrokeW, opt.StrokeW}, {opt.StrokeW, opt.StrokeW}} {
			st := c.renderTextLayer(lines, face, opt, lh, measure, opt.StrokeColor)
			c.DrawLayer(st, originX+d[0], opt.Y-pad+d[1])
		}
	}

	// main fill
	main := c.renderTextLayer(lines, face, opt, lh, measure, opt.Color)
	c.DrawLayer(main, originX, opt.Y-pad)
}

// renderTextLayer draws the text into its own transparent layer so that
// blur/glow effects can be applied without touching the canvas.
func (c *Canvas) renderTextLayer(lines []string, face font.Face, opt TextOptions, lh int,
	measure func(string) int, col color.RGBA) *image.RGBA {
	// compute bounds
	maxW := 0
	for _, ln := range lines {
		w := measure(ln)
		if opt.Spacing > 0 {
			w += opt.Spacing * maxInt(len([]rune(ln))-1, 0)
		}
		if w > maxW {
			maxW = w
		}
	}
	pad := opt.GlowSize + opt.StrokeW + opt.ShadowBlur + 4
	lw := maxW + pad*2
	lh2 := lh*len(lines) + pad*2
	if lw < 1 {
		lw = 1
	}
	if lh2 < 1 {
		lh2 = 1
	}
	layer := image.NewRGBA(image.Rect(0, 0, lw, lh2))
	d := &font.Drawer{Dst: layer, Src: image.NewUniform(col), Face: face}

	for i, ln := range lines {
		w := measure(ln)
		if opt.Spacing > 0 {
			w += opt.Spacing * maxInt(len([]rune(ln))-1, 0)
		}
		// Align within the layer; the layer itself is then positioned on the
		// canvas according to the anchor (see textOrigin).
		x := pad
		switch opt.Anchor[0] {
		case 'm':
			x = pad + (maxW-w)/2
		case 'r':
			x = pad + (maxW - w)
		}
		y := pad + i*lh
		if opt.Spacing > 0 {
			for _, ch := range []rune(ln) {
				d.Dot = fixed.P(x, y+opt.Size*4/5)
				d.DrawString(string(ch))
				adv := d.MeasureString(string(ch)).Ceil()
				x += adv + opt.Spacing
			}
		} else {
			d.Dot = fixed.P(x, y+opt.Size*4/5)
			d.DrawString(ln)
		}
	}
	// 斜体字：对整层做斜切（各效果层统一处理，阴影/辉光随字斜）。
	if opt.Italic {
		layer = shearLayer(layer, 0.20)
	}
	return layer
}

// hasNonASCII reports whether the string contains characters outside ASCII.
func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}

// wrapLines 把每一行按 maxW 像素折成多行。逐 rune 测量，中文按字断行、
// 中英文混排都安全；已有的 \n 手动换行（splitLines 的结果）保持不变。
// 目的：LLM 给出的长段落即使没手动折行，也不会被画布右边界裁掉。
func wrapLines(lines []string, measure func(string) int, maxW, spacing int) []string {
	if maxW <= 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if ln == "" {
			out = append(out, "")
			continue
		}
		cur, width := "", 0
		for _, ch := range []rune(ln) {
			cw := measure(string(ch))
			if spacing > 0 {
				cw += spacing
			}
			if width+cw > maxW && cur != "" {
				out = append(out, cur)
				cur, width = "", 0
			}
			cur += string(ch)
			width += cw
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

// splitLines splits text on "\n" into lines.
func splitLines(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, cur)
	return out
}
