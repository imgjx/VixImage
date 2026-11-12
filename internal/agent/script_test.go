package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"viximage/internal/model"
)

// 一轮典型的自循环输出：建画布 → 生图 → 加图层 → 查看作品（链底）。
const demoTurn = `{
 "think":"先铺深空底图再看看",
 "tools":[
   {"t":"canvas","w":1080,"h":1920,"bg":["#050912","#1a2a4a"],"angle":90},
   {"t":"gen","id":"hero","p":"coffee in space station, cinematic, no text no watermark","size":"720x1280"},
   {"t":"add","layer":{"k":"img","src":"hero","x":0,"y":0,"w":1080,"h":1300,"fit":"cover","ov":0.2,"blur":3,"rot":-4}},
   {"t":"add","layer":{"k":"img","src":"ref:0","x":690,"y":1350,"w":320,"h":320,"cut":true,"circle":true,"rot":8,"opacity":0.9}},
   {"t":"add","layer":{"k":"text","s":"星际咖啡\n深空也要喝一杯","x":80,"y":1420,"size":96,"color":"#ffffff","bold":true,"italic":true,"glow":"#3d8bff","glow_sz":18}},
   {"t":"add","layer":{"k":"rect","x":80,"y":1390,"w":520,"h":6,"color":"#ffcf5c","alpha":0.9,"fill":true}},
   {"t":"add","layer":{"k":"line","x1":80,"y1":1700,"x2":1000,"y2":1700,"color":"#7fb0e8","width":3}},
   {"t":"add","layer":{"k":"scrim","x":0,"y":0,"w":1080,"h":1920,"dir":"bottom","strength":0.7,"color":"#04070f"}},
   {"t":"view","srcs":["canvas"]}
 ]
}`

func parseDemoTurn(t *testing.T) *Turn {
	t.Helper()
	var turn Turn
	if err := json.Unmarshal([]byte(demoTurn), &turn); err != nil {
		t.Fatalf("示例 Turn 解析失败: %v", err)
	}
	return &turn
}

// TestTurnFreeLayout：一轮里可含多个工具、多种图层，画布由 LLM 自由决定，非写死模板。
func TestTurnFreeLayout(t *testing.T) {
	turn := parseDemoTurn(t)
	if err := validateTurn(turn, 1); err != nil {
		t.Fatalf("合法一轮应通过校验: %v", err)
	}
	if turn.Think == "" {
		t.Fatal("缺少短 ThinkFlag")
	}
	kinds := map[string]bool{}
	for _, tool := range turn.Tools {
		kinds[normTool(tool.T)] = true
	}
	for _, k := range []string{ToolCanvas, ToolGen, ToolAdd, ToolView} {
		if !kinds[k] {
			t.Fatalf("缺少工具 %s", k)
		}
	}
}

// TestChainBottom：view/finish 必须在最后一个、每轮最多一个；放中间或重复都报错。
func TestChainBottom(t *testing.T) {
	base := func(last string, extra ...string) string {
		tools := []map[string]any{{"t": "canvas", "w": 1080, "h": 1080}}
		for _, e := range extra {
			tools = append(tools, map[string]any{"t": e})
		}
		tools = append(tools, map[string]any{"t": last, "srcs": []string{"canvas"}, "src": "canvas"})
		b, _ := json.Marshal(map[string]any{"think": "x", "tools": tools})
		return string(b)
	}
	ok := func(raw string) *Turn {
		var turn Turn
		if err := json.Unmarshal([]byte(raw), &turn); err != nil {
			t.Fatalf("JSON 解不开: %v", err)
		}
		return &turn
	}
	// 正常：view / finish 在最后
	if err := validateTurn(ok(base(ToolView, "clear")), 0); err != nil {
		t.Fatalf("view 在最后应通过: %v", err)
	}
	if err := validateTurn(ok(base(ToolFinish, "clear")), 0); err != nil {
		t.Fatalf("finish 在最后应通过: %v", err)
	}
	// view 在中间 → 报错
	mid := `{"think":"x","tools":[{"t":"view","srcs":["canvas"]},{"t":"finish","src":"canvas"}]}`
	if err := validateTurn(ok(mid), 0); err == nil {
		t.Fatal("view 放在中间应报错（链底规则）")
	}
	// 两个链底工具 → 报错
	dup := `{"think":"x","tools":[{"t":"view","srcs":["canvas"]},{"t":"finish","src":"canvas"}]}`
	if err := validateTurn(ok(dup), 0); err == nil {
		t.Fatal("同时出现两个链底工具应报错")
	}
}

