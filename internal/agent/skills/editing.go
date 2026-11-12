package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Layout is the LLM-planned composition design. It lets the base model decide
// the visual language (palette, title placement, whether to use thumbnails,
// decorative rules, tagline) instead of hard-coding one fixed template.
type Layout struct {
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Tagline     string   `json:"tagline"`
	Palette     []string `json:"palette"`      // bg gradient colours
	Accent      string   `json:"accent"`       // accent colour for rules
	TextColor   string   `json:"text_color"`   // main title colour
	TextAccent  string   `json:"text_accent"`  // subtitle colour
	HeroOverlay float64  `json:"hero_overlay"` // darkening over key visual
	ScrimSide   string   `json:"scrim_side"`   // right | left | bottom | none
	ShowThumbs  *bool    `json:"show_thumbs"`  // nil = use default (true)
	TitleX      float64  `json:"title_x"`      // 0..1 relative position
	TitleY      float64  `json:"title_y"`
	TitleSize    float64 `json:"title_size"` // relative to height
	Badge        string   `json:"badge"`
	Footer       string   `json:"footer"`
	// 创意效果：斜体标题 / 斜放主视觉（度数，0 不斜）
	TitleItalic bool    `json:"title_italic"`
	HeroRotate  float64 `json:"hero_rotate"`
	HeroBlur    int     `json:"hero_blur"`
}

// showThumbsDefault is the default for the thumbnail-card row.
var showThumbsDefault = true

// DefaultLayout returns a sensible cinematic layout when the LLM is unavailable.
func DefaultLayout(title, subtitle string) *Layout {
	return &Layout{
		Title:       title,
		Subtitle:    subtitle,
		Tagline:     "",
		Palette:     []string{"#050912", "#0d1a30"},
		Accent:      "#ffcf5c",
		TextColor:   "#ffffff",
		TextAccent:  "#a9c9f5",
		HeroOverlay: 0.30,
		ScrimSide:   "right",
		ShowThumbs:  &showThumbsDefault,
		TitleX:      0.055,
		TitleY:      0.30,
		TitleSize:   0.085,
		Footer:      "VIXIMAGE AGENT · KEY VISUAL",
	}
}

// WithDefaults fills in any field the LLM left at its zero value so the
// renderer always has a complete, sane design to work with.
func (l *Layout) WithDefaults() *Layout {
	out := *l
	if out.Title == "" {
		out.Title = "VixImage 作品"
	}
	if len(out.Palette) < 2 {
		out.Palette = []string{"#050912", "#0d1a30"}
	}
	if out.Accent == "" {
		out.Accent = "#ffcf5c"
	}
	if out.TextColor == "" {
		out.TextColor = "#ffffff"
	}
	if out.TextAccent == "" {
		out.TextAccent = "#a9c9f5"
	}
	if out.ScrimSide == "" {
		out.ScrimSide = "right"
	}
	if out.HeroOverlay <= 0 {
		out.HeroOverlay = 0.30
	}
	if out.TitleX <= 0 {
		out.TitleX = 0.055
	}
	if out.TitleY <= 0 {
		out.TitleY = 0.30
	}
	if out.TitleSize <= 0 {
		out.TitleSize = 0.085
	}
	if out.Footer == "" {
		out.Footer = "VIXIMAGE AGENT · KEY VISUAL"
	}
	// A nil show_thumbs means the model did not decide → keep thumbnails on.
	if out.ShowThumbs == nil {
		t := true
		out.ShowThumbs = &t
	}
	return &out
}

