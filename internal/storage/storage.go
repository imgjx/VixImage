package storage

import (
	"database/sql"

	"viximage/internal/config"
	"viximage/internal/model"
)

// Storage is the persistence abstraction. Two implementations are provided:
// SQLite (default, zero-config) and MySQL. The active one is selected by
// config.StorageMode.
type Storage interface {
	Init() error
	Close() error

	// Upstream models
	SaveUpstream(m *model.UpstreamModel) error
	GetUpstream(id int64) (*model.UpstreamModel, error)
	ListUpstream() ([]model.UpstreamModel, error)
	DeleteUpstream(id int64) error

	// Agent (my) models
	SaveAgentModel(m *model.AgentModel) error
	GetAgentModel(id int64) (*model.AgentModel, error)
	GetAgentModelByCallName(call string) (*model.AgentModel, error)
	ListAgentModels() ([]model.AgentModel, error)
	DeleteAgentModel(id int64) error

	// Call logs / 调用概述
	AddCallLog(l *model.CallLog) error
	UpdateCallLog(l *model.CallLog) error
	ListCallLogs(limit int) ([]model.CallLog, error)
	// 分页（仪表盘列表）：返回当前页记录与总条数。
	ListCallLogsPaged(offset, limit int) ([]model.CallLog, int64, error)
	CallStats() (*model.CallStats, error)
}

// Open returns the configured storage backend.
func Open(cfg *config.Config) (Storage, error) {
	if cfg.StorageMode == "mysql" {
		s := &sqlStore{dialect: "mysql", cfg: cfg}
		if err := s.connect(cfg.MySQLDSN); err != nil {
			return nil, err
		}
		return s, nil
	}
	s := &sqlStore{dialect: "sqlite", cfg: cfg}
	if err := s.connect(cfg.SQLitePath); err != nil {
		return nil, err
	}
	return s, nil
}

// sqlStore is the shared implementation for both SQLite and MySQL.
type sqlStore struct {
	dialect string
	cfg     *config.Config
	db      *sql.DB
}
