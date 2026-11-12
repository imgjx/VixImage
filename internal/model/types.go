package model

import "time"

// ModelType is the category of an upstream model.
type ModelType string

const (
	TypeImage ModelType = "image" // image generation model (multimodal output)
	TypeLLM   ModelType = "llm"   // large language model (must be multimodal)
)

// APIFormat is the compatible upstream API shape.
type APIFormat string

const (
	// FormatOpenAI image: POST {base}/images/generations (OpenAI compatible).
	FormatOpenAI APIFormat = "openai"
	// FormatAgnes image: POST {base}/v1/images/generations (Agnes compatible).
	FormatAgnes APIFormat = "agnes"
	// FormatOpenAIChat chat: POST {base}/chat/completions (OpenAI compatible).
	FormatOpenAIChat APIFormat = "openai_chat"
)

// Agent 风格模式（我的模型可配置，决定 agent 规划时的创作姿态）。
const (
	// StyleOnDemand 按需执行：用户要什么就做什么，不加戏、最省步数。
	StyleOnDemand = "on_demand"
	// StyleCreative 创意脑洞：在需求之上额外加入自己的创意（斜体标题、
	// 斜放主视觉、更多分镜等）。
	StyleCreative = "creative"
	// StyleCinematic 电影大片：电影级质感，多分镜、严格审片、宁可多修一轮。
	StyleCinematic = "cinematic"
)

// ValidStyles 返回所有合法风格。
func ValidStyles() []string { return []string{StyleOnDemand, StyleCreative, StyleCinematic} }

// StyleLabel 风格中文名。
func StyleLabel(s string) string {
	switch s {
	case StyleCreative:
		return "创意脑洞"
	case StyleCinematic:
		return "电影大片"
	default:
		return "按需执行"
	}
}

// UpstreamModel is a registered upstream image/LLM model.
// Only multimodal-capable models are allowed.
type UpstreamModel struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`       // display name
	Type       ModelType `json:"type"`       // image | llm
	Format     APIFormat `json:"format"`     // openai | agnes | openai_chat
	BaseURL    string    `json:"base_url"`   // e.g. https://api.agnes-ai.cn
	APIKey     string    `json:"api_key"`    // secret (stored as-is; masked on read)
	ModelName  string    `json:"model_name"` // upstream model id, e.g. agnes-image-2.5-flash
	Size       string    `json:"size"`       // default size, e.g. 1024x1024
	Sizes      []string  `json:"sizes"`      // 支持的多种尺寸（空则不限制）
	N          int       `json:"n"`          // images per call
	Multimodal bool      `json:"multimodal"` // must be true
	// AgentDesc 是给 agent 看的「引擎介绍」：说明这个引擎擅长什么风格/
	// 题材，规划器据此对症选择图片模型。
	AgentDesc string `json:"agent_desc"`
	// MaxTokens 是 LLM 单次输出上限（小上下文模型尤其需要）。
	// 0 = 使用内置默认；Chat 请求会把它注入 max_tokens。
	MaxTokens int       `json:"max_tokens"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MaskKey returns the model with the api key masked for safe JSON exposure.
func (m UpstreamModel) MaskKey() UpstreamModel {
	m.APIKey = maskSecret(m.APIKey)
	return m
}

// ImageModelUse binds an upstream image model to an agent model.
type ImageModelUse struct {
	UpstreamID int64  `json:"upstream_id"`
	Name       string `json:"name"` // denormalized for display
	// Steps 是「每次调用该引擎生成一张图所消耗的步数」(成本)，默认 30。
	// 该值会写进 Agent 上下文，供大模型衡量生图是否划算、还剩多少步。
	Steps int `json:"steps"`
}