// TestNoFallbackAndAntiInjection：非法工具/超长文本/爆量工具数必须报错，绝不兜底放行。
func TestNoFallbackAndAntiInjection(t *testing.T) {
	longText := strings.Repeat("1", MaxTextRunes+50)
	cases := map[string]string{
		"空 tools":     `{"think":"x","tools":[]}`,
		"未知工具":        `{"think":"x","tools":[{"t":"fly"}]}`,
		"canvas 非法":   `{"think":"x","tools":[{"t":"canvas","w":10,"h":10}]}`,
		"gen 缺 id":    `{"think":"x","tools":[{"t":"gen","p":"a cat"}]}`,
		"gen ref 越界":  `{"think":"x","tools":[{"t":"gen","id":"a","p":"cat","ref":5}]}`,
		"view 缺 srcs": `{"think":"x","tools":[{"t":"view"}]}`,
		"文本超长(满屏1)":   `{"think":"x","tools":[{"t":"add","layer":{"k":"text","s":"` + longText + `"}}]}`,
	}
	for name, raw := range cases {
		var turn Turn
		if err := json.Unmarshal([]byte(raw), &turn); err != nil {
			t.Fatalf("%s: JSON 都解不开: %v", name, err)
		}
		if err := validateTurn(&turn, 1); err == nil {
			t.Fatalf("%s: 应报错，不能兜底放行", name)
		}
	}
	// 工具数爆量
	many := Turn{Think: "x"}
	for i := 0; i < MaxToolsPerTurn+1; i++ {
		many.Tools = append(many.Tools, Tool{T: ToolClear})
	}
	if err := validateTurn(&many, 0); err == nil {
		t.Fatal("超过单轮工具数上限应报错")
	}
}

// TestStepModel：步数模型 = 每次请求 1 步 + 生图按引擎扣分。
func TestStepModel(t *testing.T) {
	if StepPerLLMCall != 1 {
		t.Fatal("每次大模型请求应算 1 步")
	}
	if DefaultGenCost != 30 {
		t.Fatal("默认生图消耗应为 30 步")
	}
	// 模拟 3 轮请求 + 1 次生图(30) = 33 步
	used := 3*StepPerLLMCall + DefaultGenCost
	if used != 33 {
		t.Fatalf("步数计算错误: %d", used)
	}
}

// TestHasImageOutput：只有 gen/view 产生输出图，编辑类只回状态。
func TestHasImageOutput(t *testing.T) {
	if !hasImageOutput(ToolGen) || !hasImageOutput(ToolView) {
		t.Fatal("gen/view 应产生输出图")
	}
	for _, k := range []string{ToolCanvas, ToolAdd, ToolSet, ToolDel, ToolClear, ToolFinish} {
		if hasImageOutput(k) {
			t.Fatalf("%s 不应产生输出图", k)
		}
	}
}

// TestBuildStateSpec：内存状态 → 自由排版 spec + 参考图编辑/嵌入效果映射正确。
func TestBuildStateSpec(t *testing.T) {
	st := newRenderState()
	st.canvas = &CanvasSpec{W: 1080, H: 1920, BG: []string{"#050912", "#1a2a4a"}, Angle: 90}
	_ = st.addAsset("hero", &assetEntry{Path: "/tmp/hero.png", Kind: "gen", Desc: "太空咖啡"})
	_ = st.addAsset("ref:0", &assetEntry{Path: "/tmp/ref0.png", Kind: "ref", Desc: "用户输入图 0"})
	st.layers = []LayerSpec{
		{K: "img", Src: "hero", X: 0, Y: 0, W: 1080, H: 1300, Fit: "cover", Ov: 0.2, Blur: 3, Rot: -4},
		{K: "img", Src: "ref:0", X: 690, Y: 1350, W: 320, H: 320, Cut: true, Circle: true, Rot: 8, Opacity: 0.9},
		{K: "text", S: "星际咖啡\n深空也要喝一杯", X: 80, Y: 1420, Size: 96, Italic: true, Glow: "#3d8bff", GlowSz: 18},
	}
	resolve := func(src string) (string, error) {
		return st.assets[src].Path, nil
	}
	spec, err := st.BuildSpec(resolve, "/tmp/out.png")
	if err != nil {
		t.Fatalf("BuildSpec 失败: %v", err)
	}
	if spec.Width != 1080 || spec.Height != 1920 || len(spec.Items) != 3 {
		t.Fatalf("spec 不正确: %dx%d, %d 层", spec.Width, spec.Height, len(spec.Items))
	}
	im := spec.Items[0]
	if im.Src != "/tmp/hero.png" || im.Blur != 3 || im.Rotate != -4 || im.Overlay != 0.2 {
		t.Fatalf("image 效果映射错误: %+v", im)
	}
	ref := spec.Items[1]
	if ref.Src != "/tmp/ref0.png" || !ref.RemoveBG || !ref.Circle || ref.Rotate != 8 || ref.Opacity != 0.9 {
		t.Fatalf("参考图编辑/嵌入映射错误: %+v", ref)
	}
	if !spec.Items[2].Italic || !strings.Contains(spec.Items[2].Text, "\n") {
		t.Fatal("文字斜体/多行映射错误")
	}
	// 产物表应列出两条，且用户输入图明确标注「用户参考图」
	if table := st.assetTable(); !strings.Contains(table, "hero") ||
		!strings.Contains(table, "ref:0") || !strings.Contains(table, "用户参考图") {
		t.Fatalf("产物表条目/标注不全: %s", table)
	}
	// 未建画布时 BuildSpec 必须报错（无兜底）
	empty := newRenderState()
	if _, err := empty.BuildSpec(resolve, "/tmp/x.png"); err == nil {
		t.Fatal("没有画布时应报错，不能兜底")
	}
}

