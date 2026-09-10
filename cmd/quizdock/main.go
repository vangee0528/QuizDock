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
		return fmt.Errorf("unknown command %q; run quizdock help", args[0])
	}
}

func serve(args []string) error {
	defaults, err := config.DefaultDataDir()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	host := flags.String("host", "127.0.0.1", "HTTP listen address")
	port := flags.Int("port", 8765, "HTTP listen port")
	dataDir := flags.String("data-dir", defaults, "persistent data directory")
	noBrowser := flags.Bool("no-browser", false, "do not open the browser")
	webDevURL := flags.String("web-dev-url", "", "proxy frontend requests to a Vite development server")
	if err := flags.Parse(args); err != nil {
		return err
	}
	databasePath := filepath.Join(*dataDir, "quizdock.db")
	store, err := database.Open(databasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()
	address := net.JoinHostPort(*host, fmt.Sprintf("%d", *port))
	server := &http.Server{
		Addr: address, Handler: api.New(store, version, *dataDir, *webDevURL),
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
		return fmt.Errorf("bank command requires validate, inspect, import or pack")
	}
	switch args[0] {
	case "validate", "inspect":
		if len(args) != 2 {
			return fmt.Errorf("usage: quizdock bank %s FILE.qbank", args[0])
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
			return fmt.Errorf("usage: quizdock bank pack DIRECTORY OUTPUT.qbank")
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
		dataDir := flags.String("data-dir", defaults, "persistent data directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return fmt.Errorf("usage: quizdock bank import [--data-dir DIR] FILE.qbank")
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
		return fmt.Errorf("unknown bank command %q", args[0])
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
	fmt.Println(`QuizDock - local-first practice with importable question banks

Usage:
  quizdock serve [options]
  quizdock bank validate FILE.qbank
  quizdock bank inspect FILE.qbank
  quizdock bank pack DIRECTORY OUTPUT.qbank
  quizdock bank import [--data-dir DIR] FILE.qbank
  quizdock version`)
}
