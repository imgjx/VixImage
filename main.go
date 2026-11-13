package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"viximage/internal/agent"
	"viximage/internal/agentmodel"
	"viximage/internal/api"
	"viximage/internal/cache"
	"viximage/internal/config"
	"viximage/internal/gui"
	"viximage/internal/model"
	"viximage/internal/storage"
	"viximage/internal/upstream"
)

// Version 由构建时通过 -ldflags "-X main.Version=..." 注入（发布版为语义化版本号）。
var Version = "dev"

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("[VixImage] ")

	dir := flag.String("dir", ".", "配置/数据目录（config.json 所在目录）")
	configFile := flag.String("config", "", "配置文件路径（显式指定 config.json，覆盖 -dir）")
	guiPort := flag.Int("gui-port", 0, "GUI 端口（覆盖配置；0=用配置值）")
	apiPort := flag.Int("api-port", 0, "API 端口（覆盖配置；0=用配置值）")
	showVersion := flag.Bool("version", false, "打印版本号并退出")
	flag.Parse()
	if *showVersion {
		fmt.Println("VixImage", Version)
		return
	}
	args := flag.Args()

	var cfg *config.Config
	var err error
	if *configFile != "" {
		cfg, err = config.LoadFile(*configFile)
	} else {
		cfg, err = config.Load(*dir)
	}
	if err != nil {
		log.Println("加载配置失败:", err)
		os.Exit(1)
	}
	// CLI 覆盖端口（不写回配置）。
	if *guiPort != 0 {
		cfg.GUIPort = *guiPort
	}
	if *apiPort != 0 {
		cfg.APIPort = *apiPort
	}
	for _, d := range []string{cfg.DataDir, filepath.Join(cfg.DataDir, "outputs"), filepath.Join(cfg.DataDir, "uploads"), filepath.Join(cfg.DataDir, "work")} {
		_ = os.MkdirAll(d, 0o755)
	}

	// CLI management commands (no servers).
	if len(args) > 0 {
		if handled := runCLI(cfg, args); handled {
			return
		}
	}

	// Boot servers.
	store, err := storage.Open(cfg)
	if err != nil {
		fmt.Println("存储初始化失败:", err)
		os.Exit(1)
	}
	if err := store.Init(); err != nil {
		fmt.Println("存储建表失败:", err)
		os.Exit(1)
	}
	cch := cache.Open(cfg)
	up := upstream.New(store)
	ag := agentmodel.New(store, up)
	rt := agent.New(cfg, store, up, cch)

	go func() {
		log.Printf("GUI  控制台 启动: http://localhost:%d  (账号 %s)\n", cfg.GUIPort, cfg.GUIUser)
		s := gui.New(cfg, store, up, ag, rt, cch)
		if err := s.Start(); err != nil {
			log.Println("GUI 启动失败:", err)
		}
	}()
	go func() {
		log.Printf("API  服务   启动: http://localhost:%d  (apikey 鉴权)\n", cfg.APIPort)
		s := api.New(cfg, store, up, rt, cch)
		if err := s.Start(); err != nil {
			log.Println("API 启动失败:", err)
		}
	}()

	log.Println("VixImage 已启动。按 Ctrl+C 退出。")
	select {}
}

// runCLI handles management subcommands. Returns true if a command was handled.
func runCLI(cfg *config.Config, args []string) bool {
	switch args[0] {
	case "user":
		return cliUser(cfg, args[1:])
	case "apikey":
		return cliAPIKey(cfg, args[1:])
	case "config":
		fmt.Printf("GUI  端口: %d\nAPI  端口: %d\n存储模式: %s\n缓存模式: %s\nGUI 账号: %s\n",
			cfg.GUIPort, cfg.APIPort, cfg.StorageMode, cfg.CacheMode, cfg.GUIUser)
		fmt.Println("API 密钥: 请使用 `viximage apikey list` 查看（密钥存储于数据库并统计调用次数）")
		return true
	case "serve":
		return false // fall through to boot servers
	default:
		fmt.Println("未知命令:", args[0])
		fmt.Println("用法: viximage [serve] | user set --username X --password Y | apikey add/list/rm | config")
		return true
	}
}

