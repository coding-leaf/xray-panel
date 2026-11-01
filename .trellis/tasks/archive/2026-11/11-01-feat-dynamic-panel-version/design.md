# 面板版本号动态绑定与全链路展示 - 技术设计 (Design)

## 1. 总体架构与数据流向 (Architecture & Data Flow)

面板版本号采用“编译期注入/默认兜底 -> 启动参数与环境变量按序覆盖 -> 核心配置单例装载 -> HTTP API 暴露 -> 前端响应式状态监听与双端渲染”的单向数据流动模型：

```
[构建/启动期]
CI/CD ldflags (-X 'main.Version=...') ──────────┐
源码默认声明 (main.Version="v2.6.6") ───────────┼─► main.go:main()
                                                │
                                                ▼
环境变量 (PANEL_VERSION) ─────────────────► config.Load(Version, Commit, BuildTime)
CLI 启动参数 (-panel-version) ──────────────► [Flag & Env 解析器]
                                                │
                                                ▼
                                            cfg.PanelVersion / cfg.Commit / cfg.BuildTime
                                                │
                                                ▼
[运行时 API 暴露]                            main.go (初始化 Handlers)
                                                │
                                                ▼
                                            deliveryHTTP.NewDashboardHandler(monitorSvc, cfg.PanelVersion, ...)
                                                │
                                                ▼
                                            GET /api/service/status
                                            {
                                              "active": true,
                                              "subState": "running",
                                              "version": "1.8.24",        // Xray Core 版本
                                              "panelVersion": "v2.6.6",    // 面板版本
                                              "commit": "dev",
                                              "buildTime": "unknown"
                                            }
                                                │
                                                ▼
[前端双端响应式渲染]                          web/src/App.vue
Vite 编译期兜底 (__APP_VERSION__) ──────────► panelVersion (初始兜底值)
                                                │ (轮询 fetchCoreStatus 动态更新)
                                                ▼
                                      ┌────────────────────────┐
                                      │ 桌面端 Brand Header    │ <span ...>{{ panelVersion }}</span>
                                      │ 移动端 Drawer Header   │ <span ...>{{ panelVersion }}</span>
                                      └────────────────────────┘
```

---

## 2. 后端设计与契约约束 (Backend Design & Contracts)

### 2.1 启动参数与优先级解析 (`internal/config`)
在 `internal/config/config.go` 中，增强 `Config` 结构体并重构解析流程：

```go
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
    PanelVersion   string
    Commit         string
    BuildTime      string
}
```

**解析逻辑与优先级**：
1. **默认值基准**：接收由调用方传入的 `version`（通常由 `main.go` 中的 `Version` 注入）；
2. **环境变量覆盖**：读取 `getEnv("PANEL_VERSION", version)`；
3. **命令行 Flag 覆盖**：使用 `flag.StringVar(&cfg.PanelVersion, "panel-version", getEnv("PANEL_VERSION", version), "Panel version override")`；
4. **元数据赋值**：
   ```go
   cfg.Commit = commit
   cfg.BuildTime = buildTime
   ```
5. **版本查询短路分支 (`-v` / `-version`)**：
   `flag.Parse()` 完成后，若用户指定了 `-v` 或 `-version`，直接输出最终解析后的版本元数据并退出：
   ```go
   if *showVersion || *showVersionLong {
       fmt.Printf("Xray Decoupled Panel %s (Commit: %s, Built: %s)\n", cfg.PanelVersion, cfg.Commit, cfg.BuildTime)
       os.Exit(0)
   }
   ```
   *说明：此逻辑保证了即使用户使用 `panel -panel-version v2.7.0-custom -v`，打印出的版本也是覆盖后的解析版本。*

### 2.2 保持 CI/CD 与 ldflags 兼容性
- 根据 `.trellis/spec/backend/index.md:38`，`main.go` 中的变量名 `Version`, `Commit`, `BuildTime` 是 CI/CD 构建脚本的 ldflags 注入契约，**严禁修改变量名称**。
- `main.go:37-45` 维持原入口：
  ```go
  cfg := config.Load(Version, Commit, BuildTime)
  ```
  在结构化日志输出中，将输出由 `Version` 改为 `cfg.PanelVersion`，实现日志与真实运行态完全一致：
  ```go
  slog.Info("Starting Xray Decoupled Panel",
      slog.String("version", cfg.PanelVersion),
      slog.String("commit", cfg.Commit),
      slog.String("port", cfg.ListenPort),
      ...
  )
  ```

### 2.3 单元测试安全性设计 (`config_test.go`)
Go 原生 `flag.CommandLine` 在一个进程（如单元测试套件）中多次调用 `flag.Parse()` 会因为重复注册标志而 panic。
- **解决方案**：在 `internal/config/config.go` 中，将 Flag 注册与解析抽象为接收 `*flag.FlagSet` 的内部函数或可测试函数 `loadWithFlagSet(fs *flag.FlagSet, args []string, version, commit, buildTime string) *Config`，或者在测试用例中使用 `flag.CommandLine = flag.NewFlagSet(...)` 隔离重置。
- 确保测试用例覆盖：
  - 纯默认参数输入；
  - `PANEL_VERSION` 环境变量生效；
  - `-panel-version` 命令行参数覆盖；
  - `Commit` 与 `BuildTime` 结构体赋值。

---

## 3. HTTP API 交付层设计 (`internal/delivery/http`)

