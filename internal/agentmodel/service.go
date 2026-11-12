package agentmodel

import (
	"fmt"

	"viximage/internal/model"
	"viximage/internal/storage"
	"viximage/internal/upstream"
)

// Service manages "我的模型" (agent definitions).
type Service struct {
	store    storage.Storage
	upstream *upstream.Service
}

func New(store storage.Storage, up *upstream.Service) *Service {
	return &Service{store: store, upstream: up}
}

// Save validates and persists an agent model. Required fields: Name, CallName,
// BaseModelID, MaxSteps. ImageModels is a list of {upstream_id, steps}.
func (s *Service) Save(m *model.AgentModel) error {
	if m.Name == "" {
		return fmt.Errorf("模型名 必填")
	}
	if m.CallName == "" {
		return fmt.Errorf("调用名 必填")
	}
	if m.BaseModelID == 0 {
		return fmt.Errorf("agent 基架模型 必填")
	}
	if m.MaxSteps <= 0 {
		m.MaxSteps = 100 // 默认总步数（= 大模型请求轮次；生图额外按引擎扣分）
	}
	// 步数耗尽策略：空=默认 fail（生成失败）；可选 partial（返回半成品）。
	if m.Exhaust == "" {
		m.Exhaust = model.ExhaustFail
	} else if m.Exhaust != model.ExhaustFail && m.Exhaust != model.ExhaustPartial {
		return fmt.Errorf("步数耗尽策略必须是 %s 或 %s", model.ExhaustFail, model.ExhaustPartial)
	}
	// Validate base model is an enabled multimodal LLM.
	base, err := s.upstream.Get(m.BaseModelID)
	if err != nil {
		return err
	}
	if base == nil || base.Type != model.TypeLLM || !base.Multimodal {
		return fmt.Errorf("基架模型必须是已启用的多模态大模型")
	}
	// Validate image models.
	clean := make([]model.ImageModelUse, 0, len(m.ImageModels))
	for _, im := range m.ImageModels {
		if im.UpstreamID == 0 {
			continue
		}
		um, err := s.upstream.Get(im.UpstreamID)
		if err != nil {
			return err
		}
		if um == nil || um.Type != model.TypeImage {
			return fmt.Errorf("图片模型 %d 不存在或不是图片模型", im.UpstreamID)
		}
		if im.Steps <= 0 {
			im.Steps = 30 // 默认：每生成一张图消耗 30 步
		}
		im.Name = um.Name
		clean = append(clean, im)
	}
	if len(clean) == 0 {
		return fmt.Errorf("至少需要配置一个图片模型(及其步数)")
	}
	m.ImageModels = clean
	// 风格模式校验：空 = 按需执行。
	if m.Style == "" {
		m.Style = model.StyleOnDemand
	} else if !validStyle(m.Style) {
		return fmt.Errorf("风格必须是 %v 之一", model.ValidStyles())
	}
	// 学习数据开关：空 = 默认启用（写死 few-shot 拼在 system 前缀）。
	if m.Learning == "" {
		m.Learning = model.LearningOn
	} else if m.Learning != model.LearningOn && m.Learning != model.LearningOff {
		return fmt.Errorf("学习数据开关必须是 %s 或 %s", model.LearningOn, model.LearningOff)
	}
	// New models default to enabled; updates may explicitly disable them.
	if m.ID == 0 && !m.Enabled {
		m.Enabled = true
	}
	return s.store.SaveAgentModel(m)
}

func validStyle(s string) bool {
	for _, v := range model.ValidStyles() {
		if v == s {
			return true
		}
	}
	return false
}

func (s *Service) Get(id int64) (*model.AgentModel, error) { return s.store.GetAgentModel(id) }
func (s *Service) GetByCall(call string) (*model.AgentModel, error) {
	return s.store.GetAgentModelByCallName(call)
}
func (s *Service) List() ([]model.AgentModel, error) { return s.store.ListAgentModels() }
func (s *Service) Delete(id int64) error             { return s.store.DeleteAgentModel(id) }

// Update applies a partial update to an existing agent model: fields left
// empty in the patch keep their stored values, so the GUI can edit a single
// field without resending the whole definition.
func (s *Service) Update(id int64, patch *model.AgentModel) (*model.AgentModel, error) {
	cur, err := s.store.GetAgentModel(id)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, fmt.Errorf("未找到 id=%d 的模型", id)
	}
	if patch.Name != "" {
		cur.Name = patch.Name
	}
	if patch.CallName != "" {
		cur.CallName = patch.CallName
	}
	if patch.BaseModelID != 0 {
		cur.BaseModelID = patch.BaseModelID
	}
	if patch.MaxSteps != 0 {
		cur.MaxSteps = patch.MaxSteps
	}
	if patch.Prompt != "" {
		cur.Prompt = patch.Prompt
	}
	if len(patch.ImageModels) > 0 {
		cur.ImageModels = patch.ImageModels
	}
	// Style: 空字符串表示未在本次请求中提交，保持原值。
	if patch.Style != "" {
		if !validStyle(patch.Style) {
			return nil, fmt.Errorf("风格必须是 %v 之一", model.ValidStyles())
		}
		cur.Style = patch.Style
	}
	if patch.Exhaust != "" {
		if patch.Exhaust != model.ExhaustFail && patch.Exhaust != model.ExhaustPartial {
			return nil, fmt.Errorf("步数耗尽策略必须是 %s 或 %s", model.ExhaustFail, model.ExhaustPartial)
		}
		cur.Exhaust = patch.Exhaust
	}
	// Learning: 空字符串表示未在本次请求中提交，保持原值（关闭需显式提交 "off"）。
	if patch.Learning != "" {
		if patch.Learning != model.LearningOn && patch.Learning != model.LearningOff {
			return nil, fmt.Errorf("学习数据开关必须是 %s 或 %s", model.LearningOn, model.LearningOff)
		}
		cur.Learning = patch.Learning
	}
	// DeepThink and Enabled are booleans that may legitimately be turned off,
	// but the GUI edit form does not send them, so a patch only *sets* them
	// to true and never disables behind the caller's back.
	cur.DeepThink = cur.DeepThink || patch.DeepThink
	if patch.Enabled {
		cur.Enabled = true
	}
	if err := s.Save(cur); err != nil {
		return nil, err
	}
	return cur, nil
}
