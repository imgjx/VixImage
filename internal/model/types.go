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
	// FormatOpenAI image: POST {base}/v1/images/generations (OpenAI Images，兼容绝大多数网关).
	FormatOpenAI APIFormat = "openai"
	// FormatOpenAIChat chat: POST {base}/chat/completions (OpenAI chat，支持流式规划).
	FormatOpenAIChat APIFormat = "openai_chat"
	// FormatGemini chat/image: POST {base}/v1beta/models/{model}:generateContent (Google Gemini).
	FormatGemini APIFormat = "gemini"
	// FormatStability image: POST {base}/v2beta/stable-image/generate/core (Stability AI).
	FormatStability APIFormat = "stability"
	// FormatAnthropic chat: POST {base}/v1/messages (Anthropic Claude).
	FormatAnthropic APIFormat = "anthropic"
)

// ValidFormats 返回所有支持的上游 API 格式。
func ValidFormats() []APIFormat {
	return []APIFormat{FormatOpenAI, FormatOpenAIChat, FormatGemini, FormatStability, FormatAnthropic}
}

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
//
// LLM 与图片模型各有专属字段（exclusive fields）：
//   - LLM 专属：MaxTokens / ContextLimit / Temperature / TopP / SystemPrompt
//   - 图片模型专属：Size / Sizes / N / Steps / NegativePrompt / AgentDesc
//
// 专属字段只对对应类型生效，避免把图片参数填给 LLM 造成误配。
type UpstreamModel struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`       // display name
	Type      ModelType `json:"type"`       // image | llm
	Format    APIFormat `json:"format"`     // openai | openai_chat | gemini | anthropic | stability
	BaseURL   string    `json:"base_url"`   // e.g. https://api.openai.com
	APIKey    string    `json:"api_key"`    // secret (stored as-is; masked on read)
	ModelName string    `json:"model_name"` // upstream model id, e.g. gpt-4o-mini

	// ---- 图片模型专属字段（Type == image 生效）----
	Size  string   `json:"size"`  // 默认尺寸, e.g. 1024x1024
	Sizes []string `json:"sizes"` // 支持的多种尺寸（空则不限制）
	N     int      `json:"n"`     // images per call
	Steps int      `json:"steps"` // 该引擎每次生成一张图消耗的步数（成本）
	// NegativePrompt 是反向提示词（部分上游支持），仅图片模型生效。
	NegativePrompt string `json:"negative_prompt"`
	// AgentDesc 是给 agent 看的「引擎介绍」：说明这个引擎擅长什么风格/
	// 题材，规划器据此对症选择图片模型。仅图片模型生效。
	AgentDesc string `json:"agent_desc"`

	// ---- LLM 专属字段（Type == llm 生效）----
	// MaxTokens 是 LLM 单次输出上限（小上下文模型尤其需要）。
	// 0 = 使用内置默认；Chat 请求会把它注入 max_tokens。
	MaxTokens int `json:"max_tokens"`
	// ContextLimit 是单次请求发给上游的【输入上下文字符上限】（防溢出）。
	// 与 max_tokens 同类：0 = 不限制；超出时按预算裁剪较早的消息文本，
	// 保留尾部用户要求与图片，避免复杂需求把上下文撑爆导致上游卡死。
	ContextLimit int `json:"context_limit"`
	// Temperature / TopP 控制采样；0 表示不注入该参数（走上游默认）。
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
	// SystemPrompt 是该 LLM 的系统提示词前缀（与 agent 提示词拼接）。
	SystemPrompt string `json:"system_prompt"`

	// Multimodal 为 true 才允许注册 LLM（必须是多模态/视觉模型）。
	Multimodal bool      `json:"multimodal"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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

// APIKey 是 API 服务的鉴权密钥。与上游模型/我的模型不同，密钥不在配置文件里，
// 而是存于数据库，以便统计每个密钥的调用次数、可单独禁用/删除。
// 默认不预置任何密钥，需管理员在「API 密钥」页或 CLI 创建。
type APIKey struct {
	ID         int64  `json:"id"`
	Key        string `json:"key"`          // 完整密钥（仅创建时回显一次）
	Name       string `json:"name"`         // 备注/标签
	Calls      int64  `json:"calls"`        // 累计调用次数
	LastUsedAt string `json:"last_used_at"` // 最近调用时间（RFC3339），空=从未调用
	CreatedAt  string `json:"created_at"`
	Enabled    bool   `json:"enabled"`
}

// MaskKey 返回用于列表展示的掩码（不暴露完整密钥）。
func (k APIKey) MaskKey() string { return maskSecret(k.Key) }

// MaskKey 对密钥做掩码（与 model.APIKey.MaskKey 等价的包级便利函数）。
func MaskKey(s string) string { return maskSecret(s) }
