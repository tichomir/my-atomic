package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/spf13/cobra"

	agentpkg "github.com/tichomir/my-atomic/daemon/internal/agent"
	"github.com/tichomir/my-atomic/daemon/internal/api"
	"github.com/tichomir/my-atomic/daemon/internal/audit"
	"github.com/tichomir/my-atomic/daemon/internal/policy"
)

var version = "dev"

// Config mirrors the relevant fields from /etc/atomic/config.toml
type Config struct {
	Agents struct {
		DefaultProfile  string `toml:"default_profile"`
		WorkspaceRoot   string `toml:"workspace_root"`
		APISocket       string `toml:"api_socket"`
		MaxConcurrent   int    `toml:"max_concurrent_agents"`
	} `toml:"agents"`
	Audit struct {
		JournalEnabled    bool   `toml:"journal_enabled"`
		JournalIdentifier string `toml:"journal_identifier"`
		FileEnabled       bool   `toml:"file_enabled"`
		FilePath          string `toml:"file_path"`
	} `toml:"audit"`
	Policy struct {
		PolicyDir        string `toml:"policy_dir"`
		BuiltinPolicyDir string `toml:"builtin_policy_dir"`
	} `toml:"policy"`
	Runtime struct {
		FalcoEnabled        bool   `toml:"falco_enabled"`
		FalcoWebhookURL     string `toml:"falco_webhook_url"`
		AutoKillOnCritical  bool   `toml:"auto_kill_on_critical"`
	} `toml:"runtime"`
}

func main() {
	var configPath string

	root := &cobra.Command{
		Use:     "atomicagentd",
		Short:   "Atomic Agent Daemon - AI agent safety runtime for Agentic OS",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(configPath)
		},
	}

	root.Flags().StringVar(&configPath, "config", "/etc/atomic/config.toml", "Path to config file")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("atomicagentd starting", "version", version)

	// Load configuration
	var cfg Config
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		return fmt.Errorf("loading config %s: %w", configPath, err)
	}

	// Apply defaults
	if cfg.Agents.WorkspaceRoot == "" {
		cfg.Agents.WorkspaceRoot = "/var/lib/atomic/agents"
	}
	if cfg.Agents.APISocket == "" {
		cfg.Agents.APISocket = "/run/atomic/api.sock"
	}
	if cfg.Agents.DefaultProfile == "" {
		cfg.Agents.DefaultProfile = "minimal"
	}
	if cfg.Policy.BuiltinPolicyDir == "" {
		cfg.Policy.BuiltinPolicyDir = "/usr/share/atomic/policy"
	}

	// Initialize audit logger
	auditor, err := audit.NewLogger(audit.Config{
		JournalEnabled:    cfg.Audit.JournalEnabled,
		JournalIdentifier: cfg.Audit.JournalIdentifier,
		FileEnabled:       cfg.Audit.FileEnabled,
		FilePath:          cfg.Audit.FilePath,
	})
	if err != nil {
		return fmt.Errorf("initializing audit logger: %w", err)
	}
	defer auditor.Close()

	// Initialize policy engine
	policyEng, err := policy.NewEngine(cfg.Policy.BuiltinPolicyDir, cfg.Policy.PolicyDir)
	if err != nil {
		return fmt.Errorf("initializing policy engine: %w", err)
	}
	logger.Info("policy engine initialized",
		"builtin_dir", cfg.Policy.BuiltinPolicyDir,
		"override_dir", cfg.Policy.PolicyDir,
	)

	// Initialize sandbox manager
	sandbox, err := agentpkg.NewSandboxManager(cfg.Agents.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("initializing sandbox manager: %w", err)
	}
	defer sandbox.Close()

	// Initialize agent manager
	mgr := agentpkg.NewManager(agentpkg.ManagerConfig{
		WorkspaceRoot: cfg.Agents.WorkspaceRoot,
		MaxAgents:     cfg.Agents.MaxConcurrent,
	}, sandbox, policyEng, auditor)

	// Initialize API server
	server := api.NewServer(cfg.Agents.APISocket, mgr, policyEng, auditor)
	if err := server.Start(); err != nil {
		return fmt.Errorf("starting API server: %w", err)
	}

	// Log startup event
	startEvent := audit.NewEvent(audit.EventDaemonStarted, audit.SeverityInfo,
		fmt.Sprintf("atomicagentd %s started", version))
	auditor.Log(startEvent)

	// Notify systemd that we are ready
	daemon.SdNotify(false, daemon.SdNotifyReady)
	logger.Info("atomicagentd ready", "socket", cfg.Agents.APISocket)

	// Wait for shutdown signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	for {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP:
			// Reload policies on SIGHUP
			logger.Info("received SIGHUP, reloading policies")
			if err := policyEng.Reload(); err != nil {
				logger.Error("policy reload failed", "error", err)
			} else {
				logger.Info("policies reloaded successfully")
				e := audit.NewEvent(audit.EventPolicyReloaded, audit.SeverityInfo, "policies reloaded via SIGHUP")
				auditor.Log(e)
			}
		case syscall.SIGTERM, syscall.SIGINT:
			logger.Info("received shutdown signal", "signal", sig)
			daemon.SdNotify(false, daemon.SdNotifyStopping)

			shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
			defer shutdownCancel()

			if err := server.Shutdown(shutdownCtx); err != nil {
				logger.Error("API server shutdown error", "error", err)
			}

			shutdownEvent := audit.NewEvent(audit.EventDaemonShutdown, audit.SeverityInfo, "atomicagentd shutting down")
			auditor.Log(shutdownEvent)

			return nil
		}
	}
}
