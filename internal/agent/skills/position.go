package skills

import (
	"strings"
)

// ---------------------------------------------------------------------------
// 坐标定点（Precision Positioning）
//
// 目标：让「按坐标排版」从心算绝对像素，升级为语义锚点 + 百分比 + 安全边距，
// 显著提升文字/元素落点的准确度（尤其对面小、不会算大像素的模型）。
//
// 三种定点方式（可叠加，优先级：显式 X/Y > 百分比 > 九宫格锚点）：
//   1) 九宫格锚点 pos + 偏移 ox/oy + 内边距 inset
//   2) 百分比坐标 px/py/pw/ph（相对画布宽高，0~100 或 0~1）
//   3) 绝对像素 X/Y/W/H（原有行为，完全向后兼容）
//
// 换算在 agent 层渲染前调用 ResolvePosition 完成，底层渲染引擎无需改动。
// ---------------------------------------------------------------------------

// PositionSpec 描述一个图层的定点意图。零值表示不启用定点，沿用绝对坐标。
type PositionSpec struct {
	Pos   string  // 九宫格锚点：tl/tc/tr/ml/mc/mr/bl/bc/br + 中英文别名
	OX    int     // 相对锚点的水平像素偏移（可负）
	OY    int     // 相对锚点的垂直像素偏移（可负）
	Inset int     // 锚点到画布边缘的安全内边距（像素；<=0 时按短边 4%）
	PX    float64 // 左/中心 x 百分比（0~100；也接受 0~1）
	PY    float64 // 上/中心 y 百分比
	PW    float64 // 宽百分比
	PH    float64 // 高百分比
	HasPX bool
	HasPY bool
	HasPW bool
	HasPH bool
}

// Positioned 是换算后的绝对像素坐标。
type Positioned struct {
	X, Y, W, H int
}

// normAnchor 把各种锚点写法归一成 3x3 编码：第 1 位垂直 t/m/b，第 2 位水平 l/c/r。
// 返回 "" 表示无法识别（调用方应报错，不臆造）。
func normAnchor(s string) string {
	key := strings.ToLower(strings.TrimSpace(s))
	// 1) 中文别名（按原词匹配，不做字符替换）。
	switch key {
	case "左上", "上左":
		return "tl"
	case "上中", "顶中", "居中上", "中上", "顶部居中", "上方居中":
		return "tc"
	case "右上", "上右":
		return "tr"
	case "左中", "中左", "居中左", "左侧居中":
		return "ml"
	case "正中", "中心", "居中", "中央", "中间":
		return "mc"
	case "右中", "中右", "居中右", "右侧居中":
		return "mr"
	case "左下", "下左":
		return "bl"
	case "下中", "底中", "居中下", "中下", "底部居中", "下方居中":
		return "bc"
	case "右下", "下右":
		return "br"
	}
	// 2) 英文别名：去分隔符后匹配。
	k := strings.NewReplacer("-", "", "_", "", " ", "").Replace(key)
	switch k {
	case "tl", "topleft":
		return "tl"
	case "tc", "topcenter", "topcentre", "topmiddle", "centertop", "centretop":
		return "tc"
	case "tr", "topright":
		return "tr"
	case "ml", "middleleft", "centerleft", "centreleft", "left", "leftcenter", "leftmiddle":
		return "ml"
	case "mc", "middlecenter", "center", "centre", "middle", "centercenter", "centrecenter", "middlemiddle":
		return "mc"
	case "mr", "middleright", "centerright", "centreright", "right", "rightcenter", "rightmiddle":
		return "mr"
	case "bl", "bottomleft":
		return "bl"
	case "bc", "bottomcenter", "bottomcentre", "bottommiddle", "centerbottom", "centrebottom":
		return "bc"
	case "br", "bottomright":
		return "br"
	}
	// 3) 先水平后垂直的双字母写法，如 "lt"/"rc" → 交换成 t/m/b + l/c/r。
	if len(k) == 2 {
		a, b := string(k[0]), string(k[1])
		if (a == "l" || a == "c" || a == "r") && (b == "t" || b == "m" || b == "b") {
			return b + a
		}
	}
	return ""
}

// pctVal 把百分比值换算成像素。支持 0~100 与 0~1（<=1 视为比例）。
func pctVal(v, base float64) int {
	if v <= 1 && v >= 0 {
		v *= 100
	}
	n := int(v/100*base + 0.5)
	if n < 0 {
		return 0
	}
	if n > int(base) {
		return int(base)
	}
	return n
}

// ResolvePosition 把定点意图 + 原始像素坐标，换算成最终绝对像素。
// canvasW/H 为画布尺寸。返回的 W/H 在未被百分比指定时沿用原始值。
// anchor 语义：pos 只决定元素「框」的锚点落位；文字在框内的 l/m/r 对齐仍由 Anchor 负责。
func ResolvePosition(p PositionSpec, x, y, w, h, canvasW, canvasH int) Positioned {
	r := Positioned{X: x, Y: y, W: w, H: h}
	if canvasW <= 0 || canvasH <= 0 {
		return r
	}

	// 百分比尺寸优先。
	if p.HasPW {
		r.W = pctVal(p.PW, float64(canvasW))
	}
	if p.HasPH {
		r.H = pctVal(p.PH, float64(canvasH))
	}
	// 百分比坐标：把 x/y 直接设为百分比落点。
	if p.HasPX {
		r.X = pctVal(p.PX, float64(canvasW))
	}
	if p.HasPY {
		r.Y = pctVal(p.PY, float64(canvasH))
	}

	// 九宫格锚点：按锚点把元素框钉到画布对应位置，再加像素偏移。
	if a := normAnchor(p.Pos); a != "" {
		inset := p.Inset
		if inset <= 0 {
			short := canvasW
			if canvasH < short {
				short = canvasH
			}
			inset = short * 4 / 100
		}
		// 垂直分量。
		switch a[0] {
		case 't':
			r.Y = inset
		case 'm':
			r.Y = (canvasH - r.H) / 2
		case 'b':
			r.Y = canvasH - r.H - inset
		}
		// 水平分量。
		switch a[1] {
		case 'l':
			r.X = inset
		case 'c':
			r.X = (canvasW - r.W) / 2
		case 'r':
			r.X = canvasW - r.W - inset
		}
	}

	// 偏移（在锚点/百分比结果上叠加，允许负值做精细微调）。
	r.X += p.OX
	r.Y += p.OY
	if r.X < 0 {
		r.X = 0
	}
	if r.Y < 0 {
		r.Y = 0
	}
	return r
}
