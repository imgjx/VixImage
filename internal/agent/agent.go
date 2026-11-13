package agent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
	"viximage/internal/agent/skills"
	"viximage/internal/cache"
	"viximage/internal/client"
	"viximage/internal/config"
	"viximage/internal/model"
	"viximage/internal/storage"
	"viximage/internal/upstream"
)

// maxParallel 是并发创建图片请求的上限。
const maxParallel = 4

// Runtime 执行一个「我的模型」：一个简易的、技能全是图像工作相关的自循环 Agent。
// 每轮一次大模型请求 = 一步，产出一批工具调用(tools)；执行器严格解析并逐条执行。
// 有「完成信号」(finish) 与「查看作品」(view) 两个链底工具。无兜底：解析失败、
// 非法工具、生图失败、渲染失败一律报错终止。每次 Run 独立临时工作区，中间产物 100% 删除。
type Runtime struct {
	cfg   *config.Config
	store storage.Storage
	up    *upstream.Service
	cli   *client.Client
	cache cache.Cache
}

func New(cfg *config.Config, store storage.Storage, up *upstream.Service, cch cache.Cache) *Runtime {
	return &Runtime{cfg: cfg, store: store, up: up, cli: client.New(), cache: cch}
}

// RunRequest 是 API 调起一个 Agent 的载荷。
type RunRequest struct {
	AgentCallName string
	Prompt        string
	InputImages   []string // 本地文件路径（兼作参考图，可用 ref:N 嵌入）
	APIKeyMask    string
	Webhook       string
	DeepThink     bool
	Size          string // 强制覆盖所有 gen 的请求尺寸
	// Trace 为 true 时，API 响应附带每一步的短 ThinkFlag（不暴露用了哪些技能）。
	Trace bool
}

// RunResult 是 API 返回的结果。
type RunResult struct {
	OutputImages []string          `json:"output_images"`
	StepsUsed    int               `json:"steps_used"`
	LLMCalls     int               `json:"llm_calls"`
	Trace        []model.TraceStep `json:"-"` // 通过 API trace 参数按需精简输出
	Log          []string          `json:"log"`
	Status       string            `json:"status"`
	Error        string            `json:"error,omitempty"`
	Reasoning    string            `json:"reasoning,omitempty"`
	Theme        string            `json:"theme,omitempty"`
	DeepThink    bool              `json:"deep_think"`
	Style        string            `json:"style,omitempty"`
	Workspace    string            `json:"workspace,omitempty"`
	Exhausted    bool              `json:"exhausted,omitempty"`
	Partial      bool              `json:"partial,omitempty"`
}

