package skills

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// SpecItem is one element of a composition spec (JSON-driven, same schema as
// the legacy Python editor so existing specs keep working).
type SpecItem struct {
	Type string `json:"type"`

	// image
	Src         string  `json:"src"`
	X           int     `json:"x"`
	Y           int     `json:"y"`
	W           int     `json:"w"`
	H           int     `json:"h"`
	Radius      int     `json:"radius"`
	Fit         string  `json:"fit"`
	Shadow      bool    `json:"shadow"`
	ShadowBlur  int     `json:"shadow_blur"`
	ShadowDX    int     `json:"shadow_dx"`
	ShadowDY    int     `json:"shadow_dy"`
	Border      string  `json:"border"`
	BorderWidth int     `json:"border_width"`
	Overlay     float64 `json:"overlay"`
	OverlayCol  string  `json:"overlay_color"`
	// 图片处理效果：虚化、旋转（斜放）、抠图（去背景）、透视梯形（贴合斜放表面）
	Blur      int     `json:"blur"`
	Rotate    float64 `json:"rotate"`
	Persp     float64 `json:"persp"`
	Shear     float64 `json:"shear"`
	RemoveBG  bool    `json:"remove_bg"`
	RemoveTol float64 `json:"remove_tolerance"` // 抠图容差，默认 70
	// Circle 圆形裁切（把贴图裁成圆/椭圆，用于头像、星球、徽章）。
	Circle bool `json:"circle"`
	// Opacity 图层整体不透明度 0~1（≤0 或 =1 视为不透明）。
	Opacity float64 `json:"opacity"`

	// text
	Text        string `json:"text"`
	Size        int    `json:"size"`
	Color       string `json:"color"`
	Weight      string `json:"weight"`
	Anchor      string `json:"anchor"`
	Spacing     int    `json:"spacing"`
	LineHeight  float64 `json:"line_height"`
	Mono        bool   `json:"mono"`
	Italic      bool   `json:"italic"` // 斜体字
	Glow        string `json:"glow"`
	GlowSize    int    `json:"glow_size"`
	Stroke      string `json:"stroke"`
	StrokeWidth int    `json:"stroke_width"`

	// scrim
	Direction string  `json:"direction"`
	Strength  float64 `json:"strength"`

	// rect / line
	P1    []int `json:"p1"`
	P2    []int `json:"p2"`
	Width int   `json:"width"`
	Alpha float64 `json:"alpha"`
	Fill  bool   `json:"fill"`
}

// Spec is the whole composition document.
type Spec struct {
	Width  int       `json:"width"`
	Height int       `json:"height"`
	BG     *BGSpec   `json:"bg"`
	Items  []SpecItem `json:"items"`
	Output string    `json:"output"`
}

// BGSpec describes the canvas background.
type BGSpec struct {
	Type   string   `json:"type"`
	Colors []string `json:"colors"`
	Angle  float64  `json:"angle"`
	Color  string   `json:"color"`
}

