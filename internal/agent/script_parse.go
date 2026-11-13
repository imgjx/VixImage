package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// 本文件是「字段值容错」层（解析层），不是操作兜底。
// 小模型输出的字段类型/写法极不稳定：颜色写 RGB 数组、字号写数字字符串、
// 工具名直接作 key、参数包在 params 里……这里统一把它们归一化到同一批字段。
// 绝不猜测或补全操作：工具名 / 图层种类是否合法仍由 validateTurn 严格判定，
// 未知工具、链底违规、缺字段一律报错——无兜底。

// decMap 把 JSON 对象解成 map；非对象返回 nil。
func decMap(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

// rawVal 取出一个 JSON 值；不存在 / null 视为无值。
func rawVal(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, false
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil, false
	}
	return v, true
}

// firstVal 按候选键顺序取第一个有值的字段（兼容字段别名）。
func firstVal(m map[string]json.RawMessage, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := rawVal(m[k]); ok {
			return v, true
		}
	}
	return nil, false
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return ""
}

// toColor 兼容 hex 字符串 / "ffffff" / [r,g,b] / [r,g,b,a] 数字数组。
func toColor(v any) string { return normalizeColor(v) }

func toBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		s := strings.TrimSpace(strings.ToLower(x))
		if b, e := strconv.ParseBool(s); e == nil {
			return b
		}
		return s != "" && s != "0" && s != "no" && s != "off" && s != "null" && s != "false"
	}
	return false
}

func toStrSlice(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s := toStr(e); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{x}
	}
	return nil
}

func toAnySlice(v any) []any {
	if x, ok := v.([]any); ok {
		return x
	}
	return []any{v}
}

// fillTool 从参数对象里宽松填充 Tool 的字段。
func fillTool(t *Tool, m map[string]json.RawMessage) {
	g := func(keys ...string) (any, bool) { return firstVal(m, keys...) }
	if v, ok := g("w", "width"); ok {
		t.W = int(numToFloat(v))
	}
	if v, ok := g("h", "height"); ok {
		t.H = int(numToFloat(v))
	}
	if v, ok := g("bg", "background"); ok {
		t.BG = toAnySlice(v)
	}
	if v, ok := g("angle"); ok {
		t.Angle = v
	}
	if v, ok := g("solid"); ok {
		t.Solid = v
	}
	if v, ok := g("id"); ok {
		t.ID = toStr(v)
	}
	if v, ok := g("p", "prompt"); ok {
		t.P = toStr(v)
	}
	if v, ok := g("size"); ok {
		t.Size = toStr(v)
	}
	if v, ok := g("engine"); ok {
		t.Engine = toStr(v)
	}
	if v, ok := g("text", "content", "data", "value"); ok {
		t.Text = toStr(v)
	}
	if v, ok := g("fg", "front", "fore", "fg_color"); ok {
		t.FG = toColor(v)
	}
	if v, ok := g("bg_color", "bg", "back", "back_color"); ok {
		if s := toColor(v); s != "" {
			t.Bg = s
		}
	}
	if v, ok := g("ref"); ok {
		// 空字符串不视为引用；null 已在 rawVal 层排除。
		// 整数 → 输入图序号（图生图 ref:N）；字符串 → 产物 id / "ref:N" / 网址，
		// 让 LLM 能把"之前生成的图 / 复制的中间产物 / 任意图片"再做图生图迭代。
		if s, isStr := v.(string); isStr {
			if strings.TrimSpace(s) != "" {
				t.RefStr = strings.TrimSpace(s)
			}
		} else {
			n := int(numToFloat(v))
			t.Ref = &n
		}
	}
	if v, ok := g("i", "index"); ok {
		t.I = int(numToFloat(v))
	}
	if v, ok := g("n", "steps_back", "back"); ok {
		t.N = int(numToFloat(v))
	}
	if v, ok := g("as", "to", "new_id", "dest", "copy_to"); ok {
		t.As = toStr(v)
	}
	if v, ok := g("srcs"); ok {
		t.Srcs = toStrSlice(v)
	}
	if v, ok := g("src", "from", "source"); ok {
		t.Src = toStr(v)
	}
	if raw, ok := m["layer"]; ok {
		var l LayerSpec
		if json.Unmarshal(raw, &l) == nil {
			t.Layer = &l
			t.Layers = []LayerSpec{l}
		} else if json.Unmarshal(raw, &t.Layers) == nil && len(t.Layers) > 0 {
			// 模型把 layer 写成数组：一个 add 一次追加多个图层，仍逐个严格校验。
			first := t.Layers[0]
			t.Layer = &first
		}
	}
}

