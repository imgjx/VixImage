package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"viximage/internal/agent/skills"
)

// 步数模型（定稿）：
//   - 每次大模型请求（一轮自循环）= 1 步；
//   - 每次调用图像模型生图 = 该引擎 ImageModelUse.Steps 步（默认 30）；
//   - 编辑/查看/完成等工具不单独计步。
//
// 总步数预算由「我的模型」max_steps 控制（默认 100）。
const (
	StepPerLLMCall   = 1
	DefaultMaxSteps  = 100
	MaxTextRunes     = 240 // 单个 text 图层文本上限（防"满屏重复字符"注入）
	MaxUndoSteps     = 24  // undo 单次回滚步数上限
	MaxLayerCount    = 80  // 画布图层总数上限
	MaxQRText        = 800 // 单个二维码内容字符上限
	MaxOutputPerView = 6   // 单次回灌给大模型的图片数上限
	DefaultGenCost   = 30  // 引擎未配置时的默认生图消耗步数
	MaxForceReviews  = 3   // 交付前「系统级强制 view」最多回看次数（防小模型反复改动耗光步数）
)

// ---------------------------------------------------------------------------
// 一轮大模型输出：短 ThinkFlag + 一批工具调用
// ---------------------------------------------------------------------------

// Turn 是一次大模型请求的输出。think 是短思考标记（ThinkFlag），
// tools 是这一轮要顺序执行的一批图像工具调用（一次可含多个 skill use）。
type Turn struct {
	Think string `json:"think"`
	Tools []Tool `json:"tools"`
}

// Tool 是单个工具调用。用紧凑字段 t 区分类型，其余字段按需填，
// 让小上下文模型也能稳定输出。t 取值：
//
//	canvas 建立/重设画布         w,h,bg,angle,solid
//	gen    生成一张图片(有产物)   id,p,size,engine,ref
//	qrcode 生成二维码(有产物)      id,text,size,fg,bg
//	qrdecode 解析二维码内容        src
//	add    追加一个图层           layer
//	set    替换第 i 个图层        i,layer
//	del    删除第 i 个图层        i
//	clear  清空所有图层           —
//	copy   复制产物为新产物       src,id   ← 存档/分叉/复用某中间产物
//	undo   回滚最近 n 轮编辑      n(默认1)  ← 编辑失误时撤销重来
//	layers 查看当前图层清单       —        ← 拿不准第几层时先查
//	view   查看图片(有产物·链底)   srcs
//	finish 完成并标注产物(链底)    src
//
// UnmarshalJSON 兼容小模型常用的字段别名（tool/type/name/op 等），
// 属于解析容错；不改变「解析失败/非法工具一律报错」的无兜底原则。
type Tool struct {
	T string `json:"t"`

	// canvas —— BG/Solid 用宽松类型接住小模型的多种写法（hex 字符串 / RGBA 数字数组 / 布尔），
	// 执行时再归一化；避免"颜色值写法不同"导致整轮 JSON 解析失败。
	W     int   `json:"w"`
	H     int   `json:"h"`
	BG    []any `json:"bg"`
	Angle any   `json:"angle"`
	Solid any   `json:"solid"`

	// gen
	ID     string `json:"id"`
	P      string `json:"p"`
	Size   string `json:"size"`
	Engine string `json:"engine"`
	Ref    *int   `json:"ref"` // 整数 → 输入图序号（ref:0 图生图）
	RefStr string `json:"-"`   // 字符串 → 产物 id / "ref:N" / 网址：从已有产物做图生图（LLM 指定参考）

	// qrcode —— Text 是要编码的内容；FG/Bg 可选前景/背景色（解析层处理 bg 别名）。
	Text string `json:"text"`
	FG   string `json:"fg"`
	Bg   string `json:"-"`

	// add / set / del —— Layers 由解析层在 layer 写成数组时展开（一轮可一次加多个图层）。
	Layer  *LayerSpec  `json:"layer"`
	Layers []LayerSpec `json:"-"`
	I      int         `json:"i"`

	// undo —— 回退步数（默认 1）。
	N int `json:"n"`

	// copy —— 源产物(src)复制成新产物(as)。
	As string `json:"as"`

	// view / finish
	Srcs []string `json:"srcs"`
	Src  string   `json:"src"`
}