// RenderSpec renders a composition spec into an RGBA image (pure Go).
func RenderSpec(spec *Spec) (*image.RGBA, error) {
	w, h := spec.Width, spec.Height
	if w <= 0 {
		w = 1600
	}
	if h <= 0 {
		h = 900
	}
	c := NewCanvas(w, h)

	// background
	if spec.BG != nil {
		switch spec.BG.Type {
		case "gradient":
			cols := spec.BG.Colors
			if len(cols) == 0 {
				cols = []string{"#0a0e1a", "#101a30"}
			}
			c0 := ParseHex(cols[0])
			c1 := ParseHex(cols[len(cols)-1])
			c.FillGradient(c0, c1, spec.BG.Angle)
		default:
			c.FillSolid(ParseHex(orDefault(spec.BG.Color, "#0a0e1a")))
		}
	}

	// 图层不透明度归一：≤0 或 >1 视为不透明。
	op := func(v float64) float64 {
		if v <= 0 || v > 1 {
			return 1
		}
		return v
	}
	for _, it := range spec.Items {
		switch it.Type {
		case "image":
			if it.Src == "" {
				continue
			}
			src, err := LoadImage(it.Src)
			if err != nil {
				continue
			}
			// 图片处理：抠图（在原分辨率上做，保留边缘质量）
			if it.RemoveBG {
				tol := it.RemoveTol
				if tol <= 0 {
					tol = 70
				}
				src = RemoveBackground(src, tol)
			}
			mode := FitCover
			if it.Fit == "contain" {
				mode = FitContain
			}
			radius := it.Radius
			if it.Circle && radius == 0 {
				radius = minInt(it.W, it.H) / 2
			}
			c.PlaceImage(src, PlaceOptions{
				X: it.X, Y: it.Y, W: it.W, H: it.H,
				Radius: radius, Fit: mode,
				Shadow: it.Shadow, ShadowBlur: it.ShadowBlur,
				ShadowDX: it.ShadowDX, ShadowDY: it.ShadowDY,
				Border: it.Border, BorderW: it.BorderWidth,
				Overlay: it.Overlay, OverlayCol: it.OverlayCol,
				Blur: it.Blur, Rotate: it.Rotate, Opacity: it.Opacity,
				Persp: it.Persp, Shear: it.Shear,
			})
		case "scrim":
			col := ParseHex(orDefault(it.Color, "#000000"))
			c.Scrim(it.X, it.Y, it.W, it.H, col, it.Direction, it.Strength*op(it.Opacity))
		case "text":
			tc := ParseHex(orDefault(it.Color, "#ffffff"))
			tc.A = uint8(float64(tc.A) * op(it.Opacity))
			// 自动折行宽度：优先用图层显式 w，否则按锚点推导到画布右边界，
			// 留 40px 安全边距，避免长段落被裁切。
			maxW := it.W
			if maxW <= 0 {
				ab := byte('l')
				if len(it.Anchor) > 0 {
					ab = it.Anchor[0]
				}
				switch ab {
				case 'm':
					maxW = 2*minInt(it.X, c.Width()-it.X) - 40
				case 'r':
					maxW = it.X - 40
				default:
					maxW = c.Width() - it.X - 40
				}
			}
			c.DrawText(TextOptions{
				Text: it.Text, X: it.X, Y: it.Y, Size: it.Size,
				Color: tc,
				Weight: it.Weight, Mono: it.Mono, Anchor: it.Anchor,
				Spacing: it.Spacing, LineHeight: it.LineHeight,
				Italic: it.Italic, MaxWidth: maxW,
				GlowColor: ParseHex(it.Glow), GlowSize: it.GlowSize,
				StrokeColor: ParseHex(it.Stroke), StrokeW: it.StrokeWidth,
			})
		case "rect":
			col := ParseHex(orDefault(it.Color, "#ffffff"))
			a := it.Alpha
			if a <= 0 {
				a = 1
			}
			col.A = uint8(clamp01(a) * op(it.Opacity) * 255)
			drawRectOutline(c, it.X, it.Y, it.W, it.H, it.Radius, col, it.Width, it.Fill)
		case "line":
			col := ParseHex(orDefault(it.Color, "#ffffff"))
			a := it.Alpha
			if a <= 0 {
				a = 1
			}
			col.A = uint8(clamp01(a) * op(it.Opacity) * 255)
			if len(it.P1) == 2 && len(it.P2) == 2 {
				drawLine(c, it.P1[0], it.P1[1], it.P2[0], it.P2[1], col, maxInt(it.Width, 1))
			}
		}
	}
	return c.Img, nil
}

func drawRectOutline(c *Canvas, x, y, w, h, radius int, col color.RGBA, width int, fill bool) {
	if w <= 0 || h <= 0 {
		return
	}
	if fill {
		for yy := 0; yy < h; yy++ {
			for xx := 0; xx < w; xx++ {
				c.Set(x+xx, y+yy, col)
			}
		}
		return
	}
	if width <= 0 {
		width = 2
	}
	for i := 0; i < width; i++ {
		strokeRoundedRect(c, x+i, y+i, w-2*i, h-2*i, maxInt(radius-i, 0), col, i)
	}
}

func drawLine(c *Canvas, x1, y1, x2, y2 int, col color.RGBA, width int) {
	dx := absInt(x2 - x1)
	dy := -absInt(y2 - y1)
	sx, sy := -1, -1
	if x1 < x2 {
		sx = 1
	}
	if y1 < y2 {
		sy = 1
	}
	err := dx + dy
	for {
		for w := 0; w < width; w++ {
			c.Set(x1+w, y1, col)
			c.Set(x1+w, y1+width-1, col)
		}
		if x1 == x2 && y1 == y2 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x1 += sx
		}
		if e2 <= dx {
			err += dx
			y1 += sy
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// RenderSpecToFile renders a spec and writes the PNG output.
func RenderSpecToFile(spec *Spec) (string, error) {
	img, err := RenderSpec(spec)
	if err != nil {
		return "", err
	}
	out := spec.Output
	if out == "" {
		out = filepath.Join(os.TempDir(), "viximage_out.png")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", err
	}
	return out, nil
}

// LoadSpec parses a spec JSON file.
func LoadSpec(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Spec
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("解析规格失败: %w", err)
	}
	return &s, nil
}
