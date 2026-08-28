package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

func main() {
	// 1. 加载配置与初始化结构化日志
	cfg := config.Load()
	logger.Init(cfg.LogLevel, cfg.LogJSON)

	slog.Info("Starting Xray Decoupled Panel",
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
	configSvc := service.NewConfigService(configMgr, supervisor, inboundRepo, userRepo)
	userSvc := service.NewUserService(userRepo, inboundRepo, trafficLogRepo, xrayManager, configSvc)
	subSvc := service.NewSubService(userRepo, inboundRepo, settingRepo)
	monitorSvc := service.NewMonitorService(hostMonitor, xrayManager, userRepo, inboundRepo)
	alertSvc := service.NewAlertService(botAdapter, userRepo, hostMonitor, configMgr)
	logSvc := service.NewLogService(configMgr)

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
		Setting:   deliveryHTTP.NewSettingHandler(settingRepo, botAdapter),
		Log:       deliveryHTTP.NewLogHandler(logSvc),
		DNS:       deliveryHTTP.NewDNSHandler(configSvc),
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

	// 启动后台定时任务
	syncJob := deliveryCron.NewTrafficSyncJob(xrayManager, userRepo, inboundRepo, trafficLogRepo, alertSvc, 15*time.Second)
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
