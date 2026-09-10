package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/quizdock/quizdock/internal/api"
	"github.com/quizdock/quizdock/internal/config"
	"github.com/quizdock/quizdock/internal/database"
	"github.com/quizdock/quizdock/internal/qbank"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("QuizDock stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return serve(nil)
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "bank":
		return bankCommand(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("QuizDock %s\n", version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		return fmt.Errorf("未知命令 %q；请运行 quizdock help 查看帮助", args[0])
	}
}

func serve(args []string) error {
	defaults, err := config.DefaultDataDir()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	host := flags.String("host", "127.0.0.1", "HTTP 监听地址")
	port := flags.Int("port", 8765, "HTTP 监听端口")
	dataDir := flags.String("data-dir", defaults, "持久化数据目录")
	noBrowser := flags.Bool("no-browser", false, "启动时不打开浏览器")
	webDevURL := flags.String("web-dev-url", "", "将前端请求代理到 Vite 开发服务器")
	authUser := flags.String("auth-user", os.Getenv("QUIZDOCK_AUTH_USER"), "登录用户名；设置密码后默认为 admin")
	authPasswordFile := flags.String("auth-password-file", os.Getenv("QUIZDOCK_AUTH_PASSWORD_FILE"), "从文件读取登录密码")
	if err := flags.Parse(args); err != nil {
		return err
	}
	authPassword := os.Getenv("QUIZDOCK_AUTH_PASSWORD")
	if *authPasswordFile != "" {
		if authPassword != "" {
			return fmt.Errorf("QUIZDOCK_AUTH_PASSWORD 与 --auth-password-file 不能同时使用")
		}
		content, err := os.ReadFile(*authPasswordFile)
		if err != nil {
			return fmt.Errorf("读取登录密码文件：%w", err)
		}
		authPassword = strings.TrimRight(string(content), "\r\n")
	}
	if authPassword != "" && *authUser == "" {
		*authUser = "admin"
	}
	databasePath := filepath.Join(*dataDir, "quizdock.db")
	store, err := database.Open(databasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()
	address := net.JoinHostPort(*host, fmt.Sprintf("%d", *port))
	server := &http.Server{
		Addr: address, Handler: api.NewWithOptions(store, api.Options{
			Version: version, DataDir: *dataDir, WebDevURL: *webDevURL,
			AuthUsername: *authUser, AuthPassword: authPassword,
		}),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 10 * time.Minute,
		WriteTimeout: 10 * time.Minute, IdleTimeout: 90 * time.Second,
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s", address)
	slog.Info("QuizDock is ready", "version", version, "url", url, "database", databasePath)
	if !*noBrowser && *webDevURL == "" {
		go func() {
			time.Sleep(250 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				slog.Debug("could not open browser", "error", err)
			}
		}()
	}
	errChannel := make(chan error, 1)
	go func() { errChannel <- server.Serve(listener) }()
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signalChannel:
		slog.Info("shutting down")
		return api.ShutdownServer(server, 10*time.Second)
	case err := <-errChannel:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func bankCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("bank 命令需要指定 validate、inspect、import 或 pack")
	}
	switch args[0] {
	case "validate", "inspect":
		if len(args) != 2 {
			return fmt.Errorf("用法：quizdock bank %s 文件.qbank", args[0])
		}
		pkg, err := qbank.Load(args[1])
		if err != nil {
			return err
		}
		summary := qbank.Summary{Manifest: pkg.Manifest, Questions: len(pkg.Questions), Assets: len(pkg.Assets), ArchiveHash: pkg.ArchiveHash}
		if args[0] == "validate" {
			fmt.Printf("valid: %s %s (%d questions, %d assets)\n", pkg.Manifest.Name, pkg.Manifest.Version, len(pkg.Questions), len(pkg.Assets))
			return nil
		}
		return json.NewEncoder(os.Stdout).Encode(summary)
	case "pack":
		if len(args) != 3 {
			return fmt.Errorf("用法：quizdock bank pack 题库目录 输出文件.qbank")
		}
		summary, err := qbank.Pack(args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Printf("packed %s %s: %d questions, %d assets\n", summary.Manifest.Name, summary.Manifest.Version, summary.Questions, summary.Assets)
		return nil
	case "import":
		defaults, err := config.DefaultDataDir()
		if err != nil {
			return err
		}
		flags := flag.NewFlagSet("bank import", flag.ContinueOnError)
		dataDir := flags.String("data-dir", defaults, "持久化数据目录")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return fmt.Errorf("用法：quizdock bank import [--data-dir 目录] 文件.qbank")
		}
		pkg, err := qbank.Load(flags.Arg(0))
		if err != nil {
			return err
		}
		store, err := database.Open(filepath.Join(*dataDir, "quizdock.db"))
		if err != nil {
			return err
		}
		defer store.Close()
		result, err := store.ImportPackage(context.Background(), pkg)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	default:
		return fmt.Errorf("未知的 bank 命令 %q", args[0])
	}
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	return command.Start()
}

func printUsage() {
	fmt.Println(`QuizDock - 本地优先、支持可导入题库的刷题工具

用法：
  quizdock serve [选项]
  quizdock bank validate FILE.qbank
  quizdock bank inspect FILE.qbank
  quizdock bank pack DIRECTORY OUTPUT.qbank
  quizdock bank import [--data-dir DIR] FILE.qbank
  quizdock version`)
}
