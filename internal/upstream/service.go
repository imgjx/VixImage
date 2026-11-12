package upstream

import (
	"fmt"
	"strings"

	"viximage/internal/model"
	"viximage/internal/storage"
)

// Service manages upstream model CRUD with the rule that ONLY multimodal-capable
// models may be registered. Image models are always treated as multimodal output;
// LLM models must declare multimodal=true or they are rejected.
type Service struct {
	store storage.Storage
}

func New(store storage.Storage) *Service { return &Service{store: store} }

// Save validates and persists an upstream model.
func (s *Service) Save(m *model.UpstreamModel) error {
	if m.Name == "" {
		return fmt.Errorf("name required")
	}
	if m.BaseURL == "" {
		return fmt.Errorf("base_url required")
	}
	if m.ModelName == "" {
		return fmt.Errorf("model_name required")
	}
	if m.Type != model.TypeImage && m.Type != model.TypeLLM {
		return fmt.Errorf("type must be 'image' or 'llm'")
	}
	if m.N <= 0 {
		m.N = 1
	}
	if m.Type == model.TypeImage {
		// image generation models always produce multimodal output
		m.Multimodal = true
		if m.Format == "" {
			m.Format = model.FormatOpenAI
		}
	}
	if m.Type == model.TypeLLM {
		if !m.Multimodal {
			return fmt.Errorf("仅允许填多模态(视觉)大模型，请将 multimodal 设为 true")
		}
		if m.Format == "" {
			m.Format = model.FormatOpenAIChat
		}
		// 小上下文模型：未配置 max_tokens 时给一个安全缺省。
		if m.MaxTokens <= 0 {
			m.MaxTokens = 2400
		}
		if m.MaxTokens > 32000 {
			return fmt.Errorf("max_tokens 过大（上限 32000）")
		}
	}
	// 多尺寸列表：清除空项；默认尺寸若不在列表里则自动补上。
	clean := make([]string, 0, len(m.Sizes))
	for _, s := range m.Sizes {
		s = strings.TrimSpace(s)
		if s != "" {
			clean = append(clean, s)
		}
	}
	if m.Size != "" {
		found := false
		for _, s := range clean {
			if s == m.Size {
				found = true
			}
		}
		if !found {
			clean = append([]string{m.Size}, clean...)
		}
	}
	m.Sizes = clean
	return s.store.SaveUpstream(m)
}

func (s *Service) Get(id int64) (*model.UpstreamModel, error)   { return s.store.GetUpstream(id) }
func (s *Service) List() ([]model.UpstreamModel, error)         { return s.store.ListUpstream() }
func (s *Service) Delete(id int64) error                        { return s.store.DeleteUpstream(id) }

// ImageModels returns enabled image-generation upstream models.
func (s *Service) ImageModels() ([]model.UpstreamModel, error) {
	all, err := s.store.ListUpstream()
	if err != nil {
		return nil, err
	}
	var out []model.UpstreamModel
	for _, m := range all {
		if m.Type == model.TypeImage && m.Enabled {
			out = append(out, m)
		}
	}
	return out, nil
}

// LLMModels returns enabled multimodal LLM upstream models.
func (s *Service) LLMModels() ([]model.UpstreamModel, error) {
	all, err := s.store.ListUpstream()
	if err != nil {
		return nil, err
	}
	var out []model.UpstreamModel
	for _, m := range all {
		if m.Type == model.TypeLLM && m.Enabled {
			out = append(out, m)
		}
	}
	return out, nil
}