// Compose renders the final piece using a planned layout.
// The first image becomes the full-bleed key visual (cover-cropped so there
// are never black bars); the rest become thumbnail cards.
func (c *Context) Compose(images []string, layout *Layout, width, height int) (string, error) {
	if width <= 0 {
		width = 1920
	}
	if height <= 0 {
		height = 1080
	}
	// 允许 0 张图：纯文字/纯排版海报（不再强制至少一张图片）。
	if layout == nil {
		layout = DefaultLayout("", "")
	}

	items := []map[string]any{}
	margin := int(float64(width) * 0.055)

	// ---- key visual, full bleed ----
	if len(images) > 0 {
		items = append(items, map[string]any{
			"type": "image", "src": images[0], "x": 0, "y": 0,
			"w": width, "h": height, "fit": "cover",
			"overlay": layout.HeroOverlay, "overlay_color": "#050912",
			"rotate": layout.HeroRotate, "blur": layout.HeroBlur,
		})
	}

	// ---- readability scrim ----
	// One full-canvas veil only. Splitting it into overlapping regions would
	// leave visible seams where the pieces meet.
	switch layout.ScrimSide {
	case "none":
		// no directional scrim
	case "left":
		items = append(items, map[string]any{
			"type": "scrim", "x": 0, "y": 0,
			"w": width, "h": height,
			"color": "#04070f", "direction": "left", "strength": 0.86,
		})
	case "bottom":
		items = append(items, map[string]any{
			"type": "scrim", "x": 0, "y": 0,
			"w": width, "h": height,
			"color": "#04070f", "direction": "bottom", "strength": 0.84,
		})
	case "top":
		items = append(items, map[string]any{
			"type": "scrim", "x": 0, "y": 0,
			"w": width, "h": height,
			"color": "#04070f", "direction": "top", "strength": 0.84,
		})
	default: // "right" → darkest at the left edge where the text lives
		items = append(items, map[string]any{
			"type": "scrim", "x": 0, "y": 0,
			"w": width, "h": height,
			"color": "#04070f", "direction": "right", "strength": 0.86,
		})
	}

	// ---- thumbnail cards ----
	rest := images[1:]
	if len(rest) > 3 {
		rest = rest[:3]
	}
	if layout.ShowThumbs != nil && *layout.ShowThumbs && len(rest) > 0 {
		gap := int(float64(width) * 0.016)
		avail := int(float64(width) * 0.30)
		cardW := (avail - gap*(len(rest)-1)) / len(rest)
		cardH := int(float64(cardW) * 0.62)
		cardY := height - margin - cardH
		for i, img := range rest {
			cx := width - margin - cardW - i*(cardW+gap)
			items = append(items, map[string]any{
				"type": "image", "src": img, "x": cx, "y": cardY,
				"w": cardW, "h": cardH, "fit": "cover", "radius": 12,
				"shadow": true, "shadow_dx": 4, "shadow_dy": 8,
				"border": "#5aa9ff", "border_width": 1,
				"overlay": 0.12, "overlay_color": "#050912",
			})
		}
	}

	// ---- text block ----
	tx := int(float64(width) * clamp01(layout.TitleX))
	ty := int(float64(height) * clamp01(layout.TitleY))
	tsize := int(float64(height) * clamp01(layout.TitleSize))
	if tsize < 24 {
		tsize = 24
	}
	accent := orDefault(layout.Accent, "#ffcf5c")
	textCol := orDefault(layout.TextColor, "#ffffff")
	subCol := orDefault(layout.TextAccent, "#a9c9f5")

	// The title may span several lines; every element below it must follow the
	// real text block height, otherwise the rule and subtitle overlap the title.
	titleLines := 1
	if layout.Title != "" {
		titleLines = len(strings.Split(layout.Title, "\n"))
	}
	titleBlockH := tsize * titleLines
	// Shrink the title when a single line would overflow the text column.
	if titleLines == 1 && tsize > 0 {
		maxChars := 18
		if r := len([]rune(layout.Title)); r > maxChars {
			tsize = int(float64(tsize) * float64(maxChars) / float64(r))
			if tsize < 28 {
				tsize = 28
			}
			titleBlockH = tsize
		}
	}
	lineY := ty + titleBlockH + 14

	if layout.Title != "" {
		items = append(items, map[string]any{
			"type": "text", "text": layout.Title, "x": tx, "y": ty,
			"size": tsize, "color": textCol, "weight": "bold",
			"italic": layout.TitleItalic,
			"glow": "#3d8bff", "glow_size": 30,
			"shadow": true, "shadow_blur": 16,
			"line_height": 1.16,
		})
		items = append(items, map[string]any{
			"type": "line", "p1": []int{tx, lineY},
			"p2": []int{tx + int(float64(width)*0.10), lineY},
			"color": accent, "width": 5,
		})
	}
	subY := lineY + 22
	if layout.Subtitle != "" {
		items = append(items, map[string]any{
			"type": "text", "text": layout.Subtitle, "x": tx + 2,
			"y": subY, "size": int(float64(height) * 0.030),
			"color": subCol,
		})
	}
	if layout.Tagline != "" {
		items = append(items, map[string]any{
			"type": "text", "text": layout.Tagline, "x": tx + 2,
			"y": subY + int(float64(height)*0.040), "size": int(float64(height) * 0.023),
			"color": accent,
		})
	}
	if layout.Badge != "" {
		items = append(items, map[string]any{
			"type": "text", "text": layout.Badge, "x": tx + 2,
			"y": ty - int(float64(height)*0.045),
			"size": int(float64(height) * 0.024), "color": accent,
			"spacing": 2,
		})
	}
	if layout.Footer != "" {
		items = append(items, map[string]any{
			"type": "text", "text": layout.Footer,
			"x": width - margin, "y": margin,
			"size": int(float64(height) * 0.021), "color": "#7fb0e8",
			"anchor": "ra", "mono": true, "spacing": 2,
		})
	}

	pal := layout.Palette
	if len(pal) < 2 {
		pal = []string{"#050912", "#0d1a30"}
	}
	spec := map[string]any{
		"width":  width,
		"height": height,
		"bg":     map[string]any{"type": "gradient", "colors": pal, "angle": 90},
		"items":  items,
		"output": filepath.Join(c.OutDir, fmt.Sprintf("compose_%d.png", os.Getpid())),
	}
	return c.Edit(spec)
}
