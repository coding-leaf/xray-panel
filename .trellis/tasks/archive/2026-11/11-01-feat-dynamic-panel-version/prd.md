# 面板版本号动态绑定与全链路展示 (Dynamic Panel Version Binding & Display)

## Goal
实现面板版本号从构建期注入（CI ldflags）、启动参数（CLI `-panel-version`）、环境变量（`PANEL_VERSION`）到运行时配置、后端 HTTP API（`/api/service/status`）、前端顶栏桌面 Brand Header 与移动端侧边抽屉 Header 的全链路动态绑定与平滑展示，彻底消除前端 `v2.6` 硬编码与 CI 构建兜底版本失配问题。

## Confirmed Facts (Codebase Audit)
- **后端版本定义与 ldflags 契约**：
  - `main.go:31-35` 定义了全局变量 `var Version = "v2.6.6"`, `Commit = "dev"`, `BuildTime = "unknown"`。
  - `.trellis/spec/backend/index.md:38` 明确规定：`main.Version` 是 ldflags 注入契约（`-X 'main.Version=...'`），禁止重命名该变量。
  - `main.go:38` 当前调用 `config.Load(Version, Commit, BuildTime)`，但 `internal/config/config.go:11-23` 中的 `Config` 结构体未保存版本信息，仅用于在 `-v` / `-version` 时直接标准输出并退出（`config.go:42-45`）。
- **CI/CD 构建兜底版本失配**：
  - `.github/workflows/release.yml:88` 的兜底版本仍为 `TAG_VERSION="v2.6.5"`，未同步升级到 `v2.6.6`。
  - `.github/workflows/ci.yml:93` 构建注入版本仍硬编码为 `v2.6.5-ci`。
  - `web/package.json:3` 版本号为 `"2.6.6"`。
- **HTTP 状态接口缺失面板版本**：
  - `internal/delivery/http/handler_dashboard.go:28-35` 的 `GetServiceStatus` 接口仅返回 `active`, `subState`, `version`（此处 `version` 为 Xray-core 版本），未暴露面板自身的版本号（`panelVersion`）。
  - `main.go:166` 实例化 `DashboardHandler` 时仅传入 `monitorSvc`，未注入面板版本。
- **前端硬编码与展示缺漏**：
  - `web/src/App.vue:20` 桌面左上角 Brand Header 中版本徽章硬编码为 `<span ...>v2.6</span>`。
  - `web/src/App.vue:74-82` 移动端抽屉 Header 中仅展示了品牌标题 `XRAY PANEL`，完全缺失版本号徽章。
  - `web/src/App.vue:436-455` 已有 6 秒定时轮询 `/api/service/status` 的 `fetchCoreStatus` 逻辑，响应对象中仅提取了 `version`（核心版本）赋给 `coreStatus.version`。
  - `web/src/mock/index.ts:118-124` 在 Mock 模式下返回的核心状态未包含 `panelVersion`。

## Requirements

### 1. 后端版本号解析与启动配置绑定 (Backend Resolution & Config Binding)
- **配置结构体扩展**：
  - 在 `internal/config/config.go:11-23` 的 `Config` 结构体中新增 `PanelVersion string`、`Commit string`、`BuildTime string` 字段。
- **命令行参数与环境变量优先级解析**：
  - 新增命令行标志 `-panel-version`，默认值优先取环境变量 `PANEL_VERSION`，若未指定则兜底为传入 `config.Load(version, commit, buildTime)` 的 `version` 实参。
  - 解析优先级为：CLI `-panel-version` > Env `PANEL_VERSION` > 编译期注入或代码兜底值（`version`）。
  - 将最终解析得到的面板版本存入 `cfg.PanelVersion`，并将 `commit` 与 `buildTime` 分别存入 `cfg.Commit` 与 `cfg.BuildTime`。
- **版本查询 CLI 行为保证**：
  - 当通过 `-v` 或 `-version` 查看版本时，输出内容中使用最终解析的 `PanelVersion`，确保用户指定 `-panel-version v2.7.0-custom -v` 时能正确回显自定义版本。
- **保持 ldflags 兼容性**：
  - `main.go:31-35` 保持变量名 `Version`, `Commit`, `BuildTime` 完全不变，无缝承接 CI/CD `-ldflags` 注入。

### 2. HTTP 交付层接口扩展 (API Delivery Exposure)
- **状态接口扩展**：
  - 在 `internal/delivery/http/handler_dashboard.go` 的 `DashboardHandler` 中注入 `panelVersion`（以及可选的 `commit`, `buildTime`）。
  - 在 `GetServiceStatus` (`GET /api/service/status`) 响应 JSON 中新增字段：
    - `"panelVersion": h.panelVersion`
    - （可选）`"commit": h.commit`、`"buildTime": h.buildTime`
  - 保持原有 `"active"`, `"subState"`, `"version"` 契约 100% 向后兼容。