// TestContextAssembly：验证「主提示词 + 紧凑上下文 + 产物表 + 上一轮结果(条件) + 用户要求」拼接顺序。
func TestContextAssembly(t *testing.T) {
	engines := []EngineInfo{{Name: "Agnes 图片", Desc: "擅长科幻海报", Steps: 30, Sizes: []string{"1024x1024", "1280x720"}}}
	c := Ctx{
		Step: 2, Remaining: 68, Budget: 100, Style: "creative",
		Prompt: "做一张立冬海报", Engines: engines,
		CanvasDesc: "画布 1080x1920", LayerCount: 5, AssetCount: 2, NInputs: 0,
		LastResult: "生成 a1（见附图）", LastImages: []string{"data:image/png;base64,AAAA"},
	}
	parts := buildUserParts(c, "- a1 [gen] 太空咖啡", nil)
	if len(parts) == 0 {
		t.Fatal("没有生成任何 part")
	}
	text := parts[0].Text
	// 顺序：[上下文] → [产物表] → [上一轮结果] → <用户要求>
	iCtx := strings.Index(text, "[上下文]")
	iTable := strings.Index(text, "[产物表]")
	iLast := strings.Index(text, "[上一轮结果]")
	iUser := strings.Index(text, "<用户要求>")
	if !(iCtx >= 0 && iCtx < iTable && iTable < iLast && iLast < iUser) {
		t.Fatalf("拼接顺序错误: ctx=%d table=%d last=%d user=%d", iCtx, iTable, iLast, iUser)
	}
	if !strings.Contains(text, "剩余 68/100 步") {
		t.Fatal("上下文缺少剩余步数")
	}
	if !strings.Contains(text, "Agnes 图片(30步/次") || !strings.Contains(text, "擅长科幻海报") {
		t.Fatal("上下文缺少图像模型列表(名字/消耗步数/介绍)")
	}
	if !strings.Contains(text, "做一张立冬海报") {
		t.Fatal("缺少用户要求")
	}
	// 有输出图时应附 image part
	hasImg := false
	for _, p := range parts {
		if p.Type == "image_url" {
			hasImg = true
		}
	}
	if !hasImg {
		t.Fatal("上一轮有输出图，应附图片 part 回灌")
	}
	// 无上一轮结果（首轮/编辑轮）不拼结果段
	c2 := Ctx{Step: 1, Remaining: 100, Budget: 100, Prompt: "x"}
	text2 := buildUserParts(c2, "（暂无产物）", nil)[0].Text
	if strings.Contains(text2, "[上一轮结果]") {
		t.Fatal("首轮不应拼接上一轮结果段")
	}
}

// TestExtractJSON：支持 ```json 包裹与前后杂文。
func TestExtractJSON(t *testing.T) {
	in := "好的：\n```json\n{\"think\":\"x\",\"tools\":[{\"t\":\"finish\",\"src\":\"canvas\"}]}\n```\n以上。"
	got := extractJSON(in)
	var turn Turn
	if err := json.Unmarshal([]byte(got), &turn); err != nil {
		t.Fatalf("extractJSON 未取出合法对象: %v ｜ got=%q", err, got)
	}
	if len(turn.Tools) != 1 || normTool(turn.Tools[0].T) != ToolFinish {
		t.Fatalf("解析结果不对: %+v", turn.Tools)
	}
}

// TestCanvasColorTolerance：小模型常见的 bg 数字数组 / solid 布尔写法要能被接住，
// 尤其真实 Agnes 的「扁平 RGBA 数组 [10,10,30,1] + solid:true」不能让整轮 JSON 崩掉，
// 且应解析成正确的单色（这正是此前 502 的根因）。
func TestCanvasColorTolerance(t *testing.T) {
	// 扁平 RGBA 单色 + solid:true（Agnes 实跑输出）
	raw := `{"think":"建画布","tools":[{"t":"canvas","w":1080,"h":1920,"bg":[10,10,30,1],"solid":true}]}`
	var turn Turn
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		t.Fatalf("扁平 RGBA 背景不应让整轮解析失败: %v", err)
	}
	if err := validateTurn(&turn, 0); err != nil {
		t.Fatalf("合法 canvas 应通过校验: %v", err)
	}
	c := canvasSpec(turn.Tools[0])
	if c.W != 1080 || c.H != 1920 {
		t.Fatalf("画布尺寸错误: %dx%d", c.W, c.H)
	}
	if c.Solid != "#0a0a1e" {
		t.Fatalf("solid:true 应沿用 bg 推出的首色 #0a0a1e，实际 %q", c.Solid)
	}

	// 颜色串列表 → 渐变
	raw2 := `{"think":"x","tools":[{"t":"canvas","w":1000,"h":1000,"bg":["#050912","#1a2a4a"],"angle":90}]}`
	var t2 Turn
	if err := json.Unmarshal([]byte(raw2), &t2); err != nil {
		t.Fatal(err)
	}
	c2 := canvasSpec(t2.Tools[0])
	if len(c2.BG) != 2 || c2.BG[0] != "#050912" || c2.Angle != 90 {
		t.Fatalf("渐变背景解析错误: %+v", c2)
	}
	if c2.Solid != "" {
		t.Fatalf("未给 solid 时不应有纯色: %q", c2.Solid)
	}

	// 嵌套 RGBA 数组列表 → 渐变
	raw3 := `{"think":"x","tools":[{"t":"canvas","w":1000,"h":1000,"bg":[[10,10,30],[20,30,60]]}]}`
	var t3 Turn
	if err := json.Unmarshal([]byte(raw3), &t3); err != nil {
		t.Fatal(err)
	}
	c3 := canvasSpec(t3.Tools[0])
	if len(c3.BG) != 2 || c3.BG[0] != "#0a0a1e" || c3.BG[1] != "#141e3c" {
		t.Fatalf("嵌套 RGBA 解析错误: %+v", c3.BG)
	}
}

