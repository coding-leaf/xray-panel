package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"panel/internal/adapter/monitor"
	"panel/internal/adapter/repository"
	"panel/internal/adapter/telegram"
	"panel/internal/adapter/xray"
	"panel/internal/app"
	"panel/internal/config"
	deliveryCron "panel/internal/delivery/cron"
	deliveryHTTP "panel/internal/delivery/http"
	"panel/internal/pkg/logger"
	"panel/internal/service"
)

var (
	Version   = "v1.6.0"
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

	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("Failed to acquire raw DB handle", slog.String("error", err.Error()))
		os.Exit(1)
	}
	storage := sqlDB

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

	// 6. 生命周期管理与统一服务编排调度
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	httpSvc := deliveryHTTP.NewServer(cfg.ListenPort, router, deliveryHTTP.WithShutdownTimeout(5*time.Second))
	syncJob := deliveryCron.NewTrafficSyncJob(xrayManager, userRepo, inboundRepo, trafficLogRepo, alertSvc, userSvc, 5*time.Second)

	services := []struct {
		name    string
		service app.Service
	}{
		{name: "HTTP Server", service: httpSvc},
		{name: "Traffic Sync Job", service: syncJob},
		{name: "Telegram Bot", service: botHandler},
	}

	// 使用 errgroup.WithContext 统一拉起所有 Service
	g, gCtx := errgroup.WithContext(rootCtx)

	for _, s := range services {
		svc := s
		g.Go(func() error {
			slog.Info("Starting service", slog.String("name", svc.name))
			if err := svc.service.Start(gCtx); err != nil {
				slog.Error("Service exited with error", slog.String("name", svc.name), slog.String("error", err.Error()))
				return fmt.Errorf("%s: %w", svc.name, err)
			}
			if gCtx.Err() == nil {
				err := fmt.Errorf("service %s stopped unexpectedly", svc.name)
				slog.Error("Service stopped unexpectedly", slog.String("name", svc.name))
				return err
			}
			slog.Info("Service stopped cleanly", slog.String("name", svc.name))
			return nil
		})
	}

	slog.Info("All background services orchestrated and running")

	// 等待所有服务退出 (任一服务异常崩溃或收到外部终止信号时，级联通知所有服务优雅退出)
	var exitCode int
	if err := g.Wait(); err != nil {
		slog.Error("Application terminated due to service failure", slog.String("error", err.Error()))
		exitCode = 1
	} else {
		slog.Info("All background services shut down gracefully")
	}

	// 退出阶段：释放外部连接与数据库锁
	slog.Info("Executing application cleanup phases...")

	// 阶段 1: 关闭外部客户端连接
	slog.Info("[Phase 1/2] Closing Xray gRPC client connection...")
	if err := grpcClient.Close(); err != nil {
		slog.Warn("Failed to close Xray gRPC client cleanly", slog.String("error", err.Error()))
	} else {
		slog.Info("Xray gRPC client connection closed successfully")
	}

	// 阶段 2: 所有服务退出后，统一调用 storage.Close() 释放数据库锁，再退出主进程
	slog.Info("[Phase 2/2] Closing database storage to release database file lock...")
	if err := storage.Close(); err != nil {
		slog.Error("Failed to close database storage", slog.String("error", err.Error()))
		if exitCode == 0 {
			exitCode = 1
		}
	} else {
		slog.Info("Database storage closed and file locks released successfully")
	}

	slog.Info("Panel server exited safely")
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