### 3.1 `DashboardHandler` 依赖注入
`internal/delivery/http/handler_dashboard.go`：
```go
type DashboardHandler struct {
    monitorSvc   *service.MonitorService
    panelVersion string
    commit       string
    buildTime    string
}

func NewDashboardHandler(monitorSvc *service.MonitorService, panelVersion string, meta ...string) *DashboardHandler {
    h := &DashboardHandler{
        monitorSvc:   monitorSvc,
        panelVersion: panelVersion,
    }
    if len(meta) > 0 {
        h.commit = meta[0]
    }
    if len(meta) > 1 {
        h.buildTime = meta[1]
    }
    return h
}
```

### 3.2 接口响应扩展 (`/api/service/status`)
原有接口定义：
```go
func (h *DashboardHandler) GetServiceStatus(c *gin.Context) {
    status, ver, _ := h.monitorSvc.GetServiceStatus(c.Request.Context())
    c.JSON(http.StatusOK, gin.H{
        "active":       status.Active,
        "subState":     status.SubState,
        "version":      ver,
        "panelVersion": h.panelVersion,
        "commit":       h.commit,
        "buildTime":    h.buildTime,
    })
}
```
**契约设计优势**：
1. **零额外网络往返**：前端 `App.vue` 现有架构已每 6 秒轮询一次 `/api/service/status`。通过在此响应中携带 `panelVersion`，前端在首个状态返回包到达时即自动同步真实版本号，无需引入多余的独立 HTTP 请求。
2. **完全向后兼容**：不破坏既有的 `"active"`, `"subState"`, `"version"`，老旧客户端或监控脚本不会产生解析异常。

---

## 4. 前端渲染设计 (`web/src`)

### 4.1 Vite 编译期注入与兜底
在 `web/vite.config.ts` 中引入 `package.json` 的版本号并定义到编译环境变量中：
```ts
import packageJson from './package.json'

export default defineConfig({
  // ...
  define: {
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(`v${packageJson.version}`),
  },
})
```
或者在 `App.vue` 中导入/兜底：
```ts
const defaultVersion = import.meta.env.VITE_APP_VERSION || 'v2.6.6'
```

### 4.2 `App.vue` 响应式数据流
在 `web/src/App.vue` 中扩展状态管理：
```ts
const coreStatus = ref<{
  active: boolean
  version?: string
  panelVersion?: string
}>({
  active: true,
  version: '',
  panelVersion: '',
})

// 计算或响应式版本：后端返回的 panelVersion 优先，未获取或获取失败时使用编译期兜底值
const panelVersion = computed(() => coreStatus.value.panelVersion || import.meta.env.VITE_APP_VERSION || 'v2.6.6')

const fetchCoreStatus = async () => {
  if (isBlankLayout.value) return
  try {
    const res: any = await api.get('/service/status')
    if (res) {
      coreStatus.value = {
        active: !!res.active,
        version: res.version || '',
        panelVersion: res.panelVersion || coreStatus.value.panelVersion,
      }
    }
  } catch (e) {
    coreStatus.value.active = false
  }
}
```

### 4.3 模板 UI 双端对齐
1. **桌面端 Brand Header** (`web/src/App.vue:20`)：
   ```html
   <div class="flex items-center gap-1.5">
     <span class="font-mono font-semibold tracking-tight text-xs text-foreground">XRAY PANEL</span>
     <span class="text-[9px] font-mono px-1 py-0.2 bg-muted text-muted-foreground rounded border border-border/60">{{ panelVersion }}</span>
   </div>
   ```
2. **移动端 Drawer Header** (`web/src/App.vue:74-82`)：
   ```html
   <div class="h-12 flex items-center justify-between px-4 border-b border-border mb-2">
     <div class="flex items-center gap-2">
       <div class="w-6 h-6 rounded-md bg-neutral-900 border border-border flex items-center justify-center">
         <Radio class="w-3.5 h-3.5 text-neutral-200" />
       </div>
       <span class="font-mono font-semibold text-xs text-foreground">XRAY PANEL</span>
       <span class="text-[9px] font-mono px-1 py-0.2 bg-muted text-muted-foreground rounded border border-border/60">{{ panelVersion }}</span>
     </div>
     <button
       @click="isMobileDrawerOpen = false"
       class="p-1 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted"
     >
       <X class="w-4 h-4" />
     </button>
   </div>
   ```

### 4.4 Mock 模式对齐 (`web/src/mock/index.ts`)
在 `web/src/mock/index.ts:118-124`：
```ts
if (cleanUrl.endsWith('/service/status') && method === 'GET') {
  return delay({
    active: true,
    subState: 'running',
    version: 'Xray 26.3.27 (gRPC Runtime) Linux/amd64',
    panelVersion: 'v2.6.6',
  })
}
```

---

## 5. 性能、并发安全与回滚策略 (Performance & Safety)

1. **零互斥锁开销**：`cfg.PanelVersion` 与 `DashboardHandler.panelVersion` 在启动时完成一次性解析与注入，运行时只读无竞争，无需添加互斥锁。
2. **容灾降级保证**：若后端异常下线或网络抖动，前端 `panelVersion` 自动回退并保持显示编译期版本号 `v2.6.6`，页面排版徽章永不塌陷或展示空白。
3. **CI/CD 回归防护**：更新 `.github/workflows/release.yml`（`v2.6.5` -> `v2.6.6`）与 `ci.yml`（`v2.6.5-ci` -> `v2.6.6-ci`），确保代码仓库中任何场景下的默认版本均严格统一在 `2.6.6`。