// TestToolWrapperSyntax：小模型把工具名作 key / 参数包在 params 里的写法要能解析
// （真实 Agnes 实跑输出 {"canvas":{...}}），但未知工具仍严格报错——解析容错 ≠ 操作兜底。
func TestToolWrapperSyntax(t *testing.T) {
	raw := `{"think":"x","tools":[
		{"canvas":{"w":1080,"h":1920,"bg":[10,10,30,1],"solid":true}},
		{"gen":{"id":"hero","p":"a planet"}},
		{"add":{"layer":{"k":"text","s":"星际","x":10,"y":10,"size":60}}},
		{"view":{"srcs":["canvas"]}}
	]}`
	var turn Turn
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		t.Fatalf("包装写法不应让整轮解析失败: %v", err)
	}
	if err := validateTurn(&turn, 0); err != nil {
		t.Fatalf("包装写法应通过校验: %v", err)
	}
	want := []string{ToolCanvas, ToolGen, ToolAdd, ToolView}
	if len(turn.Tools) != len(want) {
		t.Fatalf("工具数不对: %d", len(turn.Tools))
	}
	for i := range want {
		if got := normTool(turn.Tools[i].T); got != want[i] {
			t.Fatalf("第%d个工具解析为 %q，期望 %q", i, got, want[i])
		}
	}
	if turn.Tools[0].W != 1080 || turn.Tools[0].H != 1920 {
		t.Fatal("canvas 参数在解包时丢失")
	}
	if turn.Tools[1].ID != "hero" || turn.Tools[1].P != "a planet" {
		t.Fatal("gen 参数在解包时丢失")
	}
	if turn.Tools[2].Layer == nil || turn.Tools[2].Layer.S != "星际" {
		t.Fatal("add.layer 在解包时丢失")
	}

	// 字符串别名 + params 包裹
	var t2 Turn
	if err := json.Unmarshal([]byte(`{"think":"x","tools":[{"tool":"clear"},{"t":"canvas","params":{"w":800,"h":600}}]}`), &t2); err != nil {
		t.Fatal(err)
	}
	if normTool(t2.Tools[0].T) != ToolClear || t2.Tools[1].W != 800 {
		t.Fatalf("别名/params 解析失败: %+v", t2.Tools)
	}

	// 未知工具仍然必须报错，绝不兜底放行
	var t3 Turn
	if err := json.Unmarshal([]byte(`{"think":"x","tools":[{"fly":{"x":1}}]}`), &t3); err != nil {
		t.Fatal(err)
	}
	if err := validateTurn(&t3, 0); err == nil {
		t.Fatal("未知工具必须报错，不能兜底")
	}
}

// TestFieldValueTolerance：真实 Agnes 常把颜色写成 RGB 数组、数字写成字符串、
// 布尔写成 1、srcs 写成单个字符串。这些属于字段值写法差异，应被归一化后通过严格校验。
func TestFieldValueTolerance(t *testing.T) {
	raw := `{"think":"x","tools":[
		{"t":"canvas","w":"1080","h":1920,"bg":[[10,10,40],[20,5,30]]},
		{"gen":{"id":"starfield","p":"a planet","size":1080}},
		{"t":"add","layer":{"k":"text","s":"星海回声","x":540,"y":"800","size":"72","color":[255,255,255],"bold":1,"glow":[0,200,255,0.8],"glow_sz":20,"anchor":"ma"}},
		{"t":"view","srcs":"canvas"}
	]}`
	var turn Turn
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		t.Fatalf("字段值写法差异不应让整轮解析失败: %v", err)
	}
	if err := validateTurn(&turn, 0); err != nil {
		t.Fatalf("归一化后应通过严格校验: %v", err)
	}
	c := canvasSpec(turn.Tools[0])
	if c.W != 1080 || c.H != 1920 {
		t.Fatalf("数字字符串宽高归一化失败: %dx%d", c.W, c.H)
	}
	if len(c.BG) != 2 || c.BG[0] != "#0a0a28" || c.BG[1] != "#14051e" {
		t.Fatalf("嵌套 RGBA 渐变归一化失败: %+v", c.BG)
	}
	if turn.Tools[1].Size != "1080" || turn.Tools[1].ID != "starfield" {
		t.Fatalf("gen 字段归一化失败: %+v", turn.Tools[1])
	}
	l := turn.Tools[2].Layer
	if l == nil || l.Y != 800 || l.Size != 72 || !l.Bold {
		t.Fatalf("图层数字/布尔归一化失败: %+v", l)
	}
	if l.Color != "#ffffff" || l.Glow != "#00c8ff" {
		t.Fatalf("颜色数组归一化失败: color=%q glow=%q", l.Color, l.Glow)
	}
	if len(turn.Tools[3].Srcs) != 1 || turn.Tools[3].Srcs[0] != "canvas" {
		t.Fatalf("单字符串 srcs 归一化失败: %+v", turn.Tools[3].Srcs)
	}
}