// Run 加载模型 → 进入自循环：拼上下文 → 大模型决策(1步) → 执行一批工具 → 回灌结果 →
// 直到 finish / 步数耗尽。每一步都记 TraceStep。
func (r *Runtime) Run(req RunRequest) (*RunResult, error) {
	am, err := r.store.GetAgentModelByCallName(req.AgentCallName)
	if err != nil {
		return nil, err
	}
	if am == nil {
		return nil, fmt.Errorf("未找到调用名 %q 的模型", req.AgentCallName)
	}
	deep := am.DeepThink || req.DeepThink
	style := am.Style
	if style == "" {
		style = model.StyleOnDemand
	}
	budget := am.MaxSteps
	if budget <= 0 {
		budget = DefaultMaxSteps
	}
	exhaust := am.Exhaust
	if exhaust == "" {
		exhaust = model.ExhaustFail
	}

	// ---- 0. 独立临时工作区 ----
	ws := r.newWorkspace(am.CallName)
	if err := os.MkdirAll(filepath.Join(ws, "tmp"), 0o755); err != nil {
		return nil, err
	}
	log := &model.CallLog{
		APIKeyMask: req.APIKeyMask, AgentCallName: req.AgentCallName, Prompt: req.Prompt,
		InputImages: len(req.InputImages), Status: "running", Style: style,
		Workspace: ws, CreatedAt: time.Now().UTC(),
	}
	_ = r.store.AddCallLog(log)

	res := &RunResult{Status: "running", DeepThink: deep, Style: style, Workspace: ws}
	var logs []string
	var logMu sync.Mutex
	logFn := func(f string, a ...any) {
		s := fmt.Sprintf(f, a...)
		logMu.Lock()
		logs = append(logs, s)
		res.Log = logs
		logMu.Unlock()
	}
	defer func() {
		if err := os.RemoveAll(ws); err == nil {
			logFn("[工作区] 已清理临时工作区（中间产物 100%% 删除）")
		}
	}()

	sctx := &skills.Context{
		Client: client.New(), OutDir: ws, WorkDir: filepath.Join(ws, "tmp"),
		Python: r.cfg.PythonBin, Engine: r.cfg.ImageEngine, Log: logFn,
	}
	tmpDir := filepath.Join(ws, "tmp")

	// ---- 1. 基座模型 & 图像引擎 ----
	base, _ := r.up.Get(am.BaseModelID)
	if base == nil {
		return r.fail(log, res, fmt.Errorf("该 Agent 未配置有效的基座模型"))
	}
	engines := []EngineInfo{}
	for _, im := range am.ImageModels {
		if um, e := r.up.Get(im.UpstreamID); e == nil && um != nil && um.Type == model.TypeImage {
			cost := im.Steps
			if cost <= 0 {
				cost = DefaultGenCost
			}
			engines = append(engines, EngineInfo{Name: um.Name, Desc: um.AgentDesc, Steps: cost, Sizes: um.Sizes})
		}
	}

	// ---- 2. 初始化内存状态：输入图作为 ref:N 产物 ----
	st := newRenderState()
	for i, p := range req.InputImages {
		id := fmt.Sprintf("ref:%d", i)
		// 用户参考图也作为一条中间产物登记进产物表，明确标注，Agent 可复制/编辑后嵌入。
		_ = st.addAsset(id, &assetEntry{Path: p, Kind: "ref", Desc: fmt.Sprintf("用户参考图 %d（可 copy/编辑后嵌入）", i)})
	}
	// 首轮附给大模型的参考图（最多 3 张，缩略以控上下文）。
	refURIs := []string{}
	for i, p := range req.InputImages {
		if i >= 3 {
			break
		}
		if uri, e := encodeImageDataURI(p, 1024); e == nil {
			refURIs = append(refURIs, uri)
		}
	}

	// resolve 把逻辑 src（产物 id / ref:N / url）解析成本地路径；失败即错误（无兜底）。
	assetPaths := func() map[string]string {
		m := map[string]string{}
		for id, e := range st.assets {
			m[id] = e.Path
		}
		return m
	}
	resolve := func(src string) (string, error) {
		return r.resolveSrc(src, assetPaths(), req.InputImages, tmpDir)
	}
	// renderCurrent 把当前画布 + 图层渲染成 PNG（供 view 查看 / finish 交付）。
	renderCurrent := func(out string) (string, error) {
		spec, err := st.BuildSpec(resolve, out)
		if err != nil {
			return "", err
		}
		if _, err := skills.RenderSpecToFile(spec); err != nil {
			return "", fmt.Errorf("渲染失败（不回退）: %w", err)
		}
		return out, nil
	}
	// renderFinishTarget 解析 finish.src：空/canvas → 渲染当前画布；否则解析为产物文件。
	// 供「系统级强制 view」与最终交付共用，保证模型看到的就是要交付的那张。
	renderFinishTarget := func(src string) (string, error) {
		s := strings.TrimSpace(src)
		if s == "" || strings.EqualFold(s, "canvas") {
			out := filepath.Join(ws, fmt.Sprintf("compose_%d.png", time.Now().UnixNano()))
			return renderCurrent(out)
		}
		return resolve(s)
	}

	p := &planner{cli: client.New()}
	// system 由「主提示词 + 可选学习数据」两个写死常量拼成，同一模型恒定，
	// 便于上游 prompt cache 命中；动态开关(深度思考/强制自检)一律放 user 消息，不污染前缀。
	withLearning := model.LearningEnabled(am.Learning)
	sys := systemPrompt(withLearning)
	// 基座 LLM 自带的系统提示词前缀（LLM 专属字段）拼在最前面。
	if sp := strings.TrimSpace(base.SystemPrompt); sp != "" {
		sys = sp + "\n\n" + sys
	}

	logFn("[启动] 自循环 Agent ｜ 基座 %s ｜ 风格 %s ｜ 总步数 %d ｜ 耗尽策略 %s ｜ 引擎 %d ｜ 学习数据 %s",
		base.ModelName, model.StyleLabel(style), budget, exhaust, len(engines), map[bool]string{true: "开", false: "关"}[withLearning])

	var trace []model.TraceStep
	used := 0        // 已消耗步数（LLM 请求 1 + 生图按引擎扣分）
	llmCalls := 0    // 大模型请求次数
	lastResult := "" // 上一轮结果文本
	var lastImages []string
	var finalPath string
	finished := false
	partial := false
	// ---- 系统级强制 view：交付前必先让模型看到"当前要交付的成片" ----
	// 与模型自觉调用 view 无关：一旦模型发出 finish，系统不立即交付，而是把
	// finish.src（默认 canvas）渲染成图回灌下一轮，逼模型自检；只有"看过同一版本
	// 成片"后才允许交付。改动过画布(dirty)会重新触发，避免看了旧图再偷偷改。
	forceReviewDone := false // 当前待交付版本是否已被系统强制回看过
	forceReviewNext := false // 下一轮是否为「系统强制自检轮」
	forceReviewCount := 0    // 已强制回看次数（上限 MaxForceReviews，防死循环）
	canvasDirty := true      // 画布/图层自上次回看后是否有改动（首轮尚未成片，视为脏）

	for step := 1; ; step++ {
		remaining := budget - used
		if remaining <= 0 {
			// 步数耗尽且未收到 finish。
			if exhaust == model.ExhaustPartial && st.canvas != nil {
				logFn("[耗尽] 步数用尽未完成，按策略返回半成品（partial）")
				out := filepath.Join(ws, fmt.Sprintf("partial_%d.png", time.Now().UnixNano()))
				fp, e := renderCurrent(out)
				if e != nil {
					return r.fail(log, res, e)
				}
				finalPath = fp
				partial = true
				res.Exhausted = true
				break
			}
			return r.fail(log, res, fmt.Errorf("步数耗尽（%d/%d）仍未收到完成信号；exhaust=%s 生成失败", used, budget, exhaust))
		}

		canvasDesc := ""
		if st.canvas != nil {
			canvasDesc = fmt.Sprintf("画布 %dx%d", st.canvas.W, st.canvas.H)
		}
		ctx := Ctx{
			Step: step, Remaining: remaining, Budget: budget, Style: style, Prompt: req.Prompt,
			Engines: engines, CanvasDesc: canvasDesc, LayerCount: len(st.layers),
			AssetCount: len(st.order), NInputs: len(req.InputImages), UndoDepth: len(st.history),
			LastResult: lastResult, LastImages: lastImages, AgentPrompt: am.Prompt,
			DeepThink: deep, ForceReview: forceReviewNext,
		}
		parts := buildUserParts(ctx, st.assetTable(), refURIs)
		forceReviewNext = false // 该提示只作用于紧邻的这一轮自检

		// ---- 3. 一次大模型请求 = 1 步 ----
		turn, err := p.decide(*base, sys, parts)
		if err != nil {
			return r.fail(log, res, err) // 解析/调用失败：直接报错，无兜底
		}
		used += StepPerLLMCall
		llmCalls++
		// 空 tools 时先把原始输出记进日志，便于定位小模型跑偏。
		if len(turn.Tools) == 0 {
			logFn("[调试] 第 %d 步原始输出：%s", step, truncateStr(p.lastRaw, 400))
		}
		if err := validateTurn(turn, len(req.InputImages)); err != nil {
			return r.fail(log, res, fmt.Errorf("第 %d 步工具非法（不回退）: %w ｜原始输出: %s", step, err, truncateStr(p.lastRaw, 1500)))
		}
		if res.Theme == "" && strings.TrimSpace(turn.Think) != "" {
			res.Theme = truncateStr(turn.Think, 40)
		}

		// ---- 4. 顺序执行一批工具 ----
		var ops []string
		var outImages []string // 本轮产生的输出图（仅 gen / view）
		var outDesc []string
		roundLayerList := "" // layers 工具产出的图层清单文本
		roundFinished := false
		finishSrc := ""

		// 先收集 gen 任务（并行），其余顺序改状态。
		type genJob struct {
			tool  Tool
			um    model.UpstreamModel
			cost  int
			size  string
			input string
		}
		var jobs []genJob
		for _, tool := range turn.Tools {
			k := normTool(tool.T)
			// 画布/图层编辑动作执行前压回滚点（动作级），undo 可精确撤销。
			if mutatesCanvas(k) {
				st.pushHistory()
			}
			// 改动画布/图层 → 置脏：交付前若"看过成片后又改了"，必须重新强制回看。
			if mutatesCanvas(k) || k == ToolUndo {
				canvasDirty = true
			}
			switch k {
			case ToolCanvas:
				st.canvas = canvasSpec(tool)
				// 重设画布 = 新作品，清空旧图层，避免多轮重复建画布时图层叠加成一团。
				st.layers = nil
				ops = append(ops, fmt.Sprintf("canvas %dx%d", tool.W, tool.H))
			case ToolAdd:
				layers := tool.Layers
				if len(layers) == 0 && tool.Layer != nil {
					layers = []LayerSpec{*tool.Layer}
				}
				if len(st.layers)+len(layers) > MaxLayerCount {
					return r.fail(log, res, fmt.Errorf("第 %d 步：图层数超过上限 %d（拒绝爆量输出）", step, MaxLayerCount))
				}
				for _, l := range layers {
					st.layers = append(st.layers, l)
					ops = append(ops, "add "+normKind(l.K))
				}
			case ToolSet:
				layers := tool.Layers
				if len(layers) == 0 && tool.Layer != nil {
					layers = []LayerSpec{*tool.Layer}
				}
				if tool.I < 0 || tool.I+len(layers) > len(st.layers) {
					return r.fail(log, res, fmt.Errorf("第 %d 步 set 索引 %d 越界（图层 %d）", step, tool.I, len(st.layers)))
				}
				for j, l := range layers {
					st.layers[tool.I+j] = l
					ops = append(ops, fmt.Sprintf("set[%d] %s", tool.I+j, normKind(l.K)))
				}
			case ToolDel:
				if tool.I < 0 || tool.I >= len(st.layers) {
					return r.fail(log, res, fmt.Errorf("第 %d 步 del 索引 %d 越界（图层 %d）", step, tool.I, len(st.layers)))
				}
				st.layers = append(st.layers[:tool.I], st.layers[tool.I+1:]...)
				ops = append(ops, fmt.Sprintf("del[%d]", tool.I))
			case ToolClear:
				st.layers = nil
				ops = append(ops, "clear")
			case ToolCopy:
				// 把某个中间产物（或当前 canvas 渲染结果）复制一份，登记成新产物 id。
				// 物理复制成独立文件：源与新产物互不干扰，Agent 可存档/分叉/分别嵌入。
				dest := copyDest(tool)
				src := strings.TrimSpace(tool.Src)
				var srcPath string
				if strings.EqualFold(src, "canvas") {
					tmp := filepath.Join(ws, fmt.Sprintf("copy_src_%d_%d.png", step, time.Now().UnixNano()))
					pth, e := renderCurrent(tmp)
					if e != nil {
						return r.fail(log, res, fmt.Errorf("第 %d 步 copy(canvas): %w", step, e))
					}
					srcPath = pth
				} else {
					pth, e := resolve(src)
					if e != nil {
						return r.fail(log, res, fmt.Errorf("第 %d 步 copy(%s): %w", step, src, e))
					}
					srcPath = pth
				}
				ext := filepath.Ext(srcPath)
				if ext == "" {
					ext = ".png"
				}
				dst := filepath.Join(tmpDir, fmt.Sprintf("copy_%d_%d%s", step, time.Now().UnixNano(), ext))
				if e := copyFile(srcPath, dst); e != nil {
					return r.fail(log, res, fmt.Errorf("第 %d 步 copy 复制文件失败: %w", step, e))
				}
				if e := st.addAsset(dest, &assetEntry{Path: dst, Kind: "edit", Desc: fmt.Sprintf("复制自 %s", src)}); e != nil {
					return r.fail(log, res, fmt.Errorf("第 %d 步 copy: %w", step, e))
				}
				ops = append(ops, fmt.Sprintf("copy %s→%s", src, dest))
			case ToolUndo:
				// 回滚最近 n 轮编辑（画布/图层/产物表整体恢复），编辑失误时撤销重来。
				n, e := st.undoHistory(tool.N)
				if e != nil {
					return r.fail(log, res, fmt.Errorf("第 %d 步 undo: %w", step, e))
				}
				ops = append(ops, fmt.Sprintf("undo×%d", n))
			case ToolLayers:
				roundLayerList = st.layerTable()
				ops = append(ops, "layers")
			case ToolGen:
				um, cost := r.pickEngine(am, tool.Engine)
				if um == nil {
					return r.fail(log, res, fmt.Errorf("第 %d 步 gen「%s」无可用图片引擎", step, tool.ID))
				}
				aspect := "1:1"
				if st.canvas != nil {
					aspect = aspectOf(st.canvas.W, st.canvas.H)
				}
				size := tool.Size
				if req.Size != "" {
					size = req.Size
				}
				// 归一到该引擎真正支持的规格：模型常自造尺寸，避免上游拒收。
				size = pickSize(size, aspect, EngineInfo{Sizes: um.Sizes})
				input := ""
				if tool.RefStr != "" {
					// LLM 指定任意参考：产物 id（之前生成的图 / 复制的中间产物）、
					// "ref:N"（输入图）、或图片网址——都做图生图（把参考内容迭代/变体）。
					pth, e := resolve(tool.RefStr)
					if e != nil {
						return r.fail(log, res, fmt.Errorf("第 %d 步 gen「%s」参考图 %q 解析失败: %w", step, tool.ID, tool.RefStr, e))
					}
					input = pth
				} else if tool.Ref != nil {
					if *tool.Ref < 0 || *tool.Ref >= len(req.InputImages) {
						return r.fail(log, res, fmt.Errorf("第 %d 步 gen「%s」ref=%d 越界（输入图共 %d 张）", step, tool.ID, *tool.Ref, len(req.InputImages)))
					}
					input = req.InputImages[*tool.Ref]
				}
				jobs = append(jobs, genJob{tool, *um, cost, size, input})
			case ToolQrcode:
			// 生成二维码 → 登记为产物（零步消耗），可当 img 图层 src 嵌进海报。
			size := 512
			if tool.W > 0 {
				size = tool.W
			}
			qrPNG, e := skills.GenerateQR(tool.Text, size, tool.FG, tool.Bg)
			if e != nil {
				return r.fail(log, res, fmt.Errorf("第 %d 步 qrcode「%s」: %w", step, tool.ID, e))
			}
			qrPath := filepath.Join(tmpDir, fmt.Sprintf("qr_%d_%d.png", step, time.Now().UnixNano()))
			if e := os.WriteFile(qrPath, qrPNG, 0o644); e != nil {
				return r.fail(log, res, fmt.Errorf("第 %d 步 qrcode 写文件失败: %w", step, e))
			}
			if e := st.addAsset(tool.ID, &assetEntry{
				Path: qrPath, Kind: "edit", Desc: fmt.Sprintf("二维码 %s", truncateStr(tool.Text, 40)),
			}); e != nil {
				return r.fail(log, res, fmt.Errorf("第 %d 步 qrcode: %w", step, e))
			}
			ops = append(ops, fmt.Sprintf("qrcode %s", tool.ID))
		case ToolQrdecode:
			// 解析二维码内容（只读），解析文本回灌下一轮决策。
			var srcPath string
			if strings.EqualFold(strings.TrimSpace(tool.Src), "canvas") {
				tmp := filepath.Join(ws, fmt.Sprintf("qrscan_%d_%d.png", step, time.Now().UnixNano()))
				pth, e := renderCurrent(tmp)
				if e != nil {
					return r.fail(log, res, fmt.Errorf("第 %d 步 qrdecode(canvas): %w", step, e))
				}
				srcPath = pth
			} else {
				pth, e := resolve(tool.Src)
				if e != nil {
					return r.fail(log, res, fmt.Errorf("第 %d 步 qrdecode(%s): %w", step, tool.Src, e))
				}
				srcPath = pth
			}
			text, e := skills.DecodeQR(srcPath)
			if e != nil {
				return r.fail(log, res, fmt.Errorf("第 %d 步 qrdecode(%s): %w", step, tool.Src, e))
			}
			ops = append(ops, fmt.Sprintf("qrdecode(%s)→%q", tool.Src, truncateStr(text, 200)))
		case ToolView:
				// 链底工具：查看 canvas 或指定产物（可多张），产出结果图回灌下一轮。
				for i, s := range tool.Srcs {
					var path string
					if strings.EqualFold(strings.TrimSpace(s), "canvas") {
						out := filepath.Join(ws, fmt.Sprintf("view_%d_%d.png", step, i))
						pth, e := renderCurrent(out)
						if e != nil {
							return r.fail(log, res, fmt.Errorf("第 %d 步 view(canvas): %w", step, e))
						}
						path = pth
					} else {
						pth, e := resolve(s)
						if e != nil {
							return r.fail(log, res, fmt.Errorf("第 %d 步 view(%s): %w", step, s, e))
						}
						path = pth
					}
					uri, e := encodeImageDataURI(path, 1024)
					if e != nil {
						return r.fail(log, res, fmt.Errorf("第 %d 步 view 编码失败: %w", step, e))
					}
					outImages = append(outImages, uri)
				}
				ops = append(ops, fmt.Sprintf("view[%s]", strings.Join(tool.Srcs, ",")))
				outDesc = append(outDesc, fmt.Sprintf("查看 %s（见附图）", strings.Join(tool.Srcs, ",")))
			case ToolFinish:
				roundFinished = true
				finishSrc = tool.Src
				ops = append(ops, "finish")
			}
		}

		// 并行执行 gen（任一失败即整体失败，无兜底）。
		if len(jobs) > 0 {
			var wg sync.WaitGroup
			sem := make(chan struct{}, maxParallel)
			errs := make([]error, len(jobs))
			paths := make([]string, len(jobs))
			for i, jb := range jobs {
				wg.Add(1)
				go func(idx int, j genJob) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					logFn("  · gen「%s」→ %s ｜ %s ｜ 消耗 %d 步", j.tool.ID, j.um.Name, j.size, j.cost)
					imgs, e := sctx.Produce(j.um, j.tool.P, j.input, j.size)
					if e != nil || len(imgs) == 0 {
						if e == nil {
							e = fmt.Errorf("上游未返回图片")
						}
						errs[idx] = fmt.Errorf("gen「%s」失败: %w", j.tool.ID, e)
						return
					}
					paths[idx] = imgs[0]
				}(i, jb)
			}
			wg.Wait()
			for _, e := range errs {
				if e != nil {
					return r.fail(log, res, e)
				}
			}
			for i, jb := range jobs {
				if err := st.addAsset(jb.tool.ID, &assetEntry{
					Path: paths[i], Kind: "gen", Size: jb.size, Desc: truncateStr(jb.tool.P, 60),
				}); err != nil {
					return r.fail(log, res, fmt.Errorf("第 %d 步: %w", step, err))
				}
				used += jb.cost
				uri, e := encodeImageDataURI(paths[i], 1024)
				if e == nil {
					outImages = append(outImages, uri)
				}
				ops = append(ops, "gen "+jb.tool.ID)
				outDesc = append(outDesc, fmt.Sprintf("已生成产物 %s（见附图）", jb.tool.ID))
			}
		}

		// 记录本步轨迹：短 ThinkFlag + 调用操作摘要（仪表盘可见）。
		trace = append(trace, model.TraceStep{Step: step, Think: turn.Think, Ops: strings.Join(ops, " · ")})
		logFn("[第 %d 步] 💭 %s ｜ 操作：%s ｜ 消耗 %d/%d 步",
			step, orStr(truncateStr(turn.Think, 40), "（无）"), orStr(strings.Join(ops, " · "), "无"), used, budget)

		// ---- 5. 组装「上一轮结果」：只有 gen/view 产生输出图；编辑轮只回状态文本 ----
		if len(outImages) > 0 {
			if len(outImages) > MaxOutputPerView {
				outImages = outImages[:MaxOutputPerView]
			}
			lastResult = strings.Join(outDesc, "；")
			if roundLayerList != "" {
				lastResult += "\n" + roundLayerList
			}
			lastImages = outImages
		} else {
			status := "编辑成功：" + orStr(strings.Join(ops, " · "), "无操作")
			if roundLayerList != "" {
				status += "\n[图层清单]\n" + roundLayerList
			}
			lastResult = status
			lastImages = nil
		}

		// ---- 6. 完成信号 → 系统级强制 view → 渲染最终产物 ----
		if roundFinished {
			// 系统级强制 view：无论模型是否自觉调用 view，交付前都由系统强制把
			// "本次要交付的成片"渲染出来回灌给模型自检。若尚未回看过、或看过之后
			// 画布又被改动(canvasDirty)，就拦截这次 finish：渲染成片 → 作为下一轮
			// 附图与自检提示 → 继续自循环（不交付）。模型看过成片后，满意可再次
			// finish 确认，有瑕疵先修正（修正会置脏，触发下一轮强制回看）。
			// forceReviewCount 上限防止小模型反复改动 / 反复 finish 耗光步数。
			if (!forceReviewDone || canvasDirty) && forceReviewCount < MaxForceReviews {
				fp, e := renderFinishTarget(finishSrc)
				if e != nil {
					return r.fail(log, res, fmt.Errorf("系统强制 view 渲染失败（不回退）: %w", e))
				}
				uri, e := encodeImageDataURI(fp, 1024)
				if e != nil {
					return r.fail(log, res, fmt.Errorf("系统强制 view 编码失败: %w", e))
				}
				forceReviewDone = true
				forceReviewNext = true
				forceReviewCount++
				canvasDirty = false
				lastImages = []string{uri}
				lastResult = "系统级强制回看：附图是你本次要交付的成片；请逐项核对，满意再 finish 确认，有瑕疵先修正。"
				// 本步轨迹标注系统拦截，便于仪表盘看清「finish 被系统强制 view 拦下」。
				if len(trace) > 0 {
					trace[len(trace)-1].Ops += " · 系统强制view(拦截)"
				}
				logFn("[系统强制view] 拦截 finish，渲染成片回灌下一轮自检（第 %d/%d 次）", forceReviewCount, MaxForceReviews)
				continue
			}
			// 已通过系统强制回看（或达到回看上限）→ 真正渲染交付。
			fp, e := renderFinishTarget(finishSrc)
			if e != nil {
				return r.fail(log, res, fmt.Errorf("finish 渲染失败: %w", e))
			}
			finalPath = fp
			finished = true
			logFn("[完成] 收到 finish，产物已标注：%s（已通过系统强制 view）", orStr(strings.TrimSpace(finishSrc), "canvas"))
			break
		}
	}

	_ = finished
	// ---- 7. 提取成品 + 清理 ----
	finals := []string{}
	outputsDir := filepath.Join(r.cfg.DataDir, "outputs")
	_ = os.MkdirAll(outputsDir, 0o755)
	dst := filepath.Join(outputsDir, fmt.Sprintf("final_%d_%s", time.Now().UnixNano(), filepath.Base(finalPath)))
	if copyFile(finalPath, dst) == nil {
		finals = append(finals, dst)
		logFn("[提取] 成品已保存：%s", dst)
	} else {
		finals = append(finals, finalPath)
	}
	res.OutputImages = finals
	if r.cfg.DeleteOutputs {
		for _, f := range finals {
			_ = os.Remove(f)
		}
		logFn("[配置] delete_outputs 已开启：成品提取后删除（仅日志留档）")
	}

	// ---- 8. 落库 ----
	traceJSON, _ := json.Marshal(trace)
	thinkAll := []string{}
	for _, t := range trace {
		if t.Think != "" {
			thinkAll = append(thinkAll, fmt.Sprintf("第%d步：%s", t.Step, t.Think))
		}
	}
	res.StepsUsed = used
	res.LLMCalls = llmCalls
	res.Trace = trace
	res.Reasoning = strings.Join(thinkAll, "\n")
	res.Partial = partial
	res.Status = "success"
	if partial {
		res.Status = "success_partial"
	}

	log.Status = res.Status
	log.OutputImages = len(finals)
	log.StepsUsed = used
	log.LLMCalls = llmCalls
	log.BaseModelUsed = baseModelName(r.up, am.BaseModelID)
	names := []string{}
	for _, e := range engines {
		names = append(names, e.Name)
	}
	log.ImageModelsUsed = strings.Join(names, ", ")
	log.DeepThink = deep
	log.Reasoning = res.Reasoning
	log.Theme = res.Theme
	log.Trace = string(traceJSON)
	log.FinishedAt = time.Now().UTC()
	log.DurationMs = time.Since(log.CreatedAt).Milliseconds()
	_ = r.store.UpdateCallLog(log)

	return res, nil
}

