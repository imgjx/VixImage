package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"viximage/internal/model"
)

func (s *sqlStore) connect(dsn string) error {
	var driver string
	switch s.dialect {
	case "mysql":
		driver = "mysql"
	default:
		driver = "sqlite"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		return fmt.Errorf("connect %s: %w", driver, err)
	}
	db.SetMaxOpenConns(8)
	s.db = db
	return nil
}

func (s *sqlStore) Close() error { return s.db.Close() }

// ai returns the auto-increment primary key clause for the dialect.
func (s *sqlStore) pk() string {
	if s.dialect == "mysql" {
		return "INT AUTO_INCREMENT PRIMARY KEY"
	}
	return "INTEGER PRIMARY KEY AUTOINCREMENT"
}

func (s *sqlStore) Init() error {
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS upstream_models (
			id %s, name TEXT, type TEXT, format TEXT, base_url TEXT, api_key TEXT,
			model_name TEXT, size TEXT, sizes TEXT, n INT, multimodal BOOLEAN,
			agent_desc TEXT, max_tokens INT DEFAULT 0, enabled BOOLEAN,
			steps INT DEFAULT 0, negative_prompt TEXT,
			temperature REAL DEFAULT 0, top_p REAL DEFAULT 0, system_prompt TEXT,
			created_at TEXT, updated_at TEXT)`, s.pk()),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS agent_models (
			id %s, name TEXT, call_name TEXT UNIQUE, base_model_id INT, max_steps INT,
			image_models TEXT, prompt TEXT, style TEXT, deep_think BOOLEAN DEFAULT 0, exhaust TEXT, learning TEXT, enabled BOOLEAN,
			created_at TEXT, updated_at TEXT)`, s.pk()),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS call_logs (
			id %s, api_key_mask TEXT, agent_call_name TEXT, prompt TEXT, input_images INT,
			output_images INT, status TEXT, steps_used INT, base_model_used TEXT,
			image_models_used TEXT, duration_ms INT, error TEXT,
			deep_think BOOLEAN DEFAULT 0, reasoning TEXT, review TEXT, theme TEXT,
			style TEXT, workspace TEXT, llm_calls INT DEFAULT 0, trace TEXT,
			created_at TEXT, finished_at TEXT)`, s.pk()),
	}
	for _, st := range stmts {
		if _, err := s.db.Exec(st); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}
	// Lightweight migrations for tables created by older versions.
	s.migrateColumn("upstream_models", "sizes", "TEXT")
	s.migrateColumn("upstream_models", "agent_desc", "TEXT")
	s.migrateColumn("upstream_models", "max_tokens", "INT DEFAULT 0")
	s.migrateColumn("upstream_models", "steps", "INT DEFAULT 0")
	s.migrateColumn("upstream_models", "negative_prompt", "TEXT")
	s.migrateColumn("upstream_models", "temperature", "REAL DEFAULT 0")
	s.migrateColumn("upstream_models", "top_p", "REAL DEFAULT 0")
	s.migrateColumn("upstream_models", "system_prompt", "TEXT")
	s.migrateColumn("agent_models", "style", "TEXT")
	s.migrateColumn("agent_models", "deep_think", "BOOLEAN DEFAULT 0")
	s.migrateColumn("agent_models", "exhaust", "TEXT")
	s.migrateColumn("agent_models", "learning", "TEXT")
	s.migrateColumn("call_logs", "deep_think", "BOOLEAN DEFAULT 0")
	s.migrateColumn("call_logs", "reasoning", "TEXT")
	s.migrateColumn("call_logs", "review", "TEXT")
	s.migrateColumn("call_logs", "theme", "TEXT")
	s.migrateColumn("call_logs", "style", "TEXT")
	s.migrateColumn("call_logs", "workspace", "TEXT")
	s.migrateColumn("call_logs", "llm_calls", "INT DEFAULT 0")
	s.migrateColumn("call_logs", "trace", "TEXT")
	return nil
}

// migrateColumn adds a column when it is missing (ignores "already exists").
func (s *sqlStore) migrateColumn(table, column, typ string) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err == nil {
		found := false
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull int
			var dflt any
			var pk int
			if e := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); e == nil && name == column {
				found = true
			}
		}
		rows.Close()
		if found {
			return
		}
	}
	_, _ = s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, typ))
}

// ---------- Upstream models ----------

func (s *sqlStore) SaveUpstream(m *model.UpstreamModel) error {
	now := time.Now().UTC().Format(time.RFC3339)
	sizesJSON, _ := json.Marshal(m.Sizes)
	if m.ID == 0 {
		m.CreatedAt = time.Now().UTC()
		m.UpdatedAt = m.CreatedAt
		res, err := s.db.Exec(`INSERT INTO upstream_models
			(name,type,format,base_url,api_key,model_name,size,sizes,n,multimodal,agent_desc,max_tokens,enabled,steps,negative_prompt,temperature,top_p,system_prompt,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			m.Name, string(m.Type), string(m.Format), m.BaseURL, m.APIKey, m.ModelName, m.Size, string(sizesJSON), m.N, m.Multimodal, m.AgentDesc, m.MaxTokens, m.Enabled, m.Steps, m.NegativePrompt, m.Temperature, m.TopP, m.SystemPrompt, now, now)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		m.ID = id
		return nil
	}
	m.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(`UPDATE upstream_models SET name=?,type=?,format=?,base_url=?,api_key=?,
		model_name=?,size=?,sizes=?,n=?,multimodal=?,agent_desc=?,max_tokens=?,enabled=?,steps=?,negative_prompt=?,temperature=?,top_p=?,system_prompt=?,updated_at=? WHERE id=?`,
		m.Name, string(m.Type), string(m.Format), m.BaseURL, m.APIKey, m.ModelName, m.Size, string(sizesJSON), m.N, m.Multimodal, m.AgentDesc, m.MaxTokens, m.Enabled, m.Steps, m.NegativePrompt, m.Temperature, m.TopP, m.SystemPrompt, now, m.ID)
	return err
}