// 工具类型常量。
const (
	ToolCanvas = "canvas"
	ToolGen    = "gen"
	ToolQrcode   = "qrcode"
	ToolQrdecode = "qrdecode"
	ToolAdd    = "add"
	ToolSet    = "set"
	ToolDel    = "del"
	ToolClear  = "clear"
	ToolCopy   = "copy"
	ToolUndo   = "undo"
	ToolLayers = "layers"
	ToolView   = "view"
	ToolFinish = "finish"
)

// Tool.UnmarshalJSON 把一轮工具调用宽松地解析成 Tool。
// 小模型写法极不稳定（颜色写 RGB 数组、字号写数字、工具名作 key、参数包在 params 里），
// 这里只做「字段值容错」：把不同写法归一化到同一批字段，绝不猜测或补全操作。
// 工具名/图层种类是否合法仍由 validateTurn 严格判定，未知一律报错——无兜底。
// 字段取值细节见 script_parse.go。
func (t *Tool) UnmarshalJSON(data []byte) error {
	obj := decMap(data)
	if obj == nil {
		return fmt.Errorf("工具项不是 JSON 对象")
	}
	// 1) 工具名：t 或常用字符串别名
	name := ""
	for _, k := range []string{"t", "tool", "type", "name", "op", "action", "skill"} {
		if v, ok := rawVal(obj[k]); ok {
			if s := toStr(v); strings.TrimSpace(s) != "" {
				name = s
				break
			}
		}
	}
	// 2) 参数对象：默认整项；若用 params/args/arguments/input 包裹则取包裹对象
	params := obj
	for _, k := range []string{"params", "args", "arguments", "input"} {
		if m := decMap(obj[k]); m != nil {
			params = m
			break
		}
	}
	// 3) 工具名直接作 key 的包装写法 {"canvas":{...}} / {"clear":true}
	if name == "" {
		for k, raw := range obj {
			if k2 := normTool(k); k2 != "" {
				name = k2
				if m := decMap(raw); m != nil {
					params = m
				} else {
					params = map[string]json.RawMessage{}
				}
				break
			}
		}
	}
	t.T = name
	fillTool(t, params)
	return nil
}

