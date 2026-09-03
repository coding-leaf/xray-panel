package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"panel/internal/adapter/monitor"
	"panel/internal/adapter/repository"
	"panel/internal/adapter/telegram"
	"panel/internal/adapter/xray"
	"panel/internal/config"
	deliveryCron "panel/internal/delivery/cron"
	deliveryHTTP "panel/internal/delivery/http"
	"panel/internal/pkg/logger"
	"panel/internal/service"
)

var (
	Version   = "v1.5.1"
	Commit    = "dev"
	BuildTime = "unknown"
)

func main() {
	// 1. 加载配置与初始化结构化日志
	cfg := config.Load(Version, Commit, BuildTime)
	logger.Init(cfg.LogLevel, cfg.LogJSON)

	slog.Info("Starting Xray Decoupled Panel",
		slog.String("version", Version),
		slog.String("port", cfg.ListenPort),
		slog.String("xray_grpc", cfg.XrayGRPCAddr),
		slog.String("db_path", cfg.DBPath),
	)

	// 2. 初始化持久化 SQLite
	db, err := repository.InitSQLite(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to init SQLite", slog.String("error", err.Error()))
		os.Exit(1)
	}

	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	trafficLogRepo := repository.NewGormTrafficLogRepository(db)
	snapshotRepo := repository.NewGormConfigSnapshotRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	adminRepo := repository.NewAdminRepository(db)

	// 3. 初始化适配器 Adapters
	grpcClient := xray.NewGRPCClient(cfg.XrayGRPCAddr)
	configMgr := xray.NewConfigManager(cfg.XrayConfigPath, cfg.XrayBinPath)
	supervisor := xray.NewSystemdSupervisor(cfg.ServiceName, cfg.XrayBinPath)
	xrayManager := xray.NewManager(grpcClient, configMgr, supervisor, inboundRepo)
	hostMonitor := monitor.NewGopsutilMonitor()

	// 加载 Telegram 设置
	bgCtx := context.Background()

	// 确保 JWT Secret 安全初始化 (优先使用环境变量/参数，未指定则从数据库加载或自动生成高熵密钥)
	if cfg.JWTSecret == "" || cfg.JWTSecret == "super-secret-key-change-me" {
		savedSecret, err := settingRepo.Get(bgCtx, "jwt_secret")
		if err == nil && savedSecret != "" {
			cfg.JWTSecret = savedSecret
		} else {
			randomBytes := make([]byte, 32)
			if _, err := rand.Read(randomBytes); err != nil {
				slog.Error("Failed to generate secure random JWT secret", slog.String("error", err.Error()))
				os.Exit(1)
			}
			cfg.JWTSecret = hex.EncodeToString(randomBytes)
			if err := settingRepo.Set(bgCtx, "jwt_secret", cfg.JWTSecret); err != nil {
				slog.Warn("Failed to persist JWT secret to database", slog.String("error", err.Error()))
			} else {
				slog.Info("Generated and persisted secure high-entropy JWT secret")
			}
		}
	}
	tgToken, _ := settingRepo.Get(bgCtx, "tg_bot_token")
	tgChatIDStr, _ := settingRepo.Get(bgCtx, "tg_admin_chat_id")
	var tgChatID int64
	if tgChatIDStr != "" {
		tgChatID, _ = strconv.ParseInt(tgChatIDStr, 10, 64)
	}

	botAdapter := telegram.NewBotAdapter(tgToken, tgChatID)
	_ = botAdapter.Init()

	botHandler := telegram.NewBotHandler(botAdapter, userRepo, inboundRepo, hostMonitor, xrayManager, cfg.PublicURL)

	// 4. 初始化业务用例深模块 Services
	grpcPort := 8080
	if cfg.XrayGRPCAddr != "" {
		if _, portStr, err := net.SplitHostPort(cfg.XrayGRPCAddr); err == nil {
			if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
				grpcPort = p
			}
		} else if p, err := strconv.Atoi(cfg.XrayGRPCAddr); err == nil && p > 0 {
			grpcPort = p
		}
	}
	compiler := xray.NewXrayCompiler(grpcPort)

	configSvc := service.NewConfigService(configMgr, supervisor, inboundRepo, userRepo, snapshotRepo, compiler)
	_ = configSvc.SyncFromFile(bgCtx)
	_ = configSvc.RecompileAndApply(bgCtx, "面板启动自动同步与编译配置")

	userSvc := service.NewUserService(userRepo, inboundRepo, trafficLogRepo, xrayManager, configSvc)
	subSvc := service.NewSubService(userRepo, inboundRepo, settingRepo)
	monitorSvc := service.NewMonitorService(hostMonitor, xrayManager, userRepo, inboundRepo)
	alertSvc := service.NewAlertService(botAdapter, userRepo, hostMonitor, configMgr)
	logSvc := service.NewLogService(configMgr)
	geoSvc := service.NewGeoDataService(cfg.XrayBinPath, xrayManager)

	// 5. 初始化 HTTP API 处理器
	handlers := &deliveryHTTP.Handlers{
		Auth:      deliveryHTTP.NewAuthHandler(adminRepo, cfg.JWTSecret),
		Dashboard: deliveryHTTP.NewDashboardHandler(monitorSvc),
		User:      deliveryHTTP.NewUserHandler(userSvc, subSvc),
		Inbound:   deliveryHTTP.NewInboundHandler(configSvc),
		Outbound:  deliveryHTTP.NewOutboundHandler(configSvc),
		Routing:   deliveryHTTP.NewRoutingHandler(configSvc),
		Config:    deliveryHTTP.NewConfigHandler(configSvc),
		Sub:       deliveryHTTP.NewSubHandler(subSvc),
		Setting:   deliveryHTTP.NewSettingHandler(settingRepo, botAdapter, configMgr, supervisor),
		Log:       deliveryHTTP.NewLogHandler(logSvc),
		DNS:       deliveryHTTP.NewDNSHandler(configSvc),
		GeoData:   deliveryHTTP.NewGeoDataHandler(geoSvc),
	}

	staticFS := getStaticFS()
	router := deliveryHTTP.SetupRouter(handlers, cfg.JWTSecret, staticFS)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.ListenPort),
		Handler: router,
	}

	// 6. 生命周期管理与优雅关机
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 启动后台定时任务 (5 秒轮询实时上下行与在线状态)
	syncJob := deliveryCron.NewTrafficSyncJob(xrayManager, userRepo, inboundRepo, trafficLogRepo, alertSvc, userSvc, 5*time.Second)
	syncJob.Start(ctx)

	// 启动 Telegram Bot 轮询
	botHandler.StartPolling(ctx)

	// 启动 HTTP 服务
	go func() {
		slog.Info("Panel HTTP server listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("Shutting down panel gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced shutdown", slog.String("error", err.Error()))
	}
	_ = grpcClient.Close()

	slog.Info("Panel server exited safely")
}