// TestLayerArray：模型常把 layer 写成数组（一轮 add 一次加多个图层），
// 应展开且每个图层独立通过严格校验；数组里有一个非法图层就整体报错。
func TestLayerArray(t *testing.T) {
	raw := `{"think":"x","tools":[
		{"t":"canvas","w":1080,"h":1920},
		{"t":"add","layer":[
			{"k":"img","src":"hero","x":0,"y":0,"w":1080,"h":1920},
			{"k":"text","s":"星海回声","x":540,"y":480,"size":110,"color":"#ffffff"}
		]}
	]}`
	var turn Turn
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		t.Fatal(err)
	}
	if err := validateTurn(&turn, 0); err != nil {
		t.Fatalf("多图层 add 应通过校验: %v", err)
	}
	add := turn.Tools[1]
	if len(add.Layers) != 2 {
		t.Fatalf("应展开为 2 个图层，实际 %d", len(add.Layers))
	}
	if add.Layer == nil || add.Layer.K != "img" {
		t.Fatal("首层兼容字段丢失")
	}
	if add.Layers[1].S != "星海回声" {
		t.Fatal("第二层内容丢失")
	}
	bad := `{"think":"x","tools":[{"t":"add","layer":[{"k":"text","s":"` + strings.Repeat("1", MaxTextRunes+10) + `"}]}]}`
	var t2 Turn
	if err := json.Unmarshal([]byte(bad), &t2); err != nil {
		t.Fatal(err)
	}
	if err := validateTurn(&t2, 0); err == nil {
		t.Fatal("数组里含非法图层必须整体报错，不能兜底")
	}
}

// TestInputContextAndRefValidation：0 张输入图时上下文明确禁用 ref:N；
// 且引用不存在的输入图在 validateTurn 阶段就报错（无兜底），不必等渲染。
func TestInputContextAndRefValidation(t *testing.T) {
	c := Ctx{Step: 2, Remaining: 90, Budget: 100, Prompt: "x", NInputs: 0}
	text := buildUserParts(c, "（暂无产物）", nil)[0].Text
	if !strings.Contains(text, "禁用 ref:N") {
		t.Fatal("0 张输入图时上下文应明确禁用 ref:N")
	}
	raw := `{"think":"x","tools":[{"t":"add","layer":{"k":"img","src":"ref:0","w":100,"h":100}}]}`
	var turn Turn
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		t.Fatal(err)
	}
	if err := validateTurn(&turn, 0); err == nil {
		t.Fatal("0 张输入图时引用 ref:0 必须在决策阶段报错")
	}
	if err := validateTurn(&turn, 1); err != nil {
		t.Fatalf("1 张输入图时 ref:0 应合法: %v", err)
	}
}

// TestCircleKindAlias：模型自造的 circle 图层是明确图形意图，应接住并转成圆角填充矩形；
// 但其它未知 kind 仍严格报错（解析容错 ≠ 操作兜底）。
func TestCircleKindAlias(t *testing.T) {
	raw := `{"think":"x","tools":[{"t":"canvas","w":1080,"h":1080},
		{"t":"add","layer":{"k":"circle","x":900,"y":150,"r":80,"color":"#a1887f","alpha":0.6}}]}`
	var turn Turn
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		t.Fatal(err)
	}
	if err := validateTurn(&turn, 0); err != nil {
		t.Fatalf("circle 应通过校验: %v", err)
	}
	l := turn.Tools[1].Layer
	if l.W != 160 || l.H != 160 {
		t.Fatalf("r=80 应换算成 160x160: %+v", l)
	}
	resolve := func(s string) (string, error) { return s, nil }
	item, err := l.toItem(resolve)
	if err != nil {
		t.Fatal(err)
	}
	if item.Type != "rect" || !item.Fill || item.Radius != 80 {
		t.Fatalf("circle 应转成圆角填充矩形: %+v", item)
	}
	bad := `{"think":"x","tools":[{"t":"add","layer":{"k":"star"}}]}`
	var t2 Turn
	if err := json.Unmarshal([]byte(bad), &t2); err != nil {
		t.Fatal(err)
	}
	if err := validateTurn(&t2, 0); err == nil {
		t.Fatal("未知 kind 必须报错，不能兜底")
	}
}

// TestTrailingCommaTolerance：小模型常写出尾随逗号，这是 JSON 语法瑕疵而非操作错误，
// 应被清洗后正常解析；字符串内部的 ",}" 不能被误删。
func TestTrailingCommaTolerance(t *testing.T) {
	in := `{"think":"x","tools":[{"t":"view","srcs":["canvas",],},],}`
	got := stripTrailingCommas(in)
	var turn Turn
	if err := json.Unmarshal([]byte(got), &turn); err != nil {
		t.Fatalf("尾随逗号应被清洗: %v ｜ got=%s", err, got)
	}
	if len(turn.Tools) != 1 || normTool(turn.Tools[0].T) != ToolView {
		t.Fatalf("解析结果不对: %+v", turn.Tools)
	}
	s := `{"think":"a,} 保留", "tools":[{"t":"clear"}]}`
	if stripTrailingCommas(s) != s {
		t.Fatalf("字符串内部内容不应被改动: %s", stripTrailingCommas(s))
	}
}

