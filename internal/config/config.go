package config

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

type Config struct {
	ListenPort     string
	XrayGRPCAddr   string
	XrayConfigPath string
	XrayBinPath    string
	DBPath         string
	JWTSecret      string
	LogLevel       string
	LogJSON        bool
	ServiceName    string
	PublicURL      string

	// 面板版本元数据
	PanelVersion string
	Commit       string
	BuildTime    string
}

var exitFunc = os.Exit

func Load(version, commit, buildTime string) *Config {
	return LoadWithFlagSet(flag.CommandLine, os.Args[1:], version, commit, buildTime)
}

func LoadWithFlagSet(fs *flag.FlagSet, args []string, version, commit, buildTime string) *Config {
	cfg := &Config{
		Commit:    commit,
		BuildTime: buildTime,
	}

	defaultBin := detectDefaultXrayBin()
	defaultConfig := detectDefaultXrayConfig()

	showVersion := fs.Bool("v", false, "Show version information")
	showVersionLong := fs.Bool("version", false, "Show version information")

	fs.StringVar(&cfg.ListenPort, "port", getEnv("PANEL_PORT", "9000"), "Panel listen port")
	fs.StringVar(&cfg.XrayGRPCAddr, "xray-grpc", getEnv("XRAY_GRPC_ADDR", "127.0.0.1:8080"), "Xray API gRPC listen address")
	fs.StringVar(&cfg.XrayConfigPath, "xray-config", getEnv("XRAY_CONFIG_PATH", defaultConfig), "Path to xray config.json")
	fs.StringVar(&cfg.XrayBinPath, "xray-bin", getEnv("XRAY_BIN_PATH", defaultBin), "Path to xray binary")
	fs.StringVar(&cfg.DBPath, "db", getEnv("DB_PATH", "data/panel.db"), "Path to SQLite database")
	fs.StringVar(&cfg.JWTSecret, "jwt-secret", getEnv("PANEL_JWT_SECRET", getEnv("JWT_SECRET", "")), "JWT secret key (leave empty to auto-generate and persist in database)")
	fs.StringVar(&cfg.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	fs.BoolVar(&cfg.LogJSON, "log-json", false, "Log in JSON format")
	fs.StringVar(&cfg.ServiceName, "service", getEnv("SERVICE_NAME", "xray"), "Xray systemd service name")
	fs.StringVar(&cfg.PublicURL, "public-url", getEnv("PUBLIC_URL", "http://127.0.0.1:9000"), "Public panel URL for subscription links")
	fs.StringVar(&cfg.PanelVersion, "panel-version", getEnv("PANEL_VERSION", version), "Panel version override")

	_ = fs.Parse(args)

	if *showVersion || *showVersionLong {
		fmt.Printf("Xray Decoupled Panel %s (Commit: %s, Built: %s)\n", cfg.PanelVersion, cfg.Commit, cfg.BuildTime)
		exitFunc(0)
	}

	return cfg
}

func detectDefaultXrayBin() string {
	if runtime.GOOS == "windows" {
		for _, p := range []string{"xray.exe", "../xray-core/xray.exe", "xray-core/xray.exe"} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return "xray.exe"
	}
	for _, p := range []string{"/usr/local/bin/xray", "/usr/bin/xray"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if lp, err := exec.LookPath("xray"); err == nil {
		return lp
	}
	return "/usr/local/bin/xray"
}

func detectDefaultXrayConfig() string {
	for _, p := range []string{
		"/usr/local/etc/xray/config.json",
		"/etc/xray/config.json",
		"config.json",
		"../server-configs/xray/config.json",
		"server-configs/xray/config.json",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if runtime.GOOS == "linux" {
		return "/usr/local/etc/xray/config.json"
	}
	return "config.json"
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