// UnmarshalJSON 兼容 tools 的别名（actions/calls/steps）与 think 的别名。
func (tn *Turn) UnmarshalJSON(data []byte) error {
	type alias Turn
	var a struct {
		alias
		Actions json.RawMessage `json:"actions"`
		Calls   json.RawMessage `json:"calls"`
		Steps   json.RawMessage `json:"steps"`
		Thought string          `json:"thought"`
		Reason  string          `json:"reasoning"`
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*tn = Turn(a.alias)
	if tn.Think == "" {
		tn.Think = a.Thought
		if tn.Think == "" {
			tn.Think = a.Reason
		}
	}
	if len(tn.Tools) == 0 {
		for _, raw := range []json.RawMessage{a.Actions, a.Calls, a.Steps} {
			if len(raw) == 0 {
				continue
			}
			var ts []Tool
			if json.Unmarshal(raw, &ts) == nil && len(ts) > 0 {
				tn.Tools = ts
				break
			}
		}
	}
	return nil
}

// isBottomTool 报告该工具是否为「链底专用」（只能出现在 tools 最后一个）。
func isBottomTool(t string) bool { return t == ToolView || t == ToolFinish }

// hasImageOutput 报告该工具是否会产生「可回灌给大模型的图像输出」。
// 定稿规则：只有 GenImage 图像生成 与 图像查看(view) 会输出结果图，
// 其余编辑类工具（含 copy/undo/layers）只回「是否成功编辑」的状态文本。
func hasImageOutput(t string) bool { return t == ToolGen || t == ToolView }

// copyDest 取 copy 的新产物 id：优先 as，其次沿用 gen 习惯的 id。
func copyDest(t Tool) string {
	if s := strings.TrimSpace(t.As); s != "" {
		return s
	}
	return strings.TrimSpace(t.ID)
}

// isEditingTool 报告该工具是否会改动画布/图层/产物表（需要在执行前压回滚点）。
// view/finish 只读或收尾，layers/undo 不改状态，均不算编辑。
func isEditingTool(t string) bool {
	switch t {
	case ToolCanvas, ToolGen, ToolAdd, ToolSet, ToolDel, ToolClear, ToolCopy, ToolQrcode:
		return true
	}
	return false
}

// mutatesCanvas 报告该工具是否改动画布/图层——这类动作执行前压回滚点。
// gen 只加产物、copy 只加产物，都不动排版，故不压（undo 也无需撤销它们）。
func mutatesCanvas(t string) bool {
	switch t {
	case ToolCanvas, ToolAdd, ToolSet, ToolDel, ToolClear:
		return true
	}
	return false
}

// hasUndo 报告一轮里是否含 undo（纯回滚轮不压新回滚点）。
func hasUndo(t *Turn) bool {
	for _, tl := range t.Tools {
		if normTool(tl.T) == ToolUndo {
			return true
		}
	}
	return false
}

// validateTurn 严格校验一轮输出。任何不合法都返回错误——无兜底、不补全、不猜测。
// 链底规则：view/finish 必须在 tools 的最后一个，且整轮最多出现一个。
func validateTurn(t *Turn, nInputs int) error {
	if len(t.Tools) == 0 {
		return fmt.Errorf("本轮 tools 为空：没有可执行的调用")
	}
	bottomSeen := false
	genIDs := map[string]bool{}
	for i, tool := range t.Tools {
		k := normTool(tool.T)
		if k == "" {
			return fmt.Errorf("tools[%d] 未知工具 %q（合法：canvas/gen/qrcode/qrdecode/add/set/del/clear/copy/undo/layers/view/finish）", i, tool.T)
		}
		if isBottomTool(k) {
			if bottomSeen {
				return fmt.Errorf("tools[%d] 链底工具 %q 重复：view/finish 每轮最多一个", i, k)
			}
			if i != len(t.Tools)-1 {
				return fmt.Errorf("链底工具 %q 必须放在 tools 最后一个（当前在 %d/%d），否则报错", k, i+1, len(t.Tools))
			}
			bottomSeen = true
		}
		switch k {
		case ToolCanvas:
			if tool.W < 200 || tool.W > 4096 || tool.H < 200 || tool.H > 4096 {
				return fmt.Errorf("tools[%d] canvas 宽高非法（200~4096）：%dx%d", i, tool.W, tool.H)
			}
		case ToolGen:
			if strings.TrimSpace(tool.ID) == "" {
				return fmt.Errorf("tools[%d] gen 缺少 id", i)
			}
			if genIDs[tool.ID] {
				return fmt.Errorf("tools[%d] gen.id %q 本轮重复", i, tool.ID)
			}
			genIDs[tool.ID] = true
			if strings.TrimSpace(tool.P) == "" {
				return fmt.Errorf("tools[%d] gen 缺少提示词 p", i)
			}
			if tool.Ref != nil && (*tool.Ref < 0 || *tool.Ref >= nInputs) {
				return fmt.Errorf("tools[%d] gen.ref=%d 越界（输入图共 %d 张）", i, *tool.Ref, nInputs)
			}
		case ToolQrcode:
			if strings.TrimSpace(tool.ID) == "" {
				return fmt.Errorf("tools[%d] qrcode 缺少 id", i)
			}
			if strings.TrimSpace(tool.Text) == "" {
				return fmt.Errorf("tools[%d] qrcode 缺少 text（要编码的内容）", i)
			}
			if len([]rune(tool.Text)) > MaxQRText {
				return fmt.Errorf("tools[%d] qrcode 内容长度 %d 超过上限 %d", i, len([]rune(tool.Text)), MaxQRText)
			}
		case ToolQrdecode:
			if strings.TrimSpace(tool.Src) == "" {
				return fmt.Errorf("tools[%d] qrdecode 缺少 src（产物 id / canvas / 图片网址）", i)
			}
		case ToolAdd, ToolSet:
			if tool.Layer == nil {
				return fmt.Errorf("tools[%d] %s 缺少 layer", i, k)
			}
			layers := tool.Layers
			if len(layers) == 0 {
				layers = []LayerSpec{*tool.Layer}
			}
			for j := range layers {
				lj := &layers[j]
				if err := validateLayer(lj, i); err != nil {
					return err
				}
				// 引用不存在的输入图在决策阶段就报错（无兜底），不必等渲染。
				if normKind(lj.K) == "image" {
					low := strings.ToLower(strings.TrimSpace(lj.Src))
					if strings.HasPrefix(low, "ref:") || strings.HasPrefix(low, "input:") {
						var n int
						idx := lj.Src[strings.IndexByte(lj.Src, ':')+1:]
						if _, e := fmt.Sscanf(idx, "%d", &n); e != nil || n < 0 || n >= nInputs {
							return fmt.Errorf("tools[%d] 图层[%d] src %q 越界（本次输入图共 %d 张，禁止引用不存在的参考图）", i, j, lj.Src, nInputs)
						}
					}
				}
			}
		case ToolDel:
			if tool.I < 0 {
				return fmt.Errorf("tools[%d] del 索引 i=%d 非法", i, tool.I)
			}
		case ToolCopy:
			if strings.TrimSpace(tool.Src) == "" {
				return fmt.Errorf("tools[%d] copy 缺少 src（源产物 id 或 canvas）", i)
			}
			if strings.TrimSpace(copyDest(tool)) == "" {
				return fmt.Errorf("tools[%d] copy 缺少 as（新产物 id）", i)
			}
		case ToolUndo:
			// n<=0 归一为 1；负得离谱/爆量直接拒绝（防爆量、防注入）。
			if tool.N > MaxUndoSteps {
				return fmt.Errorf("tools[%d] undo 步数 n=%d 过大（上限 %d）", i, tool.N, MaxUndoSteps)
			}
		case ToolLayers:
			// 无参数：返回当前图层清单文本供下一轮决策。
		case ToolView:
			if len(tool.Srcs) == 0 {
				return fmt.Errorf("tools[%d] view 缺少 srcs", i)
			}
			if len(tool.Srcs) > MaxOutputPerView {
				return fmt.Errorf("tools[%d] view 一次最多查看 %d 张图", i, MaxOutputPerView)
			}
		case ToolFinish:
			// src 可为空（默认取 canvas）；具体引用在执行时解析。
		}
	}
	return nil
}

// validateLayer 校验单个图层，重点是文本长度（防注入）与图层种类。
func validateLayer(l *LayerSpec, idx int) error {
	k := normKind(l.K)
	if k == "" {
		return fmt.Errorf("tools[%d] 未知图层 kind %q", idx, l.K)
	}
	if k == "text" && len([]rune(l.S)) > MaxTextRunes {
		return fmt.Errorf("tools[%d] 文本长度 %d 超过上限 %d（拒绝大量重复/无意义字符）",
			idx, len([]rune(l.S)), MaxTextRunes)
	}
	if k == "image" && strings.TrimSpace(l.Src) == "" {
		return fmt.Errorf("tools[%d] img 图层缺少 src", idx)
	}
	return nil
}

func normTool(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case ToolCanvas, "setcanvas", "newcanvas":
		return ToolCanvas
	case ToolGen, "genimage", "generate":
		return ToolGen
	case ToolQrcode, "qr", "二维码", "生成二维码":
		return ToolQrcode
	case ToolQrdecode, "qr_scan", "qrdecode_image", "解析二维码", "扫码":
		return ToolQrdecode
	case ToolAdd, "addlayer":
		return ToolAdd
	case ToolSet, "setlayer":
		return ToolSet
	case ToolDel, "dellayer":
		return ToolDel
	case ToolClear, "clearlayers":
		return ToolClear
	case ToolCopy, "clone", "duplicate", "fork", "复制", "另存":
		return ToolCopy
	case ToolUndo, "rollback", "revert", "撤销", "回退":
		return ToolUndo
	case ToolLayers, "listlayers", "inspect", "图层":
		return ToolLayers
	case ToolView, "preview", "look", "see":
		return ToolView
	case ToolFinish, "done", "complete":
		return ToolFinish
	}
	return ""
}