// LayerSpec.UnmarshalJSON 让图层各字段同样享受字段值容错
// （颜色数组、数字字符串、布尔字符串等），不改变字段类型与严格校验。
func (l *LayerSpec) UnmarshalJSON(data []byte) error {
	m := decMap(data)
	if m == nil {
		return fmt.Errorf("图层不是 JSON 对象")
	}
	fillLayer(l, m)
	return nil
}

// fillLayer 从参数对象里宽松填充 LayerSpec 的字段。
func fillLayer(l *LayerSpec, m map[string]json.RawMessage) {
	g := func(keys ...string) (any, bool) { return firstVal(m, keys...) }
	if v, ok := g("k", "kind", "type"); ok {
		l.K = toStr(v)
	}
	if v, ok := g("src"); ok {
		l.Src = toStr(v)
	}
	if v, ok := g("x"); ok {
		l.X = int(numToFloat(v))
	}
	if v, ok := g("y"); ok {
		l.Y = int(numToFloat(v))
	}
	if v, ok := g("w"); ok {
		l.W = int(numToFloat(v))
	}
	if v, ok := g("h"); ok {
		l.H = int(numToFloat(v))
	}
	if v, ok := g("fit"); ok {
		l.Fit = toStr(v)
	}
	if v, ok := g("ov"); ok {
		l.Ov = numToFloat(v)
	}
	if v, ok := g("ov_col"); ok {
		l.OvCol = toColor(v)
	}
	if v, ok := g("blur"); ok {
		l.Blur = int(numToFloat(v))
	}
	if v, ok := g("rot"); ok {
		l.Rot = numToFloat(v)
	}
	if v, ok := g("persp"); ok {
		l.Persp = numToFloat(v)
	}
	if v, ok := g("shear"); ok {
		l.Shear = numToFloat(v)
	}
	if v, ok := g("cut"); ok {
		l.Cut = toBool(v)
	}
	if v, ok := g("cut_tol"); ok {
		l.CutTol = numToFloat(v)
	}
	if v, ok := g("circle"); ok {
		l.Circle = toBool(v)
	}
	if v, ok := g("radius"); ok {
		l.Radius = int(numToFloat(v))
	}
	if v, ok := g("opacity"); ok {
		l.Opacity = numToFloat(v)
	}
	if v, ok := g("shadow"); ok {
		l.Shadow = toBool(v)
	}
	if v, ok := g("sh_blur"); ok {
		l.ShBlur = int(numToFloat(v))
	}
	if v, ok := g("sh_dx"); ok {
		l.ShDX = int(numToFloat(v))
	}
	if v, ok := g("sh_dy"); ok {
		l.ShDY = int(numToFloat(v))
	}
	if v, ok := g("border"); ok {
		l.Border = toColor(v)
	}
	if v, ok := g("border_w"); ok {
		l.BorderW = int(numToFloat(v))
	}
	if v, ok := g("s", "text"); ok {
		l.S = toStr(v)
	}
	if v, ok := g("size", "font_size"); ok {
		l.Size = int(numToFloat(v))
	}
	if v, ok := g("color"); ok {
		l.Color = toColor(v)
	}
	if v, ok := g("bold"); ok {
		l.Bold = toBool(v)
	}
	if v, ok := g("italic"); ok {
		l.Italic = toBool(v)
	}
	if v, ok := g("anchor"); ok {
		l.Anchor = toStr(v)
	}
	if v, ok := g("glow"); ok {
		l.Glow = toColor(v)
	}
	if v, ok := g("glow_sz"); ok {
		l.GlowSz = int(numToFloat(v))
	}
	if v, ok := g("stroke"); ok {
		l.Stroke = toColor(v)
	}
	if v, ok := g("stroke_w"); ok {
		l.StrokeW = int(numToFloat(v))
	}
	if v, ok := g("spacing"); ok {
		l.Spacing = int(numToFloat(v))
	}
	if v, ok := g("line_h"); ok {
		l.LineH = numToFloat(v)
	}
	if v, ok := g("mono"); ok {
		l.Mono = toBool(v)
	}
	if v, ok := g("alpha"); ok {
		l.Alpha = numToFloat(v)
	}
	if v, ok := g("fill"); ok {
		l.Fill = toBool(v)
	}
	if v, ok := g("x1"); ok {
		l.X1 = int(numToFloat(v))
	}
	if v, ok := g("y1"); ok {
		l.Y1 = int(numToFloat(v))
	}
	if v, ok := g("x2"); ok {
		l.X2 = int(numToFloat(v))
	}
	if v, ok := g("y2"); ok {
		l.Y2 = int(numToFloat(v))
	}
	if v, ok := g("line_w", "line_width", "width"); ok {
		l.Width = int(numToFloat(v))
	}
	if v, ok := g("dir"); ok {
		l.Dir = toStr(v)
	}
	if v, ok := g("strength"); ok {
		l.Strength = numToFloat(v)
	}
	// circle 常用 r 表示半径；换算成 rect 的 w/h，渲染时再取圆角画圆。
	if strings.EqualFold(l.K, "circle") && l.W == 0 {
		if v, ok := g("r"); ok {
			if radius := int(numToFloat(v)); radius > 0 {
				l.W, l.H = 2*radius, 2*radius
			}
		}
	}
}

