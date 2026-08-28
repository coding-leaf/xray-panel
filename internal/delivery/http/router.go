package http

import (
	"io/fs"
	"net/http"
	"strings"

	"panel/internal/delivery/http/middleware"

	"github.com/gin-gonic/gin"
)

type Handlers struct {
	Auth      *AuthHandler
	Dashboard *DashboardHandler
	User      *UserHandler
	Inbound   *InboundHandler
	Outbound  *OutboundHandler
	Routing   *RoutingHandler
	Config    *ConfigHandler
	Sub       *SubHandler
	Setting   *SettingHandler
	Log       *LogHandler
	DNS       *DNSHandler
	GeoData   *GeoDataHandler
}

func SetupRouter(handlers *Handlers, jwtSecret string, staticFS fs.FS) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 全局日志与 Recovery 中间件
	r.Use(middleware.SlogLogger())
	r.Use(middleware.Recovery())

	// CORS 中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Request-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	api := r.Group("/api")
	{
		// 公开接口
		api.POST("/auth/login", handlers.Auth.Login)
		api.GET("/sub/:token", handlers.Sub.GetSubscription)

		// 管理受保护接口 (JWT 鉴权)
		authGroup := api.Group("")
		authGroup.Use(middleware.JWTAuth(jwtSecret))
		{
			authGroup.POST("/auth/password", handlers.Auth.ChangePassword)

			// 仪表盘
			authGroup.GET("/dashboard", handlers.Dashboard.GetDashboard)

			// 用户管理
			authGroup.GET("/users", handlers.User.List)
			authGroup.POST("/users", handlers.User.Create)
			authGroup.PUT("/users/:id", handlers.User.Update)
			authGroup.DELETE("/users/:id", handlers.User.Delete)
			authGroup.POST("/users/:id/reset", handlers.User.ResetTraffic)
			authGroup.POST("/users/:id/reset-traffic", handlers.User.ResetTraffic)
			authGroup.GET("/users/:id/share", handlers.User.GetShareLink)
			authGroup.GET("/users/:id/traffic-history", handlers.User.GetTrafficHistory)

			// 节点入站管理
			authGroup.GET("/inbounds", handlers.Inbound.List)
			authGroup.POST("/inbounds", handlers.Inbound.Create)
			authGroup.PUT("/inbounds/:id", handlers.Inbound.Update)
			authGroup.DELETE("/inbounds/:id", handlers.Inbound.Delete)
			authGroup.GET("/inbounds/reality-keypair", handlers.Inbound.GenerateRealityKey)

			// 出站管理
			authGroup.GET("/outbounds", handlers.Outbound.List)
			authGroup.POST("/outbounds", handlers.Outbound.Save)
			authGroup.DELETE("/outbounds/:tag", handlers.Outbound.Delete)

			// 路由分流规则管理
			authGroup.GET("/routing", handlers.Routing.Get)
			authGroup.POST("/routing", handlers.Routing.Save)

			// DNS 设置管理
			authGroup.GET("/dns", handlers.DNS.Get)
			authGroup.POST("/dns", handlers.DNS.Save)

			// 原始配置在线校验与保存及快照
			authGroup.GET("/config/raw", handlers.Config.GetRaw)
			authGroup.POST("/config/validate", handlers.Config.ValidateRaw)
			authGroup.POST("/config/save", handlers.Config.SaveAndApply)
			authGroup.GET("/config/snapshots", handlers.Config.ListSnapshots)
			authGroup.POST("/config/snapshots/:id/rollback", handlers.Config.RollbackSnapshot)

			// GeoData 规则库与升级
			authGroup.GET("/geodata/status", handlers.GeoData.GetStatus)
			authGroup.POST("/geodata/update", handlers.GeoData.UpdateGeoData)

			// 运行日志查看
			authGroup.GET("/logs", handlers.Log.GetLogs)

			// 系统服务控制
			authGroup.POST("/service/restart", handlers.Config.RestartService)

			// 系统设置
			authGroup.GET("/settings", handlers.Setting.GetSettings)
			authGroup.POST("/settings", handlers.Setting.SaveSettings)
			authGroup.POST("/settings/test-telegram", handlers.Setting.TestTelegram)
		}
	}

	// 嵌入静态前端资源 (SPA fallback)
	if staticFS != nil {
		fileServer := http.FileServer(http.FS(staticFS))
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			if strings.HasPrefix(path, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"error": "API route not found"})
				return
			}

			// 检查静态资源是否存在，不存在则返回 index.html 实现 SPA 路由
			f, err := staticFS.Open(strings.TrimPrefix(path, "/"))
			if err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}

			indexFile, err := staticFS.Open("index.html")
			if err == nil {
				_ = indexFile.Close()
				c.Request.URL.Path = "/"
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}

			c.String(http.StatusOK, "Xray Decoupled Panel API is running. Frontend assets not found.")
		})
	}

	return r
}