// TestMergeTurnObjects：小模型把 think 与 tools 拆成两个并列 JSON 对象时，
// 应合并成可解析的一轮输出；单对象不走合并。
func TestMergeTurnObjects(t *testing.T) {
	in := "```json\n{\"think\":\"第3轮微调\"}\n{\"tools\":[{\"t\":\"finish\",\"src\":\"canvas\"}]}\n```"
	got := mergeTurnObjects(in)
	if got == "" {
		t.Fatal("应合并碎片对象")
	}
	var turn Turn
	if err := json.Unmarshal([]byte(got), &turn); err != nil {
		t.Fatalf("合并后应可解析: %v ｜ %s", err, got)
	}
	if len(turn.Tools) != 1 || normTool(turn.Tools[0].T) != ToolFinish {
		t.Fatalf("合并结果不对: %+v", turn.Tools)
	}
	if turn.Think != "第3轮微调" {
		t.Fatal("think 丢失")
	}
	if mergeTurnObjects(`{"think":"x","tools":[{"t":"clear"}]}`) != "" {
		t.Fatal("单对象不应触发合并")
	}
}

// TestResolveSrcPrefixTolerance：模型给产物 id 加 gen:/prod: 等前缀时应剥离后正常解析，
// 但完全无法识别的引用仍报错（无兜底）。
func TestResolveSrcPrefixTolerance(t *testing.T) {
	r := &Runtime{}
	assets := map[string]string{"mountain_scene": "/tmp/m.png"}
	for _, src := range []string{"gen:mountain_scene", "asset:mountain_scene",
		"prod:mountain_scene", "image:mountain_scene", "mountain_scene"} {
		p, err := r.resolveSrc(src, assets, nil, "/tmp")
		if err != nil || p != "/tmp/m.png" {
			t.Fatalf("src %q 解析失败: err=%v path=%q", src, err, p)
		}
	}
	if _, err := r.resolveSrc("unknown_thing", assets, nil, "/tmp"); err == nil {
		t.Fatal("未知引用必须报错，不能兜底")
	}
}

// TestCopyTool：copy 解析/别名/校验，且新产物名可写在 as 或 id。
func TestCopyTool(t *testing.T) {
	raw := `{"think":"存档一版","tools":[{"t":"copy","src":"hero","as":"hero_v1"}]}`
	var turn Turn
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		t.Fatal(err)
	}
	if err := validateTurn(&turn, 0); err != nil {
		t.Fatalf("合法 copy 应通过校验: %v", err)
	}
	if normTool(turn.Tools[0].T) != ToolCopy || copyDest(turn.Tools[0]) != "hero_v1" {
		t.Fatalf("copy 解析错误: %+v", turn.Tools[0])
	}
	// 别名 + 新产物名写在 id
	var t2 Turn
	if err := json.Unmarshal([]byte(`{"think":"x","tools":[{"clone":{"from":"canvas","id":"v2"}}]}`), &t2); err != nil {
		t.Fatal(err)
	}
	if normTool(t2.Tools[0].T) != ToolCopy || copyDest(t2.Tools[0]) != "v2" || t2.Tools[0].Src != "canvas" {
		t.Fatalf("clone 别名/id 写法解析失败: %+v", t2.Tools[0])
	}
	if err := validateTurn(&t2, 0); err != nil {
		t.Fatalf("合法 clone 应通过校验: %v", err)
	}
	// 缺 as/id 必须报错（无兜底）
	var t3 Turn
	if err := json.Unmarshal([]byte(`{"think":"x","tools":[{"t":"copy","src":"hero"}]}`), &t3); err != nil {
		t.Fatal(err)
	}
	if err := validateTurn(&t3, 0); err == nil {
		t.Fatal("copy 缺新产物名必须报错")
	}
	// copy 不算图像输出工具
	if hasImageOutput(ToolCopy) {
		t.Fatal("copy 不应产生回灌图片，只回状态文本")
	}
}

// TestUndoHistory：动作级快照回滚画布/图层（产物表常驻），编辑失误可精确撤销。
func TestUndoHistory(t *testing.T) {
	st := newRenderState()
	_ = st.addAsset("ref:0", &assetEntry{Path: "/tmp/r0.png", Kind: "ref", Desc: "用户参考图"})

	// 未建画布时 pushHistory 不记录回滚点（undo 永不退回「画布未建立」）。
	st.pushHistory()
	if len(st.history) != 0 {
		t.Fatal("无画布时不应记录回滚点")
	}

	// 建画布（首次不压快照），随后每个编辑动作前各压一次。
	st.canvas = &CanvasSpec{W: 1080, H: 1080, BG: []string{"#000000", "#111111"}}
	st.pushHistory() // add 动作前
	st.layers = append(st.layers, LayerSpec{K: "text", S: "标题", X: 10, Y: 10, Size: 80})
	st.pushHistory() // 第二个 add 动作前
	st.layers = append(st.layers, LayerSpec{K: "rect", X: 0, Y: 0, W: 100, H: 100})
	_ = st.addAsset("hero", &assetEntry{Path: "/tmp/h.png", Kind: "gen", Desc: "主视觉"})
	if len(st.layers) != 2 || len(st.history) != 2 {
		t.Fatal("编辑后状态不对")
	}

	// 回滚 1 个编辑动作 → 撤销第二个图层，画布仍在。
	n, err := st.undoHistory(1)
	if err != nil || n != 1 {
		t.Fatalf("undo 失败: n=%d err=%v", n, err)
	}
	if len(st.layers) != 1 || st.canvas == nil {
		t.Fatalf("应只剩 1 图层且画布仍在: layers=%d", len(st.layers))
	}
	// 产物表常驻，undo 不动它（版本交由 copy 管理）。
	if _, ok := st.assets["hero"]; !ok || len(st.order) != 2 {
		t.Fatalf("undo 不应动产物表: order=%+v", st.order)
	}

	// 回滚 2 步（只剩 1 个快照）→ 最多退到最初，不报错、画布仍在。
	if _, err := st.undoHistory(9); err != nil {
		t.Fatalf("回退超过快照数应收敛而非报错: %v", err)
	}
	if st.canvas == nil {
		t.Fatal("undo 不应把画布退成 nil")
	}
	if len(st.layers) != 0 {
		t.Fatalf("应回到 0 图层，实际 %d", len(st.layers))
	}

	// 快照用尽后再 undo → 报错（无兜底）
	if _, err := st.undoHistory(1); err == nil {
		t.Fatal("无快照时 undo 必须报错")
	}

	// 深拷贝校验：快照不得与当前 canvas 共享指针
	if snap := st.cloneSnap(); snap.canvas == st.canvas {
		t.Fatal("快照必须深拷贝 canvas 指针")
	}
}