// canvasSpec 把一轮 canvas 工具的宽松字段归一化成 CanvasSpec。
// bg 兼容三种写法：颜色串列表(渐变) / 嵌套 RGBA 数组列表(渐变) / 扁平 RGBA 数组(单色，小模型常见)；
// solid 兼容字符串颜色 / 布尔(true 时沿用 bg 首色，无 bg 才用默认深底)。
// 颜色项解析不了就丢弃，但不会让整轮 JSON 崩——这是字段值容错，不是「操作无法解析」的兜底。
func canvasSpec(t Tool) *CanvasSpec {
	c := &CanvasSpec{W: t.W, H: t.H}
	c.Angle = numToFloat(t.Angle)

	// bg —— 情况 A：整体是扁平数字数组 [r,g,b] / [r,g,b,a]，当成单个颜色。
	flat := len(t.BG) >= 3
	for _, v := range t.BG {
		if !isNum(v) {
			flat = false
			break
		}
	}
	if flat {
		nums := make([]int, 0, len(t.BG))
		for _, v := range t.BG {
			nums = append(nums, int(numToFloat(v)))
		}
		if col := rgbHex(nums); col != "" {
			c.BG = append(c.BG, col)
		}
	} else {
		// 情况 B：逐项解析（颜色串 / 嵌套 RGBA 数组）。
		for _, v := range t.BG {
			if col := normalizeColor(v); col != "" {
				c.BG = append(c.BG, col)
			}
		}
	}

	// solid
	if s, ok := t.Solid.(string); ok && strings.TrimSpace(s) != "" {
		c.Solid = s
	} else if b, ok := t.Solid.(bool); ok && b {
		if len(c.BG) > 0 {
			c.Solid = c.BG[0] // 尊重模型给的颜色值
		} else {
			c.Solid = "#0a0e1a"
		}
	}
	return c
}