func cliUser(cfg *config.Config, args []string) bool {
	fs := flag.NewFlagSet("user", flag.ContinueOnError)
	op := fs.String("op", "set", "操作: set")
	user := fs.String("username", "", "新账号")
	pass := fs.String("password", "", "新密码")
	// Go 的 flag 在遇到第一个位置参数就停止解析，因此先剥离 "set" 这类子命令。
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		*op = args[0]
		args = args[1:]
	}
	_ = fs.Parse(args)
	if *op == "set" {
		if *user != "" {
			cfg.GUIUser = *user
		}
		if *pass != "" {
			h, err := config.HashPassword(*pass)
			if err != nil {
				fmt.Println("错误:", err)
				return true
			}
			cfg.GUIPassHash = h
		}
		if err := cfg.Save(); err != nil {
			fmt.Println("保存失败:", err)
			return true
		}
		fmt.Printf("已更新 GUI 账号 -> 账号:%s 密码已设置\n", cfg.GUIUser)
	}
	return true
}

func cliAPIKey(cfg *config.Config, args []string) bool {
	if len(args) == 0 {
		fmt.Println("apikey 子命令: list | add [key] | rm <id|key>")
		return true
	}
	store, err := storage.Open(cfg)
	if err != nil {
		fmt.Println("存储打开失败:", err)
		return true
	}
	if err := store.Init(); err != nil {
		fmt.Println("存储初始化失败:", err)
		return true
	}
	defer store.Close()
	switch args[0] {
	case "list":
		keys, err := store.ListAPIKeys()
		if err != nil {
			fmt.Println("查询失败:", err)
			return true
		}
		if len(keys) == 0 {
			fmt.Println("（无密钥。提示：默认不创建密钥，请先 `viximage apikey add` 新建一个）")
			return true
		}
		fmt.Printf("%-4s %-20s %-12s %6s %s\n", "ID", "密钥", "备注", "调用", "最近调用")
		for _, k := range keys {
			last := k.LastUsedAt
			if last == "" {
				last = "—"
			}
			fmt.Printf("%-4d %-20s %-12s %6d %s\n", k.ID, model.MaskKey(k.Key), clip(k.Name, 12), k.Calls, last)
		}
	case "add", "gen":
		var key string
		if args[0] == "add" && len(args) >= 2 {
			key = args[1]
			if ex, _ := store.GetAPIKey(key); ex != nil {
				fmt.Println("错误: 该密钥已存在")
				return true
			}
		}
		mk := &model.APIKey{Key: key, Name: ""}
		if err := store.SaveAPIKey(mk); err != nil {
			fmt.Println("创建失败:", err)
			return true
		}
		fmt.Printf("已创建密钥，完整密钥（仅此一次，请妥善保存）：\n%s\n", mk.Key)
	case "rm":
		if len(args) < 2 {
			fmt.Println("用法: apikey rm <id|key>")
			return true
		}
		id, name := resolveKeyID(store, args[1])
		if id <= 0 {
			fmt.Println("未找到该密钥:", args[1])
			return true
		}
		if err := store.DeleteAPIKey(id); err != nil {
			fmt.Println("删除失败:", err)
			return true
		}
		fmt.Printf("已删除密钥 %s（id=%d）\n", name, id)
	default:
		fmt.Println("未知 apikey 子命令:", args[0])
		return true
	}
	return true
}

// keyLookup 是 CLI 解析密钥时只依赖的最小接口（storage.Storage 天然满足）。
type keyLookup interface {
	GetAPIKey(key string) (*model.APIKey, error)
}

// resolveKeyID 把 CLI 传入的 id 或完整密钥解析为库内 id；返回 0 表示未找到。
func resolveKeyID(store keyLookup, s string) (int64, string) {
	if id := atoi64Safe(s); id > 0 {
		return id, "#" + s
	}
	if k, err := store.GetAPIKey(s); err == nil && k != nil {
		return k.ID, model.MaskKey(k.Key)
	}
	return 0, ""
}

func atoi64Safe(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var _ = strings.TrimSpace
