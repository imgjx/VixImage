package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Config is the root configuration for VixImage.
// It is persisted to config.json and can be edited by CLI / console / GUI.
type Config struct {
	mu sync.RWMutex `json:"-"`

	Version int `json:"version"`

	// Ports: GUI and API run on different ports.
	GUIPort int `json:"gui_port"`
	APIPort int `json:"api_port"`

	// GUI login (account / password). Password is stored hashed (bcrypt).
	GUIUser     string `json:"gui_user"`
	GUIPassHash string `json:"gui_pass_hash"`

	// API keys (apikey auth for the API service).
	APIKeys []string `json:"api_keys"`

	// Data directory for caches / sqlite / outputs / uploads.
	DataDir string `json:"data_dir"`

	// Storage: dual mode. Mode = "sqlite" | "mysql".
	StorageMode string `json:"storage_mode"`
	SQLitePath  string `json:"sqlite_path"`
	MySQLDSN    string `json:"mysql_dsn"`

	// Cache: dual mode. Mode = "file" | "redis".
	CacheMode    string `json:"cache_mode"`
	RedisAddr    string `json:"redis_addr"`
	RedisPass    string `json:"redis_pass"`
	RedisDB      int    `json:"redis_db"`
	CacheFileDir string `json:"cache_file_dir"`

	// Python interpreter used by the image-editing skill (PIL fallback).
	PythonBin string `json:"python_bin"`

	// ImageEngine selects the image-editing backend:
	//   "go"     — native Go renderer (default, no Python dependency)
	//   "python" — legacy PIL script
	ImageEngine string `json:"image_engine"`

	// DeleteOutputs 生产完成并提取后是否删除产物成品。
	// false（默认）= 成品保留在 outputs；true = 提取后连成品一起删除
	// （工作区中间产物则无论开关如何都 100% 删除）。
	DeleteOutputs bool `json:"delete_outputs"`
}

// Default returns a config populated with sensible defaults.
func Default() *Config {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	dataDir := filepath.Join(home, ".viximage")
	return &Config{
		Version:     1,
		GUIPort:     8080,
		APIPort:     8090,
		GUIUser:     "admin",
		GUIPassHash: "", // set by init
		// 注意：APIKeys 不再预置默认密钥（vix-sk-default-0001 已是过去式）。
		// 该字段仅为旧版本 config.json 中密钥的迁移入口，新版本密钥存于数据库。
		APIKeys:      nil,
		DataDir:      dataDir,
		StorageMode:  "sqlite",
		SQLitePath:   filepath.Join(dataDir, "viximage.db"),
		MySQLDSN:     "root:root@tcp(127.0.0.1:3306)/viximage?charset=utf8mb4&parseTime=true",
		CacheMode:    "file",
		RedisAddr:    "127.0.0.1:6379",
		RedisPass:    "",
		RedisDB:      0,
		CacheFileDir: filepath.Join(dataDir, "cache"),
		PythonBin:    "python3",
		ImageEngine:  "go",
	}
}

// ensureWritable 确保目录可创建且可写；不可写时回退到用户主目录下的
// .viximage（Windows 安装到 Program Files 时程序目录通常不可写，
// 静默失败会导致缓存/数据库不可用）。
func ensureWritable(dir string) string {
	if dir == "" {
		dir = Default().DataDir
	}
	if mkWritable(dir) {
		return dir
	}
	fallback := Default().DataDir
	if fallback != dir && mkWritable(fallback) {
		log.Printf("警告: 数据目录 %s 不可写，改用 %s", dir, fallback)
		return fallback
	}
	// 兜底：临时目录。
	tmp := filepath.Join(os.TempDir(), "viximage")
	if mkWritable(tmp) {
		log.Printf("警告: 数据目录 %s 不可写，改用 %s", dir, tmp)
		return tmp
	}
	return dir
}

// mkWritable 创建目录并测试能否写入。
func mkWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".vix_wtest_*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	_ = os.Remove(name)
	return true
}

// path returns the config file path given a directory.
func pathOf(dir string) string {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return filepath.Join(dir, "config.json")
}

var (
	global *Config
	path   string
)

// Load reads config from disk, falling back to defaults and merging env overrides.
// If the file does not exist it is created with defaults (and a default password).
func Load(dir string) (*Config, error) {
	p := pathOf(dir)
	path = p
	c := Default()
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, c)
	}
	// The -dir flag names the config directory; keep all runtime data inside it
	// so separate instances (tests, multi-tenant) never share one database.
	if dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		c.DataDir = filepath.Join(dir, "data")
		c.SQLitePath = filepath.Join(c.DataDir, "viximage.db")
		c.CacheFileDir = filepath.Join(c.DataDir, "cache")
	}
	// Ensure data dirs exist. 若指定目录不可写（如 Windows 装在 Program Files），
	// 回退到用户主目录，避免缓存/数据库静默不可用。
	c.DataDir = ensureWritable(c.DataDir)
	_ = os.MkdirAll(filepath.Dir(c.SQLitePath), 0o755)
	_ = os.MkdirAll(c.CacheFileDir, 0o755)

	// Seed default password if none set.
	if c.GUIPassHash == "" {
		c.GUIPassHash = hashDefault("admin123")
	}
	global = c
	return c, nil
}

// LoadFile 从显式指定的配置文件路径加载配置（路径须指向 .json 文件）。
// 与 Load(dir) 不同：不会把运行时数据收进目录子目录，DataDir 取配置里的值或默认值。
func LoadFile(configPath string) (*Config, error) {
	path = configPath
	c := Default()
	hasSQLitePath := false
	if b, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(b, c)
		var raw map[string]any
		if json.Unmarshal(b, &raw) == nil {
			_, hasSQLitePath = raw["sqlite_path"]
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// 文件里没写 data_dir 时退回默认 home 目录，避免落到二进制所在目录。
	if c.DataDir == "" {
		c.DataDir = Default().DataDir
	}
	// 目录不可写时回退到用户主目录（Windows 装在 Program Files 等场景）。
	c.DataDir = ensureWritable(c.DataDir)
	// 缓存目录跟随数据目录，保证与回退后的路径一致。
	c.CacheFileDir = filepath.Join(c.DataDir, "cache")
	if !hasSQLitePath {
		c.SQLitePath = filepath.Join(c.DataDir, "viximage.db")
	}
	_ = os.MkdirAll(c.DataDir, 0o755)
	_ = os.MkdirAll(filepath.Dir(c.SQLitePath), 0o755)
	_ = os.MkdirAll(c.CacheFileDir, 0o755)
	if c.GUIPassHash == "" {
		c.GUIPassHash = hashDefault("admin123")
	}
	global = c
	return c, nil
}

// Save persists the config to disk.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return os.WriteFile(path, b, 0o600)
}

func (c *Config) Get() *Config { return c }