// isNum 报告该值是否为数值（json 解到 []any 时数字默认是 float64，单测里可能是 int 等）。
func isNum(v any) bool {
	switch v.(type) {
	case float64, float32, int, int32, int64, json.Number:
		return true
	}
	return false
}

// normalizeColor 把单个颜色项转成 "#rrggbb"。
func normalizeColor(v any) string {
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return ""
		}
		if !strings.HasPrefix(s, "#") {
			s = "#" + s
		}
		return s
	case []any: // [r,g,b] / [r,g,b,a]
		nums := make([]int, 0, len(x))
		for _, e := range x {
			nums = append(nums, int(numToFloat(e)))
		}
		return rgbHex(nums)
	}
	return ""
}

func rgbHex(n []int) string {
	if len(n) < 3 {
		return ""
	}
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return v
	}
	return fmt.Sprintf("#%02x%02x%02x", clamp(n[0]), clamp(n[1]), clamp(n[2]))
}

func numToFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		var f float64
		_, _ = fmt.Sscanf(x, "%f", &f)
		return f
	}
	return 0
}

// ---------------------------------------------------------------------------
// 内存渲染状态（中间件）：画布 + 图层 + 产物表
// ---------------------------------------------------------------------------

// assetEntry 是产物表里的一条（可被图层 src 或 view.srcs 引用，也可被 copy 复制）。
type assetEntry struct {
	Path string // 本地文件路径
	Desc string // 一句话说明（紧凑产物表用）
	Kind string // "ref" 用户参考图 | "gen" 生成图 | "edit" 中间产物
	Size string // 尺寸串（可选）
}