// resolveSrc 把逻辑图片引用解析成本地文件路径：
//   - "ref:N" / "input:N" / 产物 id → 对应本地路径
//   - http/data/绝对路径             → 下载/解码落地或直接返回
func (r *Runtime) resolveSrc(src string, assets map[string]string, inputs []string, tmpDir string) (string, error) {
	s := strings.TrimSpace(src)
	low := strings.ToLower(s)
	// 模型常给产物 id 加自造前缀（gen:/asset:/prod:/image:/out:），剥离后按产物 id 解析。
	// 引用目标明确，属于字段写法容错，不是「猜操作」。
	for _, p := range []string{"gen:", "asset:", "prod:", "product:", "image:", "out:", "output:"} {
		if strings.HasPrefix(low, p) {
			s = strings.TrimSpace(s[len(p):])
			low = strings.ToLower(s)
			break
		}
	}
	if strings.HasPrefix(low, "ref:") || strings.HasPrefix(low, "input:") {
		idxStr := s[strings.IndexByte(s, ':')+1:]
		var n int
		if _, err := fmt.Sscanf(idxStr, "%d", &n); err != nil || n < 0 || n >= len(inputs) {
			return "", fmt.Errorf("src %q 越界（输入图共 %d 张）", src, len(inputs))
		}
		return inputs[n], nil
	}
	if p, ok := assets[s]; ok {
		return p, nil
	}
	if strings.HasPrefix(s, "data:") {
		return r.materialize(s, tmpDir, "ref")
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return r.materialize(s, tmpDir, "url")
	}
	if strings.HasPrefix(s, "/") {
		if _, err := os.Stat(s); err == nil {
			return s, nil
		}
		return "", fmt.Errorf("src 指向的本地文件不存在：%s", s)
	}
	return "", fmt.Errorf("无法解析图片引用 %q（不是产物 id / ref:N / URL）", src)
}

