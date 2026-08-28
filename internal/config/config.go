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
}

func Load(version, commit, buildTime string) *Config {
	cfg := &Config{}

	defaultBin := detectDefaultXrayBin()
	defaultConfig := detectDefaultXrayConfig()

	showVersion := flag.Bool("v", false, "Show version information")
	showVersionLong := flag.Bool("version", false, "Show version information")

	flag.StringVar(&cfg.ListenPort, "port", getEnv("PANEL_PORT", "9000"), "Panel listen port")
	flag.StringVar(&cfg.XrayGRPCAddr, "xray-grpc", getEnv("XRAY_GRPC_ADDR", "127.0.0.1:8080"), "Xray API gRPC listen address")
	flag.StringVar(&cfg.XrayConfigPath, "xray-config", getEnv("XRAY_CONFIG_PATH", defaultConfig), "Path to xray config.json")
	flag.StringVar(&cfg.XrayBinPath, "xray-bin", getEnv("XRAY_BIN_PATH", defaultBin), "Path to xray binary")
	flag.StringVar(&cfg.DBPath, "db", getEnv("DB_PATH", "data/panel.db"), "Path to SQLite database")
	flag.StringVar(&cfg.JWTSecret, "jwt-secret", getEnv("PANEL_JWT_SECRET", getEnv("JWT_SECRET", "super-secret-key-change-me")), "JWT secret key")
	flag.StringVar(&cfg.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	flag.BoolVar(&cfg.LogJSON, "log-json", false, "Log in JSON format")
	flag.StringVar(&cfg.ServiceName, "service", getEnv("SERVICE_NAME", "xray"), "Xray systemd service name")
	flag.StringVar(&cfg.PublicURL, "public-url", getEnv("PUBLIC_URL", "http://127.0.0.1:9000"), "Public panel URL for subscription links")

	flag.Parse()

	if *showVersion || *showVersionLong {
		fmt.Printf("Xray Decoupled Panel %s (Commit: %s, Built: %s)\n", version, commit, buildTime)
		os.Exit(0)
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
