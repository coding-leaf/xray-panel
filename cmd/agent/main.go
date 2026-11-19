package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"panel/internal/adapter/xray"
	"panel/internal/agent"
	"panel/internal/pkg/logger"
)

// Version can be overridden at build time via -ldflags "-X 'main.Version=...'".
var Version = "v2.7.1"

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}

func main() {
	var (
		masterURL   = flag.String("master-url", envOrDefault("MASTER_URL", ""), "Master panel base URL (e.g. https://panel.example.com)")
		token       = flag.String("token", envOrDefault("AGENT_TOKEN", ""), "Node Bearer token")
		xrayBin     = flag.String("xray-bin", envOrDefault("XRAY_BIN", "xray"), "Path to xray binary")
		xrayConfig  = flag.String("xray-config", envOrDefault("XRAY_CONFIG", "/usr/local/etc/xray/config.json"), "Path to xray config.json")
		xrayGRPC    = flag.String("xray-grpc", envOrDefault("XRAY_GRPC", "127.0.0.1:8080"), "Local xray gRPC API address (must match remote compiled Xray API inbound 127.0.0.1:8080)")
		xrayService   = flag.String("xray-service", envOrDefault("XRAY_SERVICE", "xray"), "Systemd service name for xray")
		xrayAssetsDir = flag.String("xray-assets-dir", envOrDefault("XRAY_ASSETS_DIR", "/usr/local/share/xray"), "Directory for xray geoip.dat and geosite.dat")
		spoolDir      = flag.String("spool-dir", envOrDefault("SPOOL_DIR", "/var/lib/xray-agent"), "Directory for pending traffic batch spool")
		interval    = flag.Duration("interval", envDurationOrDefault("SYNC_INTERVAL", 60*time.Second), "Heartbeat sync interval")
		logLevel    = flag.String("log-level", envOrDefault("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
		logJSON     = flag.Bool("log-json", false, "Output structured JSON logs")
	)
	flag.Parse()

	logger.Init(*logLevel, *logJSON)

	client, err := agent.NewClient(*masterURL, *token, nil)
	if err != nil {
		slog.Error("Failed to initialize agent client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	spool, err := agent.NewSpool(*spoolDir)
	if err != nil {
		slog.Error("Failed to initialize traffic spool", slog.String("error", err.Error()))
		os.Exit(1)
	}

	quotaStore, err := agent.NewQuotaStore(*spoolDir)
	if err != nil {
		slog.Error("Failed to initialize quota store", slog.String("error", err.Error()))
		os.Exit(1)
	}

	grpcClient := xray.NewGRPCClient(*xrayGRPC)
	defer func() {
		if closeErr := grpcClient.Close(); closeErr != nil {
			slog.Warn("Failed to close xray gRPC client", slog.String("error", closeErr.Error()))
		}
	}()

	grpcAdapter, err := agent.NewXrayGRPCAdapter(grpcClient)
	if err != nil {
		slog.Error("Failed to initialize xray gRPC adapter", slog.String("error", err.Error()))
		os.Exit(1)
	}

	cfgMgr, err := agent.NewConfigManager(agent.ConfigManagerConfig{
		XrayBin:     *xrayBin,
		ConfigPath:  *xrayConfig,
		ServiceName: *xrayService,
		GRPCAddr:    *xrayGRPC,
	}, agent.OSCommandRunner{}, grpcAdapter)
	if err != nil {
		slog.Error("Failed to initialize config manager", slog.String("error", err.Error()))
		os.Exit(1)
	}

	userSync, err := agent.NewUserSyncer(grpcAdapter)
	if err != nil {
		slog.Error("Failed to initialize user syncer", slog.String("error", err.Error()))
		os.Exit(1)
	}

	geoUpdater, err := agent.NewHTTPGeoDataUpdater(agent.HTTPGeoDataUpdaterConfig{
		AssetsDir: *xrayAssetsDir,
	}, cfgMgr)
	if err != nil {
		slog.Error("Failed to initialize geodata updater", slog.String("error", err.Error()))
		os.Exit(1)
	}

	runner, err := agent.NewRunner(
		agent.RunnerConfig{
			AgentVersion: Version,
			Interval:     *interval,
		},
		client,
		spool,
		cfgMgr,
		userSync,
		grpcAdapter,
		agent.NewMetricsCollector(),
	)
	if err != nil {
		slog.Error("Failed to initialize agent runner", slog.String("error", err.Error()))
		os.Exit(1)
	}
	runner.WithQuotaStore(quotaStore).
		WithLogCollector(agent.NewJournalctlLogCollector(agent.OSCommandRunner{})).
		WithGeoDataUpdater(geoUpdater)

	slog.Info("Starting xray-panel thin agent",
		slog.String("version", Version),
		slog.String("xray_grpc", *xrayGRPC),
		slog.String("xray_config", *xrayConfig),
		slog.Duration("interval", *interval),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := runner.Run(ctx); err != nil {
		slog.Error("Agent runner exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("Agent stopped cleanly")
}