// materialize 把 data URI / URL 落地为工作区文件，供渲染器读取。
func (r *Runtime) materialize(src, tmpDir, tag string) (string, error) {
	_ = os.MkdirAll(tmpDir, 0o755)
	dst := filepath.Join(tmpDir, fmt.Sprintf("src_%s_%d.png", tag, time.Now().UnixNano()))
	var data []byte
	if strings.HasPrefix(src, "data:") {
		i := strings.IndexByte(src, ',')
		if i < 0 {
			return "", fmt.Errorf("非法 data URI")
		}
		b, err := base64.StdEncoding.DecodeString(src[i+1:])
		if err != nil {
			return "", fmt.Errorf("data URI 解码失败: %w", err)
		}
		data = b
	} else {
		resp, err := http.Get(src)
		if err != nil {
			return "", fmt.Errorf("下载图片失败: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("下载图片 http %d", resp.StatusCode)
		}
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

func aspectOf(w, h int) string {
	if w <= 0 || h <= 0 {
		return "1:1"
	}
	ratio := float64(w) / float64(h)
	switch {
	case ratio > 1.6:
		return "16:9"
	case ratio < 0.7:
		return "9:16"
	case ratio > 0.92 && ratio < 1.08:
		return "1:1"
	default:
		return "4:3"
	}
}

// pickEngine 按 gen.engine 名匹配图片引擎，否则返回第一个可用引擎。
func (r *Runtime) pickEngine(am *model.AgentModel, prefer string) (*model.UpstreamModel, int) {
	var first *model.UpstreamModel
	firstCost := DefaultGenCost
	for _, im := range am.ImageModels {
		um, _ := r.up.Get(im.UpstreamID)
		if um == nil || um.Type != model.TypeImage {
			continue
		}
		cost := im.Steps
		if cost <= 0 {
			cost = DefaultGenCost
		}
		if first == nil {
			first, firstCost = um, cost
		}
		if prefer != "" && (strings.Contains(um.Name, prefer) || strings.Contains(prefer, um.Name)) {
			return um, cost
		}
	}
	return first, firstCost
}

func (r *Runtime) newWorkspace(call string) string {
	return filepath.Join(r.cfg.DataDir, "workspaces", fmt.Sprintf("%s_%d", sanitizeName(call), time.Now().UnixNano()))
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "run"
	}
	return b.String()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (r *Runtime) fail(log *model.CallLog, res *RunResult, err error) (*RunResult, error) {
	res.Status = "failed"
	res.Error = err.Error()
	log.Status = "failed"
	log.Error = err.Error()
	log.FinishedAt = time.Now().UTC()
	log.DurationMs = time.Since(log.CreatedAt).Milliseconds()
	_ = r.store.UpdateCallLog(log)
	res.Log = append(res.Log, "[失败] "+err.Error())
	return res, err
}

func truncateStr(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "..."
}

// encodeImageDataURI 读取本地图片，缩放到长边 ≤ maxEdge（控上下文），编码为 PNG data URI。
func encodeImageDataURI(path string, maxEdge int) (string, error) {
	img, err := skills.LoadImage(path)
	if err != nil {
		return "", err
	}
	if maxEdge > 0 {
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		m := w
		if h > m {
			m = h
		}
		if m > maxEdge {
			scale := float64(maxEdge) / float64(m)
			nw, nh := int(float64(w)*scale), int(float64(h)*scale)
			if nw < 1 {
				nw = 1
			}
			if nh < 1 {
				nh = 1
			}
			dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
			draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
			img = dst
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func baseModelName(up *upstream.Service, id int64) string {
	if up == nil || id == 0 {
		return ""
	}
	m, _ := up.Get(id)
	if m == nil {
		return ""
	}
	return m.Name
}