// stateSnap 是一次「编辑前」的内存状态快照，供 undo 回滚。
// 只存画布/图层的深拷贝，不渲染图片文件——回滚成本极低，
// 刻意不做成「每步生成中间图片产物」那套重机制。
//
// 分工：undo 只撤销「排版编辑」（画布 + 图层）；产物表(assets)一旦生成/复制就常驻，
// 由 copy 工具负责版本存档与分叉。这样 undo 不会把已花步数生成的素材图弄丢，
// 也符合「编辑错了回滚一下」的直觉——只退排版，不退素材。
type stateSnap struct {
	canvas *CanvasSpec
	layers []LayerSpec
}

// renderState 是自循环全程持有的状态。编辑类工具只改这里，不落盘、不回图；
// 只有 view/finish 才把画布渲染成 PNG（"看自己的作品"/交付）。
type renderState struct {
	canvas  *CanvasSpec
	layers  []LayerSpec
	assets  map[string]*assetEntry // id -> 产物
	order   []string               // 产物表稳定顺序
	history []stateSnap            // 每次编辑前的回滚点（undo 用）
}

func newRenderState() *renderState {
	return &renderState{assets: map[string]*assetEntry{}}
}

func (s *renderState) addAsset(id string, e *assetEntry) error {
	if _, exists := s.assets[id]; exists {
		// Agent 反复观摩后重做同名产物 = 覆盖更新，不报错、不重复进产物表。
		// 这不是「兜底」：工具本身仍是合法的 gen，只是产物版本被替换。
		s.assets[id] = e
		return nil
	}
	s.assets[id] = e
	s.order = append(s.order, id)
	return nil
}

// cloneSnap 深拷贝当前画布/图层为一个回滚点。
func (s *renderState) cloneSnap() stateSnap {
	snap := stateSnap{layers: append([]LayerSpec(nil), s.layers...)}
	if s.canvas != nil {
		c := *s.canvas
		c.BG = append([]string(nil), s.canvas.BG...)
		snap.canvas = &c
	}
	return snap
}

// pushHistory 记录一次「编辑动作前」的画布/图层状态。
// 尚未建立画布时不记录回滚点：保证 undo 永远不会退回到「画布未建立」这种不可用状态
// （这是快照记录策略，不是对非法操作的兜底）。
func (s *renderState) pushHistory() {
	if s.canvas == nil {
		return
	}
	s.history = append(s.history, s.cloneSnap())
}

// undoHistory 回滚 n 个编辑动作（n<=0 归一为 1）。快照不足则报错——无兜底。
// 只恢复画布/图层，产物表保持不变（素材常驻）。返回实际回滚的步数。
func (s *renderState) undoHistory(n int) (int, error) {
	if n <= 0 {
		n = 1
	}
	if len(s.history) == 0 {
		return 0, fmt.Errorf("没有可回滚的编辑历史（history 为空）")
	}
	if n > len(s.history) {
		n = len(s.history) // 最多退到最初状态
	}
	var snap stateSnap
	for i := 0; i < n; i++ {
		snap = s.history[len(s.history)-1]
		s.history = s.history[:len(s.history)-1]
	}
	s.canvas = nil
	if snap.canvas != nil {
		c := *snap.canvas
		c.BG = append([]string(nil), snap.canvas.BG...)
		s.canvas = &c
	}
	s.layers = append([]LayerSpec(nil), snap.layers...)
	return n, nil
}