// normalizeTailView 收敛一种明确且高频的写法：模型想在「交付那一轮」先 view 再 finish，
// 写成 [...view, finish]。view 是只读、不改画面，而系统级强制 view 已保证交付前必回看，
// 故此处把紧跟在末尾 finish 之前的 view 移除，等价于「系统接管这次查看」。
//
// 严格有界：仅当最后一个工具是 finish、且它前面的链底工具是 view 时才收敛；
// 其它链底违规（finish 不在最后、多个 finish、view 夹在编辑动作中间等）一律不动，
// 仍由 validateTurn 报错——无兜底、不猜测操作、不改任何编辑动作。
func normalizeTailView(t *Turn) {
	if t == nil || len(t.Tools) < 2 {
		return
	}
	if normTool(t.Tools[len(t.Tools)-1].T) != ToolFinish {
		return
	}
	// 从倒数第二个往前，去掉连续的 view（只动 view，遇到别的立即停）。
	cut := len(t.Tools) - 1
	for cut > 0 && normTool(t.Tools[cut-1].T) == ToolView {
		cut--
	}
	if cut == len(t.Tools)-1 {
		return // finish 前面不是 view，不做任何改动
	}
	t.Tools = append(t.Tools[:cut], t.Tools[len(t.Tools)-1])
}

// stripTrailingCommas 清掉对象/数组里的尾随逗号（小模型常见语法瑕疵）。
// 只在字符串外处理，绝不改动字符串内容；属于 JSON 语法容错，不涉及操作判定。
func stripTrailingCommas(s string) string {
	r := []rune(s)
	var b strings.Builder
	inStr, esc := false, false
	for i := 0; i < len(r); i++ {
		ch := r[i]
		if inStr {
			b.WriteRune(ch)
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		if ch == '"' {
			inStr = true
			b.WriteRune(ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(r) && (r[j] == ' ' || r[j] == '\n' || r[j] == '\t' || r[j] == '\r') {
				j++
			}
			if j < len(r) && (r[j] == '}' || r[j] == ']') {
				continue
			}
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// turnKeys 是合并碎片对象时允许保留的字段（只认这些，避免把无关对象混进来）。
var turnKeys = map[string]bool{
	"think": true, "thought": true, "reasoning": true,
	"tools": true, "actions": true, "calls": true, "steps": true,
}

// mergeTurnObjects 处理小模型把 think 与 tools 拆成多个并列 JSON 对象的情况。
// 只把白名单字段合并成一个对象（先出现者优先），不改变任何工具内容；
// 单对象时返回空串，交给 extractJSON 常规处理。属于输出格式归一，非操作兜底。
func mergeTurnObjects(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
		if k := strings.IndexByte(s, '{'); k > 0 && strings.HasPrefix(strings.TrimSpace(s[:k]), "json") {
			s = s[k:]
		}
	}
	var objs []string
	depth, start, inStr, esc := 0, -1, false, false
	for i, ch := range s {
		if inStr {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				objs = append(objs, s[start:i+1])
				start = -1
			}
		}
	}
	if len(objs) < 2 {
		return ""
	}
	merged := map[string]json.RawMessage{}
	for _, o := range objs {
		var m map[string]json.RawMessage
		if json.Unmarshal([]byte(o), &m) != nil {
			continue
		}
		for k, v := range m {
			if turnKeys[k] {
				if _, exists := merged[k]; !exists {
					merged[k] = v
				}
			}
		}
	}
	hasTools := false
	for _, k := range []string{"tools", "actions", "calls", "steps"} {
		if _, ok := merged[k]; ok {
			hasTools = true
			break
		}
	}
	if !hasTools {
		return ""
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return ""
	}
	return string(b)
}
