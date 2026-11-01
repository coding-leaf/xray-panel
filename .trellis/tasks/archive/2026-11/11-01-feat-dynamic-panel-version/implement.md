# 面板版本号动态绑定与全链路展示 - 实施计划 (Implement Plan)

## Ordered Implementation Checklist

### 1. 后端版本配置与启动参数解析 (Backend Config & CLI Flag)
- [ ] 1.1 在 `internal/config/config.go` 的 `Config` 结构体中新增 `PanelVersion string`、`Commit string`、`BuildTime string` 字段。
- [ ] 1.2 增强 `internal/config/config.go` 中的 `Load` 函数：
  - 增加 `-panel-version` 命令行参数注册，默认值为 `getEnv("PANEL_VERSION", version)`；
  - 支持按 `CLI -panel-version` > `PANEL_VERSION` 环境变量 > `version` 实参的优先级解析并赋值到 `cfg.PanelVersion`；
  - 赋值 `cfg.Commit = commit` 与 `cfg.BuildTime = buildTime`；
  - 在 `-v` / `-version` 分支输出中使用解析后的 `cfg.PanelVersion`。
- [ ] 1.3 编写 `internal/config/config_test.go` 单元测试：
  - 测试默认场景下 `cfg.PanelVersion == version`；
  - 测试设置 `PANEL_VERSION` 环境变量时优先读取；
  - 测试指定 `-panel-version` 启动参数时具有最高优先级；
  - 测试 `Commit` 与 `BuildTime` 的正确保存；
  - 采用独立的 `flag.FlagSet` 或通过 `flag.CommandLine` 重置机制保证单测可重复并发执行无 panic。
- [ ] 1.4 在 `main.go:40` 的结构化日志中，将 `slog.String("version", Version)` 调整为 `slog.String("version", cfg.PanelVersion)`，确保启动日志准确反映覆盖后的版本。

### 2. HTTP 交付层与 API 暴露 (HTTP Delivery & API Exposure)
- [ ] 2.1 改造 `internal/delivery/http/handler_dashboard.go`：
  - 在 `DashboardHandler` 结构体中增加 `panelVersion string`、`commit string`、`buildTime string` 字段；
  - 调整构造函数 `NewDashboardHandler(monitorSvc *service.MonitorService, panelVersion string, meta ...string) *DashboardHandler`；
  - 在 `GetServiceStatus` (`GET /api/service/status`) JSON 响应中新增 `"panelVersion": h.panelVersion`、`"commit": h.commit`、`"buildTime": h.buildTime`。
- [ ] 2.2 编写 `internal/delivery/http/handler_dashboard_test.go` 单元测试：
  - 使用 `httptest.NewRecorder` 与 Gin 引擎测试 `GetServiceStatus`；
  - 断言响应包含 `panelVersion` 且与注入值一致。
- [ ] 2.3 在 `main.go:166` 中调整 `deliveryHTTP.NewDashboardHandler` 调用，注入 `cfg.PanelVersion`, `cfg.Commit`, `cfg.BuildTime`。
- [ ] 2.4 同步更新 CI/CD 构建工作流事实源：
  - 更新 `.github/workflows/release.yml:88` 的回退版本号由 `v2.6.5` 改为 `v2.6.6`；
  - 更新 `.github/workflows/ci.yml:93` 的构建测试版本由 `v2.6.5-ci` 改为 `v2.6.6-ci`。

### 3. 前端动态渲染与双端展示 (Frontend Dynamic Rendering)
- [ ] 3.1 改造 `web/vite.config.ts`：
  - 引入 `package.json` 中的 `version` 字段，并在 `define` 中注入 `'import.meta.env.VITE_APP_VERSION': JSON.stringify('v' + packageJson.version)` 作为编译期兜底。
- [ ] 3.2 改造 `web/src/App.vue`：
  - 在 `coreStatus` 响应式对象中补充 `panelVersion?: string` 字段；
  - 声明响应式或计算属性 `panelVersion = computed(() => coreStatus.value.panelVersion || import.meta.env.VITE_APP_VERSION || 'v2.6.6')`；
  - 在 `fetchCoreStatus` 中，当 `/api/service/status` 返回非空的 `res.panelVersion` 时更新 `coreStatus.value.panelVersion`；
  - 替换桌面左侧 Brand Header (`App.vue:20`) 硬编码的 `v2.6` 为 `{{ panelVersion }}`；
  - 在移动端 Drawer Header (`App.vue:74-82`) 的品牌标题旁新增版本徽章：
    `<span class="text-[9px] font-mono px-1 py-0.2 bg-muted text-muted-foreground rounded border border-border/60">{{ panelVersion }}</span>`。
- [ ] 3.3 改造 `web/src/mock/index.ts:118-124`：
  - 在 Mock 模式的 `/api/service/status` 响应中新增 `panelVersion: 'v2.6.6'`，确保 Demo 模式正常渲染。

### 4. 门禁验证与端到端检查 (Validation & Quality Gates)
- [ ] 4.1 运行后端单元测试全套：`mise x -- go test ./...`
- [ ] 4.2 运行后端静态分析：`mise x -- go vet ./...`
- [ ] 4.3 运行前端生产编译构建：`cd web && mise x -- npm run build`
- [ ] 4.4 运行后端完整二进制构建：`mise x -- go build .`
- [ ] 4.5 命令行交互回归验证：
  - 运行 `go run . -v` 验证默认版本；
  - 运行 `PANEL_VERSION=v2.7.0-env-test go run . -v` 验证环境变量覆盖；
  - 运行 `go run . -panel-version v2.7.0-cli-test -v` 验证参数覆盖。

---

## 风险文件与回滚策略 (Risky Files & Rollback Points)
- **`main.go:31-35`**：严禁重命名 `Version`, `Commit`, `BuildTime`，任何重命名都会导致 CI ldflags 注入静默失效。
- **`internal/config/config.go`**：必须通过 `flag.CommandLine` 安全隔离测试，避免单测套件并发或重复解析导致 `flag redefined` 崩溃。
- **`web/src/App.vue`**：双端版本徽章修改仅限文字插值与抽屉头部排版补充，严禁破坏导航抽屉的移动端响应式交互及开关状态。