// layerTable 生成当前图层清单文本（layers 工具用，帮助 Agent 定位 set/del 索引）。
func (s *renderState) layerTable() string {
	if len(s.layers) == 0 {
		return "（当前无图层）"
	}
	var b strings.Builder
	for i, l := range s.layers {
		hint := ""
		switch normKind(l.K) {
		case "text":
			hint = truncateStr(strings.ReplaceAll(l.S, "\n", "/"), 24)
		case "image":
			hint = "src=" + truncateStr(l.Src, 24)
		}
		fmt.Fprintf(&b, " #%d %s @%d,%d %dx%d %s\n", i, normKind(l.K), l.X, l.Y, l.W, l.H, hint)
	}
	return strings.TrimRight(b.String(), "\n")
}

// assetTable 生成紧凑产物表文本（"中间件的那个表"）。明确标注用户参考图 / 生成图 / 中间产物。
func (s *renderState) assetTable() string {
	if len(s.order) == 0 {
		return "（暂无产物）"
	}
	var b strings.Builder
	for _, id := range s.order {
		e := s.assets[id]
		kind := e.Kind
		switch kind {
		case "ref":
			kind = "用户参考图"
		case "gen":
			kind = "生成图"
		case "edit":
			kind = "中间产物"
		}
		sz := ""
		if e.Size != "" {
			sz = " " + e.Size
		}
		fmt.Fprintf(&b, "- %s [%s%s] %s\n", id, kind, sz, truncateStr(e.Desc, 48))
	}
	return strings.TrimRight(b.String(), "\n")
}

// BuildSpec 把当前画布 + 图层翻译成底层通用渲染规格。
// resolve 把逻辑 src（asset id / ref:N / url）解析成本地路径；失败即错误（无兜底）。
func (s *renderState) BuildSpec(resolve func(string) (string, error), output string) (*skills.Spec, error) {
	if s.canvas == nil {
		return nil, fmt.Errorf("尚未用 canvas 工具建立画布，无法渲染")
	}
	spec := &skills.Spec{Width: s.canvas.W, Height: s.canvas.H, Output: output}
	if s.canvas.Solid != "" {
		spec.BG = &skills.BGSpec{Type: "solid", Color: s.canvas.Solid}
	} else {
		cols := s.canvas.BG
		if len(cols) < 2 {
			cols = []string{"#0a0e1a", "#101a30"}
		}
		spec.BG = &skills.BGSpec{Type: "gradient", Colors: cols, Angle: s.canvas.Angle}
	}
	for i, l := range s.layers {
		item, err := l.toItem(resolve)
		if err != nil {
			return nil, fmt.Errorf("图层[%d] 执行失败: %w", i, err)
		}
		spec.Items = append(spec.Items, item)
	}
	return spec, nil
}

// ---------------------------------------------------------------------------
// 画布 / 图层规格（沿用紧凑字段，适配小上下文模型）
// ---------------------------------------------------------------------------

// CanvasSpec 描述画布。w/h 由大模型按需求自选（竖屏/横版/方形等）。
type CanvasSpec struct {
	W     int      `json:"w"`
	H     int      `json:"h"`
	BG    []string `json:"bg"`
	Angle float64  `json:"angle"`
	Solid string   `json:"solid"`
}