- **路由与装配层集成**：
  - 在 `main.go:166` 实例化 `deliveryHTTP.NewDashboardHandler` 时，将 `cfg.PanelVersion`（及 `cfg.Commit`, `cfg.BuildTime`）注入处理器。
- **CI/CD 兜底版本同步**：
  - 同步更新 `.github/workflows/release.yml:88` 的回退版本号为 `v2.6.6`。
  - 同步更新 `.github/workflows/ci.yml:93` 的构建测试版本号为 `v2.6.6-ci`。

### 3. 前端顶栏与移动端抽屉全链路动态展示 (Frontend Dynamic Rendering)
- **编译期构建默认值注入**：
  - 在 `web/vite.config.ts` 中通过 `define` 机制注入前端初始兜底版本（如从 `package.json` 提取的 `v2.6.6`，或声明为 `import.meta.env.VITE_APP_VERSION`）。
- **响应式状态动态绑定**：
  - 在 `web/src/App.vue` 中定义响应式或计算属性 `panelVersion`，初始值为编译期兜底版本（如 `v2.6.6`）。
  - 在现有 `fetchCoreStatus` 轮询逻辑中，当 `/api/service/status` 返回非空的 `res.panelVersion` 时，动态更新前端状态。
- **UI 界面对齐与双端展示**：
  - **桌面端 Brand Header** (`App.vue:20`)：将硬编码的 `v2.6` 替换为 `{{ panelVersion }}` 动态徽章。
  - **移动端 Drawer Header** (`App.vue:74-82`)：在品牌标题 `XRAY PANEL` 右侧补齐版本徽章，样式与桌面端保持一致（`text-[9px] font-mono px-1 py-0.2 bg-muted text-muted-foreground rounded border border-border/60`），确保移动端体验完整。
- **Mock 环境对齐**：
  - 在 `web/src/mock/index.ts:118-124` 的 `/api/service/status` 模拟响应中增加 `panelVersion: 'v2.6.6'`，确保 demo 演示模式及本地开发环境无缝对齐。

## Acceptance Criteria
- [ ] **AC-1 (CLI/Env 优先级单测)**：在 `internal/config/config_test.go` 中编写单元测试，验证：
  - 默认情况下 `cfg.PanelVersion == version`；
  - 设置 `PANEL_VERSION` 环境变量时，`cfg.PanelVersion` 优先使用环境变量；
  - 传入命令行参数 `-panel-version` 时，优先级高于环境变量与默认值；
  - `cfg.Commit` 与 `cfg.BuildTime` 正确绑定。
- [ ] **AC-2 (API 返回验证单测)**：在 `internal/delivery/http/handler_dashboard_test.go` 中编写单元测试，调用 `GET /api/service/status` 验证返回包含正确的 `panelVersion` 且结构合法。
- [ ] **AC-3 (CI/CD 事实源对齐)**：`.github/workflows/release.yml:88` 与 `ci.yml:93` 兜底版本号均已对齐至 `v2.6.6`，且未重命名 `main.Version` 变量。
- [ ] **AC-4 (前端桌面端动态渲染)**：`web/src/App.vue:20` 移除 `v2.6` 硬编码，根据后端返回的 `panelVersion` 动态展示；无后端连接时展示 `package.json` 兜底版本 `v2.6.6`。
- [ ] **AC-5 (前端移动端抽屉展示)**：`web/src/App.vue:74-82` 补齐版本徽章，与桌面端样式和内容完全同步。
- [ ] **AC-6 (Mock 对齐)**：`web/src/mock/index.ts` 包含 `panelVersion` 字段，Demo 模式下正常渲染版本。
- [ ] **AC-7 (全量质量门禁)**：
  - `mise x -- go test ./...` 全绿（含新增的 config 与 handler 单测）；
  - `mise x -- go vet ./...` 无任何警告；
  - `cd web && mise x -- npm run build` 构建成功且类型检查通过。

## Out of Scope
- 二进制在线自更新 / 自动下载新版本安装机制（属于独立版本升级用例）。
- Xray-core 核心版本字符串重写（Xray 版本仍由 `monitorSvc.GetServiceStatus` 原生返回）。
- 运行时无需重启动态热修改面板版本号（版本号作为进程启动静态生命周期元数据，随进程重启即可）。