// AgentModel is a "我的模型" (agent definition created by the user).
type AgentModel struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`          // 模型名 *
	CallName    string          `json:"call_name"`     // 调用名 *
	BaseModelID int64           `json:"base_model_id"` // agent 基架模型 (upstream LLM) *
	MaxSteps    int             `json:"max_steps"`     // 操作最大步数 *
	ImageModels []ImageModelUse `json:"image_models"`  // 图片模型列表及每个模型的步数使用
	Prompt      string          `json:"prompt"`        // system/role prompt for the agent
	// Style 决定 agent 的创作姿态：on_demand 按需执行 / creative 创意脑洞 /
	// cinematic 电影大片。
	Style string `json:"style"`
	// DeepThink enables the deep-thinking planning mode: the base model
	// deliberates at length before committing to a plan.
	DeepThink bool `json:"deep_think"`
	// Exhaust 步数耗尽且仍未收到完成信号时的策略：
	//   "fail"    生成失败（默认）
	//   "partial" 返回当前已渲染的半成品成片
	Exhaust string `json:"exhaust"`
	// Learning 是否把写死的 few-shot「学习数据」拼到主提示词之后（system 固定前缀）。
	// 空(默认) / on = 启用；off = 关闭。开关改变 system 前缀，但同一模型仍恒定、可缓存。
	Learning  string    `json:"learning"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// 步数耗尽策略常量。
const (
	ExhaustFail    = "fail"
	ExhaustPartial = "partial"
)

// 学习数据（写死的 few-shot 示范）开关，按「我的模型」独立配置。
// 用字符串三态区分「未提交保持原值」与「显式关闭」，编辑时才能关掉。
const (
	LearningOn  = "on"
	LearningOff = "off"
)

// LearningEnabled 报告是否启用学习数据：空(默认) / on = 启用，仅 off = 关闭。
func LearningEnabled(v string) bool { return v != LearningOff }

// CallLog records a single API invocation of an agent model.
type CallLog struct {
	ID              int64  `json:"id"`
	APIKeyMask      string `json:"api_key_mask"`
	AgentCallName   string `json:"agent_call_name"`
	Prompt          string `json:"prompt"`
	InputImages     int    `json:"input_images"`
	OutputImages    int    `json:"output_images"`
	Status          string `json:"status"` // running | success | failed
	StepsUsed       int    `json:"steps_used"`
	BaseModelUsed   string `json:"base_model_used"`
	ImageModelsUsed string `json:"image_models_used"`
	DurationMs      int64  `json:"duration_ms"`
	Error           string `json:"error"`
	// DeepThink records whether deep-thinking mode was active for this call.
	DeepThink bool `json:"deep_think"`
	// Reasoning is the base model's deliberation trail.
	Reasoning string `json:"reasoning"`
	// Review is the visual QA verdict from the multimodal reviewer.
	Review string `json:"review"`
	// Theme is the creative direction chosen by the planner.
	Theme string `json:"theme"`
	// Style 是本次调用采用的风格模式。
	Style string `json:"style"`
	// Workspace 记录本次运行的临时工作区（用于排查/审计）。
	Workspace string `json:"workspace"`
	// LLMCalls 是本次自循环里大模型的请求次数（= 循环轮数 = 步数）。
	LLMCalls int `json:"llm_calls"`
	// Trace 存每一步的轨迹 JSON（[]TraceStep），供仪表盘展开查看每步思考与调用操作。
	Trace      string    `json:"trace"`
	CreatedAt  time.Time `json:"created_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// TraceStep 是自循环中「一步」(一次大模型请求) 的轨迹。
// think 是模型输出的短 ThinkFlag；ops 是这一步实际执行的调用操作摘要（仪表盘可见）。
// 通过 API 的 trace 参数附带时，只输出 {step, think}，不暴露具体技能。
type TraceStep struct {
	Step  int    `json:"step"`
	Think string `json:"think"`
	Ops   string `json:"ops,omitempty"`
}

// CallStats is an aggregate for the 调用概述 page.
type CallStats struct {
	Total     int64 `json:"total"`
	Success   int64 `json:"success"`
	Failed    int64 `json:"failed"`
	Running   int64 `json:"running"`
	Outputs   int64 `json:"outputs"`
	StepsUsed int64 `json:"steps_used"`
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		return "******"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