// LayerSpec 是单个绘图/编辑图层，字段为紧凑 JSON。
// src 可为：产物 id、"ref:N"（第 N 张输入图，可先 cut/blur/rot 编辑再嵌入）、或图片 URL。
type LayerSpec struct {
	K   string `json:"k"` // img | text | rect | line | scrim
	Src string `json:"src"`
	X   int    `json:"x"`
	Y   int    `json:"y"`
	W   int    `json:"w"`
	H   int    `json:"h"`

	// image 效果
	Fit     string  `json:"fit"`
	Ov      float64 `json:"ov"`
	OvCol   string  `json:"ov_col"`
	Blur    int     `json:"blur"`
	Rot     float64 `json:"rot"`
	Persp   float64 `json:"persp"`
	Shear   float64 `json:"shear"`
	Cut     bool    `json:"cut"`
	CutTol  float64 `json:"cut_tol"`
	Circle  bool    `json:"circle"`
	Radius  int     `json:"radius"`
	Opacity float64 `json:"opacity"`
	Shadow  bool    `json:"shadow"`
	ShBlur  int     `json:"sh_blur"`
	ShDX    int     `json:"sh_dx"`
	ShDY    int     `json:"sh_dy"`
	Border  string  `json:"border"`
	BorderW int     `json:"border_w"`

	// text
	S       string  `json:"s"`
	Size    int     `json:"size"`
	Color   string  `json:"color"`
	Bold    bool    `json:"bold"`
	Italic  bool    `json:"italic"`
	Anchor  string  `json:"anchor"`
	Glow    string  `json:"glow"`
	GlowSz  int     `json:"glow_sz"`
	Stroke  string  `json:"stroke"`
	StrokeW int     `json:"stroke_w"`
	Spacing int     `json:"spacing"`
	LineH   float64 `json:"line_h"`
	Mono    bool    `json:"mono"`

	// rect / line / scrim
	Alpha    float64 `json:"alpha"`
	Fill     bool    `json:"fill"`
	X1       int     `json:"x1"`
	Y1       int     `json:"y1"`
	X2       int     `json:"x2"`
	Y2       int     `json:"y2"`
	Width    int     `json:"width"`
	Dir      string  `json:"dir"`
	Strength float64 `json:"strength"`
}

func normKind(k string) string {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "img", "image":
		return "image"
	case "text", "txt":
		return "text"
	case "rect", "box":
		return "rect"
	case "circle", "ellipse", "dot":
		return "circle"
	case "line":
		return "line"
	case "scrim", "shade":
		return "scrim"
	}
	return ""
}

func isRemote(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "data:") || strings.HasPrefix(s, "/")
}

func (l LayerSpec) toItem(resolve func(string) (string, error)) (skills.SpecItem, error) {
	k := normKind(l.K)
	it := skills.SpecItem{
		Type: k, X: l.X, Y: l.Y, W: l.W, H: l.H,
		Opacity: l.Opacity, Color: l.Color, Alpha: l.Alpha,
	}
	switch k {
	case "image":
		path, err := resolve(l.Src)
		if err != nil {
			return it, err
		}
		it.Src = path
		it.Fit = l.Fit
		it.Overlay = l.Ov
		it.OverlayCol = l.OvCol
		it.Blur = l.Blur
		it.Rotate = l.Rot
		it.Persp = l.Persp
		it.Shear = l.Shear
		it.RemoveBG = l.Cut
		it.RemoveTol = l.CutTol
		it.Circle = l.Circle
		it.Radius = l.Radius
		it.Shadow = l.Shadow
		it.ShadowBlur = l.ShBlur
		it.ShadowDX = l.ShDX
		it.ShadowDY = l.ShDY
		it.Border = l.Border
		it.BorderWidth = l.BorderW
	case "text":
		it.Text = l.S
		it.Size = l.Size
		if l.Bold {
			it.Weight = "bold"
		}
		it.Italic = l.Italic
		it.Anchor = l.Anchor
		it.Glow = l.Glow
		it.GlowSize = l.GlowSz
		it.Stroke = l.Stroke
		it.StrokeWidth = l.StrokeW
		it.Spacing = l.Spacing
		it.LineHeight = l.LineH
		it.Mono = l.Mono
	case "rect":
		it.Fill = l.Fill
		it.Radius = l.Radius
		it.Width = l.Width
	case "circle":
		// 圆形是明确的图形意图：用带圆角的填充矩形绘制，无需改底层渲染器。
		it.Type = "rect"
		it.Fill = true
		it.Radius = l.W / 2
		if it.Radius <= 0 {
			it.Radius = l.H / 2
		}
	case "line":
		it.P1 = []int{l.X1, l.Y1}
		it.P2 = []int{l.X2, l.Y2}
		it.Width = l.Width
	case "scrim":
		it.Direction = l.Dir
		it.Strength = l.Strength
	}
	return it, nil
}
