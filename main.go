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
		fmt.Printf("API Keys: %v\n", maskList(cfg.APIKeys))
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
		fmt.Println("apikey 子命令: add <key> | rm <key> | list")
		return true
	}
	switch args[0] {
	case "add":
		if len(args) < 2 {
			fmt.Println("用法: apikey add <key>")
			return true
		}
		cfg.APIKeys = append(cfg.APIKeys, args[1])
	case "rm":
		if len(args) < 2 {
			fmt.Println("用法: apikey rm <key>")
			return true
		}
		out := cfg.APIKeys[:0]
		for _, k := range cfg.APIKeys {
			if k != args[1] {
				out = append(out, k)
			}
		}
		cfg.APIKeys = out
	case "list":
		fmt.Println("API Keys:", maskList(cfg.APIKeys))
		return true
	default:
		fmt.Println("未知 apikey 子命令:", args[0])
		return true
	}
	if err := cfg.Save(); err != nil {
		fmt.Println("保存失败:", err)
		return true
	}
	fmt.Println("已更新 API Keys:", maskList(cfg.APIKeys))
	return true
}

func maskList(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if len(k) <= 8 {
			out = append(out, "******")
			continue
		}
		out = append(out, k[:4]+"****"+k[len(k)-4:])
	}
	return out
}

var _ = strings.TrimSpace