// TestLayerTableAndIsEditing：layers 清单含编号；isEditingTool 只认会改状态的工具。
func TestLayerTableAndIsEditing(t *testing.T) {
	st := newRenderState()
	st.layers = []LayerSpec{
		{K: "text", S: "星海\n回声", X: 540, Y: 400, Size: 96},
		{K: "image", Src: "hero", X: 0, Y: 0, W: 1080, H: 1080},
	}
	tbl := st.layerTable()
	if !strings.Contains(tbl, "#0") || !strings.Contains(tbl, "#1") ||
		!strings.Contains(tbl, "text") || !strings.Contains(tbl, "src=hero") {
		t.Fatalf("图层清单内容不全: %s", tbl)
	}
	for _, k := range []string{ToolCanvas, ToolGen, ToolAdd, ToolSet, ToolDel, ToolClear, ToolCopy} {
		if !isEditingTool(k) {
			t.Fatalf("%s 应算编辑工具", k)
		}
	}
	for _, k := range []string{ToolView, ToolFinish, ToolUndo, ToolLayers} {
		if isEditingTool(k) {
			t.Fatalf("%s 不应算编辑工具", k)
		}
	}
	turn := &Turn{Tools: []Tool{{T: ToolAdd}, {T: ToolUndo}}}
	if !hasUndo(turn) {
		t.Fatal("hasUndo 未识别含 undo 的一轮")
	}
	// mutatesCanvas 只认会改画布/图层的动作（压回滚点用），gen/copy 只加产物不算
	for _, k := range []string{ToolCanvas, ToolAdd, ToolSet, ToolDel, ToolClear} {
		if !mutatesCanvas(k) {
			t.Fatalf("%s 应算画布编辑动作", k)
		}
	}
	for _, k := range []string{ToolGen, ToolCopy, ToolView, ToolFinish, ToolUndo, ToolLayers} {
		if mutatesCanvas(k) {
			t.Fatalf("%s 不应算画布编辑动作", k)
		}
	}
}

// TestQualityPrompt：主提示词必须强调质量完美/只允许极细微瑕疵、完成前 view 自检、一步多动作。
func TestQualityPrompt(t *testing.T) {
	for _, kw := range []string{"质量必须完美", "只允许极细微瑕疵", "view", "自检", "一步", "很多个动作", "copy", "undo", "用户参考图"} {
		if !strings.Contains(plannerSystemPrompt, kw) {
			t.Fatalf("主提示词缺少关键字 %q", kw)
		}
	}
}

// TestSystemPromptCachePrefix：主提示词 + 学习数据都是写死常量，拼成稳定 system 前缀。
// 关键：同一开关下每次返回完全一致（命中 prompt cache），主提示词必须在最前、学习数据在后。
func TestSystemPromptCachePrefix(t *testing.T) {
	on := systemPrompt(true)
	off := systemPrompt(false)
	// 关闭时只发主提示词
	if off != plannerSystemPrompt {
		t.Fatal("关闭学习数据时 system 应恰好等于主提示词")
	}
	// 开启时 = 主提示词 + 学习数据，且顺序固定（主提示词在前）
	if !strings.HasPrefix(on, plannerSystemPrompt) {
		t.Fatal("开启时 system 必须以写死的主提示词开头（缓存公共前缀）")
	}
	if !strings.Contains(on, learningData) {
		t.Fatal("开启时 system 必须拼接学习数据")
	}
	if strings.Index(on, plannerSystemPrompt) > strings.Index(on, "【示范") {
		t.Fatal("学习数据必须位于主提示词之后")
	}
	// 稳定性：多次调用返回完全相同（可命中缓存）
	for i := 0; i < 5; i++ {
		if systemPrompt(true) != on {
			t.Fatal("system 前缀必须恒定，否则无法命中上游缓存")
		}
	}
	// 主提示词/学习数据本身必须是编译期常量拼接（不含动态占位）
	if strings.Contains(on, "%d") && !strings.Contains(plannerSystemPrompt, "%d") {
		t.Fatal("system 里不应混入动态内容")
	}
}

