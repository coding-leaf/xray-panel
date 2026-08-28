package config

import (
	"flag"
	"os"
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

func Load() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.ListenPort, "port", getEnv("PANEL_PORT", "9000"), "Panel listen port")
	flag.StringVar(&cfg.XrayGRPCAddr, "xray-grpc", getEnv("XRAY_GRPC_ADDR", "127.0.0.1:8080"), "Xray API gRPC listen address")
	flag.StringVar(&cfg.XrayConfigPath, "xray-config", getEnv("XRAY_CONFIG_PATH", "config.json"), "Path to xray config.json")
	flag.StringVar(&cfg.XrayBinPath, "xray-bin", getEnv("XRAY_BIN_PATH", "xray"), "Path to xray binary")
	flag.StringVar(&cfg.DBPath, "db", getEnv("DB_PATH", "data/panel.db"), "Path to SQLite database")
	flag.StringVar(&cfg.JWTSecret, "jwt-secret", getEnv("JWT_SECRET", "super-secret-key-change-me"), "JWT secret key")
	flag.StringVar(&cfg.LogLevel, "log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	flag.BoolVar(&cfg.LogJSON, "log-json", false, "Log in JSON format")
	flag.StringVar(&cfg.ServiceName, "service", getEnv("SERVICE_NAME", "xray"), "Xray systemd service name")
	flag.StringVar(&cfg.PublicURL, "public-url", getEnv("PUBLIC_URL", "http://127.0.0.1:9000"), "Public panel URL for subscription links")

	flag.Parse()
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
