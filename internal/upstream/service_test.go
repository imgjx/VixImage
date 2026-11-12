package upstream

import (
	"testing"

	"viximage/internal/model"
)

// fakeStore 是一个最小的 storage.Storage 实现（只实现上游模型相关方法）。
type fakeStore struct {
	saved model.UpstreamModel
}

func (f *fakeStore) Init() error    { return nil }
func (f *fakeStore) Close() error   { return nil }
func (f *fakeStore) SaveUpstream(m *model.UpstreamModel) error {
	f.saved = *m
	m.ID = 1
	return nil
}
func (f *fakeStore) GetUpstream(id int64) (*model.UpstreamModel, error) { return nil, nil }
func (f *fakeStore) ListUpstream() ([]model.UpstreamModel, error)       { return nil, nil }
func (f *fakeStore) DeleteUpstream(id int64) error                      { return nil }
func (f *fakeStore) SaveAgentModel(m *model.AgentModel) error           { return nil }
func (f *fakeStore) GetAgentModel(id int64) (*model.AgentModel, error)   { return nil, nil }
func (f *fakeStore) GetAgentModelByCallName(call string) (*model.AgentModel, error) {
	return nil, nil
}
func (f *fakeStore) ListAgentModels() ([]model.AgentModel, error) { return nil, nil }
func (f *fakeStore) DeleteAgentModel(id int64) error             { return nil }
func (f *fakeStore) AddCallLog(l *model.CallLog) error           { return nil }
func (f *fakeStore) UpdateCallLog(l *model.CallLog) error        { return nil }
func (f *fakeStore) ListCallLogs(limit int) ([]model.CallLog, error) {
	return nil, nil
}
func (f *fakeStore) ListCallLogsPaged(offset, limit int) ([]model.CallLog, int64, error) {
	return nil, 0, nil
}
func (f *fakeStore) CallStats() (*model.CallStats, error) { return nil, nil }

// 图片模型：不该带 LLM 专属字段（max_tokens / temperature / top_p / system_prompt）。
func TestImageModelDropsLLMFields(t *testing.T) {
	f := &fakeStore{}
	s := New(f)
	m := &model.UpstreamModel{
		Name: "img", Type: model.TypeImage, BaseURL: "https://x", ModelName: "m1",
		MaxTokens: 4096, Temperature: 0.9, TopP: 0.8, SystemPrompt: "xx",
		Size: "1024x1024", Sizes: []string{"1024x1024"}, N: 2, Steps: 25,
	}
	if err := s.Save(m); err != nil {
		t.Fatal(err)
	}
	if m.MaxTokens != 0 || m.Temperature != 0 || m.TopP != 0 || m.SystemPrompt != "" {
		t.Fatalf("图片模型仍带 LLM 字段: %+v", m)
	}
	if m.Size != "1024x1024" || m.N != 2 || m.Steps != 25 {
		t.Fatalf("图片模型专属字段被误清: %+v", m)
	}
}

// LLM：不该带图片模型专属字段（尺寸 / 张数 / 步数 / 反向提示词 / 引擎介绍）。
func TestLLMModelDropsImageFields(t *testing.T) {
	f := &fakeStore{}
	s := New(f)
	m := &model.UpstreamModel{
		Name: "llm", Type: model.TypeLLM, BaseURL: "https://x", ModelName: "m2",
		Multimodal: true, Size: "1024x1024", Sizes: []string{"1024x1024"},
		N: 3, Steps: 40, NegativePrompt: "blur", AgentDesc: "擅长人像",
	}
	if err := s.Save(m); err != nil {
		t.Fatal(err)
	}
	if m.Size != "" || len(m.Sizes) != 0 || m.N != 0 || m.Steps != 0 ||
		m.NegativePrompt != "" || m.AgentDesc != "" {
		t.Fatalf("LLM 仍带图片字段: %+v", m)
	}
	if m.MaxTokens <= 0 {
		t.Fatalf("LLM 应有 max_tokens 缺省: %+v", m)
	}
}

// LLM 未勾选多模态 → 拒绝。
func TestLLMRequiresMultimodal(t *testing.T) {
	s := New(&fakeStore{})
	m := &model.UpstreamModel{Name: "llm", Type: model.TypeLLM, BaseURL: "https://x", ModelName: "m"}
	if err := s.Save(m); err == nil {
		t.Fatal("非多模态 LLM 应被拒绝")
	}
}