const upstreamCols = `id,name,type,format,base_url,api_key,model_name,size,sizes,n,multimodal,agent_desc,max_tokens,enabled,steps,negative_prompt,temperature,top_p,system_prompt,created_at,updated_at`

func (s *sqlStore) GetUpstream(id int64) (*model.UpstreamModel, error) {
	rows, err := s.db.Query(`SELECT `+upstreamCols+` FROM upstream_models WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanUpstream(rows)
}

func (s *sqlStore) ListUpstream() ([]model.UpstreamModel, error) {
	rows, err := s.db.Query(`SELECT ` + upstreamCols + ` FROM upstream_models ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.UpstreamModel
	for rows.Next() {
		m, err := scanUpstream(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, nil
}

func (s *sqlStore) DeleteUpstream(id int64) error {
	_, err := s.db.Exec(`DELETE FROM upstream_models WHERE id=?`, id)
	return err
}

func scanUpstream(rows *sql.Rows) (*model.UpstreamModel, error) {
	var m model.UpstreamModel
	var t, f, c, u, sizesJSON string
	var multimodal, enabled bool
	if err := rows.Scan(&m.ID, &m.Name, &t, &f, &m.BaseURL, &m.APIKey, &m.ModelName, &m.Size, &sizesJSON, &m.N, &multimodal, &m.AgentDesc, &m.MaxTokens, &enabled, &m.Steps, &m.NegativePrompt, &m.Temperature, &m.TopP, &m.SystemPrompt, &c, &u); err != nil {
		return nil, err
	}
	m.Type = model.ModelType(t)
	m.Format = model.APIFormat(f)
	m.Multimodal = multimodal
	m.Enabled = enabled
	_ = json.Unmarshal([]byte(sizesJSON), &m.Sizes)
	m.CreatedAt, _ = time.Parse(time.RFC3339, c)
	m.UpdatedAt, _ = time.Parse(time.RFC3339, u)
	return &m, nil
}

// ---------- Agent (my) models ----------

func (s *sqlStore) SaveAgentModel(m *model.AgentModel) error {
	now := time.Now().UTC().Format(time.RFC3339)
	imj, _ := json.Marshal(m.ImageModels)
	if m.ID == 0 {
		m.CreatedAt = time.Now().UTC()
		m.UpdatedAt = m.CreatedAt
		res, err := s.db.Exec(`INSERT INTO agent_models
			(name,call_name,base_model_id,max_steps,image_models,prompt,style,deep_think,exhaust,learning,enabled,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			m.Name, m.CallName, m.BaseModelID, m.MaxSteps, string(imj), m.Prompt, m.Style, m.DeepThink, m.Exhaust, m.Learning, m.Enabled, now, now)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		m.ID = id
		return nil
	}
	m.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(`UPDATE agent_models SET name=?,call_name=?,base_model_id=?,max_steps=?,
		image_models=?,prompt=?,style=?,deep_think=?,exhaust=?,learning=?,enabled=?,updated_at=? WHERE id=?`,
		m.Name, m.CallName, m.BaseModelID, m.MaxSteps, string(imj), m.Prompt, m.Style, m.DeepThink, m.Exhaust, m.Learning, m.Enabled, now, m.ID)
	return err
}

const agentCols = `id,name,call_name,base_model_id,max_steps,image_models,prompt,style,deep_think,exhaust,learning,enabled,created_at,updated_at`

func (s *sqlStore) GetAgentModel(id int64) (*model.AgentModel, error) {
	return s.getAgent(`WHERE id=?`, id)
}
func (s *sqlStore) GetAgentModelByCallName(call string) (*model.AgentModel, error) {
	return s.getAgent(`WHERE call_name=?`, call)
}
func (s *sqlStore) getAgent(where string, arg any) (*model.AgentModel, error) {
	rows, err := s.db.Query(`SELECT `+agentCols+` FROM agent_models `+where, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var m model.AgentModel
	var imj string
	var c, u string
	var enabled, deep bool
	if err := rows.Scan(&m.ID, &m.Name, &m.CallName, &m.BaseModelID, &m.MaxSteps, &imj, &m.Prompt, &m.Style, &deep, &m.Exhaust, &m.Learning, &enabled, &c, &u); err != nil {
		return nil, err
	}
	m.Enabled = enabled
	m.DeepThink = deep
	_ = json.Unmarshal([]byte(imj), &m.ImageModels)
	m.CreatedAt, _ = time.Parse(time.RFC3339, c)
	m.UpdatedAt, _ = time.Parse(time.RFC3339, u)
	return &m, nil
}

func (s *sqlStore) ListAgentModels() ([]model.AgentModel, error) {
	rows, err := s.db.Query(`SELECT ` + agentCols + ` FROM agent_models ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.AgentModel
	for rows.Next() {
		var m model.AgentModel
		var imj string
		var c, u string
		var enabled, deep bool
		if err := rows.Scan(&m.ID, &m.Name, &m.CallName, &m.BaseModelID, &m.MaxSteps, &imj, &m.Prompt, &m.Style, &deep, &m.Exhaust, &m.Learning, &enabled, &c, &u); err != nil {
			return nil, err
		}
		m.Enabled = enabled
		m.DeepThink = deep
		_ = json.Unmarshal([]byte(imj), &m.ImageModels)
		m.CreatedAt, _ = time.Parse(time.RFC3339, c)
		m.UpdatedAt, _ = time.Parse(time.RFC3339, u)
		out = append(out, m)
	}
	return out, nil
}

func (s *sqlStore) DeleteAgentModel(id int64) error {
	_, err := s.db.Exec(`DELETE FROM agent_models WHERE id=?`, id)
	return err
}

// ---------- Call logs ----------

const callLogCols = `id,api_key_mask,agent_call_name,prompt,input_images,output_images,status,steps_used,base_model_used,image_models_used,duration_ms,error,deep_think,reasoning,review,theme,style,workspace,llm_calls,trace,created_at,finished_at`

func (s *sqlStore) AddCallLog(l *model.CallLog) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO call_logs
		(api_key_mask,agent_call_name,prompt,input_images,output_images,status,steps_used,base_model_used,image_models_used,duration_ms,error,style,workspace,llm_calls,trace,created_at,finished_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.APIKeyMask, l.AgentCallName, l.Prompt, l.InputImages, l.OutputImages, l.Status, l.StepsUsed,
		l.BaseModelUsed, l.ImageModelsUsed, l.DurationMs, l.Error, l.Style, l.Workspace, l.LLMCalls, l.Trace, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	l.ID = id
	return nil
}

func (s *sqlStore) UpdateCallLog(l *model.CallLog) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE call_logs SET status=?,output_images=?,steps_used=?,base_model_used=?,
		image_models_used=?,duration_ms=?,error=?,deep_think=?,reasoning=?,review=?,theme=?,style=?,workspace=?,llm_calls=?,trace=?,finished_at=? WHERE id=?`,
		l.Status, l.OutputImages, l.StepsUsed, l.BaseModelUsed, l.ImageModelsUsed, l.DurationMs, l.Error,
		l.DeepThink, l.Reasoning, l.Review, l.Theme, l.Style, l.Workspace, l.LLMCalls, l.Trace, now, l.ID)
	return err
}

func scanCallLog(rows *sql.Rows) (model.CallLog, error) {
	var l model.CallLog
	var c, u string
	var deep bool
	if err := rows.Scan(&l.ID, &l.APIKeyMask, &l.AgentCallName, &l.Prompt, &l.InputImages, &l.OutputImages, &l.Status, &l.StepsUsed, &l.BaseModelUsed, &l.ImageModelsUsed, &l.DurationMs, &l.Error, &deep, &l.Reasoning, &l.Review, &l.Theme, &l.Style, &l.Workspace, &l.LLMCalls, &l.Trace, &c, &u); err != nil {
		return l, err
	}
	l.DeepThink = deep
	l.CreatedAt, _ = time.Parse(time.RFC3339, c)
	l.FinishedAt, _ = time.Parse(time.RFC3339, u)
	return l, nil
}

func (s *sqlStore) ListCallLogs(limit int) ([]model.CallLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+callLogCols+` FROM call_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CallLog
	for rows.Next() {
		l, err := scanCallLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

// ListCallLogsPaged 返回当前页记录与满足条件的总条数（供仪表盘分页）。
func (s *sqlStore) ListCallLogsPaged(offset, limit int) ([]model.CallLog, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM call_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT `+callLogCols+` FROM call_logs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.CallLog
	for rows.Next() {
		l, err := scanCallLog(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, nil
}

func (s *sqlStore) CallStats() (*model.CallStats, error) {
	var st model.CallStats
	row := s.db.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN status='success' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END), SUM(CASE WHEN status='running' THEN 1 ELSE 0 END),
		SUM(output_images), SUM(steps_used) FROM call_logs`)
	var succ, fail, run, outs, steps sql.NullInt64
	if err := row.Scan(&st.Total, &succ, &fail, &run, &outs, &steps); err != nil {
		return nil, err
	}
	st.Success = nullToInt(succ)
	st.Failed = nullToInt(fail)
	st.Running = nullToInt(run)
	st.Outputs = nullToInt(outs)
	st.StepsUsed = nullToInt(steps)
	return &st, nil
}

func nullToInt(n sql.NullInt64) int64 {
	if n.Valid {
		return n.Int64
	}
	return 0
}

// ensure strings import used for potential future helpers
var _ = strings.TrimSpace