// TestLearningDataCreative：学习数据示范「更主动的创意实现」——画中画 / 斜放屏幕 / 参考图嵌套 / 图生图。
func TestLearningDataCreative(t *testing.T) {
	for _, kw := range []string{"画中画", "场景化", "ref:0", "gen", "persp", "shear", "图生图", "一轮铺完整版", "view"} {
		if !strings.Contains(learningData, kw) {
			t.Fatalf("学习数据缺少创意示范关键字 %q", kw)
		}
	}
	// 主提示词也要有主动创意方法论引导
	for _, kw := range []string{"主动创意", "场景化构图", "物件内容", "gen{ref", "rot", "persp", "shear", "画中画贴合铁律"} {
		if !strings.Contains(plannerSystemPrompt, kw) {
			t.Fatalf("主提示词缺少主动创意方法论 %q", kw)
		}
	}
}

// TestForceReviewHints：系统级强制 view 的提示片段放 user（不污染写死 system 前缀）。
func TestForceReviewHints(t *testing.T) {
	if !strings.Contains(forceReviewHint, "强制回看") || !strings.Contains(forceReviewHint, "finish") {
		t.Fatal("强制自检提示应要求核对成片并再次 finish 确认")
	}
	if !strings.Contains(deepThinkHint, "推演") {
		t.Fatal("深度思考提示应引导推演构图")
	}
	// 两个提示都拼进 user parts，且不出现在 system
	c := Ctx{Step: 3, Remaining: 50, Budget: 100, Prompt: "做海报", DeepThink: true, ForceReview: true}
	text := buildUserParts(c, "（暂无产物）", nil)[0].Text
	if !strings.Contains(text, "系统强制自检") || !strings.Contains(text, "深度思考") {
		t.Fatal("开关提示应出现在 user 消息里")
	}
	for _, sys := range []string{systemPrompt(true), systemPrompt(false)} {
		if strings.Contains(sys, "系统强制自检") || strings.Contains(sys, "深度思考") {
			t.Fatal("动态开关提示不应污染写死的 system 前缀")
		}
	}
	// 关闭时 user 不含这些提示
	plain := buildUserParts(Ctx{Step: 1, Budget: 100, Prompt: "x"}, "（暂无产物）", nil)[0].Text
	if strings.Contains(plain, "系统强制自检") {
		t.Fatal("未开启强制自检时不应拼该提示")
	}
}

// TestMaxForceReviewsBound：交付前强制 view 必须有上限，防小模型反复改动耗光步数。
func TestMaxForceReviewsBound(t *testing.T) {
	if MaxForceReviews < 1 || MaxForceReviews > 5 {
		t.Fatalf("强制回看上限不合理: %d", MaxForceReviews)
	}
}

// TestNormalizeTailView：交付轮 [...view, finish] 收敛（系统级强制 view 接管），其余链底违规仍报错。
func TestNormalizeTailView(t *testing.T) {
	// 典型写法：view 紧接 finish → 去掉 view，只剩 finish，合法
	var t1 Turn
	_ = json.Unmarshal([]byte(`{"think":"x","tools":[{"t":"add","layer":{"k":"text","s":"标题","size":40}},{"t":"view","srcs":["canvas"]},{"t":"finish","src":"canvas"}]}`), &t1)
	normalizeTailView(&t1)
	if len(t1.Tools) != 2 || normTool(t1.Tools[0].T) != ToolAdd || normTool(t1.Tools[1].T) != ToolFinish {
		t.Fatalf("view+finish 应收敛为 add+finish: %+v", t1.Tools)
	}
	if err := validateTurn(&t1, 0); err != nil {
		t.Fatalf("收敛后应合法: %v", err)
	}
	// 单个 view（无 finish）不动
	var t2 Turn
	_ = json.Unmarshal([]byte(`{"think":"x","tools":[{"t":"add","layer":{"k":"text","s":"标题","size":40}},{"t":"view","srcs":["canvas"]}]}`), &t2)
	normalizeTailView(&t2)
	if len(t2.Tools) != 2 || normTool(t2.Tools[1].T) != ToolView {
		t.Fatal("末尾无 finish 时不应改动")
	}
	// view 夹在中间 + 末尾 finish（view 与 finish 之间有编辑）→ 不收敛，仍报错（无兜底）
	var t3 Turn
	_ = json.Unmarshal([]byte(`{"think":"x","tools":[{"t":"view","srcs":["canvas"]},{"t":"add","layer":{"k":"text","s":"标题","size":40}},{"t":"finish","src":"canvas"}]}`), &t3)
	normalizeTailView(&t3)
	if err := validateTurn(&t3, 0); err == nil {
		t.Fatal("view 夹在中间仍应被链底规则报错，不能兜底")
	}
	// finish 不在最后：不动
	var t4 Turn
	_ = json.Unmarshal([]byte(`{"think":"x","tools":[{"t":"finish","src":"canvas"},{"t":"view","srcs":["canvas"]}]}`), &t4)
	normalizeTailView(&t4)
	if err := validateTurn(&t4, 0); err == nil {
		t.Fatal("finish 不在最后仍应报错")
	}
}

// TestLearningSwitch：学习数据开关三态（空/on 启用，仅 off 关闭），并驱动 system 前缀长度。
func TestLearningSwitch(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"", true},
		{model.LearningOn, true},
		{model.LearningOff, false},
	}
	for _, c := range cases {
		if got := model.LearningEnabled(c.val); got != c.want {
			t.Fatalf("LearningEnabled(%q)=%v, want %v", c.val, got, c.want)
		}
	}
	if len(systemPrompt(model.LearningEnabled(model.LearningOff))) >=
		len(systemPrompt(model.LearningEnabled(model.LearningOn))) {
		t.Fatal("关闭学习数据时 system 前缀应更短")
	}
}
