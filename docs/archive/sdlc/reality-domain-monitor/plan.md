# Plan: Reality 域名合规性定期监测与面板告警 - 实施计划

- **关联 Spec**: reality-domain-monitor
- **实施执行人 / Agent**: Dev
- **当前状态**: Draft
- **Change Tier**: Tier 2 (单模块特性演进)

---

## 1. 变更文件清单 (Files that change)

### 领域模型与核心纯函数 (Domain Layer)
* `internal/domain/reality.go` (New: 定义 `RealityDomainStatus`, `RealityCheckItem`, `RealitySummaryStatus` 领域结构与状态枚举)
* `internal/domain/reality_evaluator.go` (New: 提取入站 Reality 配置纯解析器 `ExtractRealityTargets` 与无副作用判定纯函数核 `EvaluateRealityProbe`)

### 探测适配器与服务编排 (Adapter & Service Layer)
* `internal/adapter/reality/prober.go` (New: 实现 `RealityProber` 接口 `DefaultRealityProber`，基于标准库 `net.Dialer` 与 `crypto/tls` 执行非阻塞握手)
* `internal/service/reality_monitor_service.go` (New: 实现 `RealityMonitorService`，维护并发安全读写锁状态缓存，编排批量受限探测与状态聚合)
* `internal/service/alert_service.go` (Modify: 新增 `NotifyRealityAbnormal` 方法，基于 24 小时内存缓存去重冷却，通过 Telegram 推送异常卡片)
* `internal/delivery/cron/reality_sync.go` (New: 实现 `RealitySyncJob`，实现 `app.Service` 契约，支持 12 小时默认周期的后台定期巡检)
* `main.go` (Modify: 实例化 `RealityMonitorService` 与 `RealitySyncJob`，挂载进 `errgroup` 统一生命周期管理)

### HTTP 传输层与路由集成 (Delivery Layer)
* `internal/delivery/http/handler_inbound.go` (Modify: 新增 `GetRealityStatus` 与 `TriggerRealityCheck` 处理函数，映射至 `RealityMonitorService`)
* `internal/delivery/http/router.go` (Modify: 在 `authGroup` 注册 `/inbounds/reality-status` 与 `/inbounds/reality-status/check` 路由并保留 `/v1` 别名)

### 前端交互与状态呈现 (Web Layer)
* `web/src/api/index.ts` (Modify: 补充 Mock 数据路由处理，避免开发/离线模式报错)
* `web/src/api/reality.ts` (New: 封装 `getRealityStatus` 与 `checkRealityStatus` 的 API 请求方法与 TypeScript 类型定义)
* `web/src/views/InboundsView.vue` (Modify: 顶部增加“检测伪装域名”触发按钮，卡片区展示 Reality 状态徽标与 Tooltip 详情)
* `web/src/views/DashboardView.vue` (Modify: 顶部动态展示 Reality 域名异常警示横条，支持一键导航至入站管理)

### 单元测试与集成测试 (Tests)
* `internal/domain/reality_evaluator_test.go` (New: 纯算法核穷尽单测，覆盖 TLS < 1.3、ALPN 缺失、证书过期/7天临期/域名不匹配、Cloudflare CDN 特征、TCP 超时等)
* `internal/adapter/reality/prober_test.go` (New: 基于本地 `httptest.NewTLSServer` 验证 TLS 1.3 探测与超时熔断)
* `internal/service/reality_monitor_service_test.go` (New: 并发安全测试与告警联动 Mock 验证，验证 `-race` 全绿)
* `internal/service/alert_service_test.go` (Modify: 补充 `NotifyRealityAbnormal` 冷却去重机制单测)
* `internal/delivery/http/handler_reality_test.go` (New: 验证 GET/POST 端点的鉴权、状态返回与错误处理)

---

## 2. 伴随式分步实施与验证 (Step-by-Step Implementation Loops)

### Milestone 1: 领域模型、合规评估纯函数核与网络探针实现 (M1)
> 目标：实现完全解耦、零外部网络依赖的规则推导核与标准库探测器，并通过穷尽单元测试。

* **Step 1.1 (领域模型定义)**:
  - 新建 `internal/domain/reality.go`；
  - 定义 `RealityDomainStatus` 枚举（`ok`, `warning`, `error`）；
  - 定义 `RealityCheckItem` 与 `RealitySummaryStatus` 结构体；
  - 局部验证命令: `go test -race -v ./internal/domain`
  - 预期判据: 编译无误，包测试通过。

* **Step 1.2 (配置提取与合规评估纯函数核实现 - Fail-repro First)**:
  - 新建 `internal/domain/reality_evaluator_test.go`，编写测试用例覆写各项规则（TLS 1.2 拒绝、缺失 ALPN 警告、证书临期 5 天警告、证书过期 1 天报错、Cloudflare 响应头与证书报错、TCP 握手失败报错）；
  - 运行单测确认测试报红（Red）；
  - 新建 `internal/domain/reality_evaluator.go`，实现 `ExtractRealityTargets(inbound *domain.Inbound)` 与 `EvaluateRealityProbe(tcpErr error, tlsVer uint16, alpn string, cert *x509.Certificate, headers http.Header, now time.Time)`；
  - 严格按确定性优先级返回 `status`, `errorType`, `details`；
  - 局部验证命令: `go test -race -v -run TestEvaluateRealityProbe ./internal/domain`
  - 预期判据: 纯函数测试用例全部变绿（Pass）。

* **Step 1.3 (标准库受限网络探针实现)**:
  - 新建 `internal/adapter/reality/prober.go`，定义接口 `RealityProber` 并实现 `DefaultRealityProber`；
  - 使用 `net.Dialer{Timeout: 3*time.Second}` 与 `tls.Client{InsecureSkipVerify: true}` 进行握手；
  - 发送轻量 HTTP GET 请求以抓取 HTTP 响应头（用于检测 `server: cloudflare` 或 `cf-ray`）；
  - 新建 `internal/adapter/reality/prober_test.go`，使用本地 `httptest.NewTLSServer` 模拟 TLS 1.3 服务器并验证探测逻辑；
  - 局部验证命令: `go test -race -v ./internal/adapter/reality`
  - 预期判据: 探针握手与超时机制验证通过。

---

### Milestone 2: 巡检服务 RealityMonitorService、读写锁缓存与告警联动 (M2)
> 目标：实现高并发只读、后台受控并发探测、内存状态聚合及 Telegram 告警冷却。

* **Step 2.1 (AlertService 告警联动与冷却)**:
  - 修改 `internal/service/alert_service.go`，增加 `NotifyRealityAbnormal(ctx context.Context, item domain.RealityCheckItem) error`；
  - 采用 `reality_alert:<inboundTag>:<serverName>:<errorType>` 作为缓存键，设置 24 小时冷却；
  - 格式化 Telegram 告警消息（包含 InboundTag、域名、错误类型、状态与建议）；
  - 局部验证命令: `go test -race -v -run TestNotifyRealityAbnormal ./internal/service`
  - 预期判据: 首次异常成功发送，24 小时内重复异常被冷却拦截。

* **Step 2.2 (RealityMonitorService 编排与读写锁缓存)**:
  - 新建 `internal/service/reality_monitor_service.go`；
  - 内置 `sync.RWMutex` 保护 `RealitySummaryStatus` 缓存，`GetStatus()` 亚毫秒并发只读；
  - 实现 `CheckAll(ctx context.Context)`：读取入站列表，并发度限制（最大 5 个并发 worker），每个目标超时 3s，探测结果送入 `EvaluateRealityProbe`；
  - 检测到 Warning/Error 时异步触发 `alertSvc.NotifyRealityAbnormal`；
  - 编写 `internal/service/reality_monitor_service_test.go`，模拟 MockProber 验证并发读写与竞态安全；
  - 局部验证命令: `go test -race -v -run TestRealityMonitorService ./internal/service`
  - 预期判据: 并发读写测试通过，无数据竞态（`-race` 无告警）。

* **Step 2.3 (后台 12 小时定时任务与生命周期接入)**:
  - 新建 `internal/delivery/cron/reality_sync.go`，定义 `RealitySyncJob` 满足 `app.Service` 接口；
  - 修改 `main.go`，注入 `RealityMonitorService` 与 `RealitySyncJob`，纳入 `errgroup` 统一调度并在启动后异步触发初次预检；
  - 局部验证命令: `go build .`
  - 预期判据: 整体编译构建成功，依赖注入无缺失。

---

### Milestone 3: HTTP API 端点接入与路由集成 (M3)
> 目标：对外暴露受 JWT 保护的巡检状态查询与手动立即检测接口。

* **Step 3.1 (Handler 方法实现与路由装配)**:
  - 修改 `internal/delivery/http/handler_inbound.go`，注入 `RealityMonitorService`，新增 `GetRealityStatus` 与 `TriggerRealityCheck`；
  - 修改 `internal/delivery/http/router.go`，在管理员 `authGroup` 下注册：
    - `GET /inbounds/reality-status` (并支持 `/v1/inbounds/reality-status`)
    - `POST /inbounds/reality-status/check` (并支持 `/v1/inbounds/reality-status/check`)
  - 局部验证命令: `go test -race -v -run TestInbound ./internal/delivery/http`
  - 预期判据: 现有 Inbound 测试不退化，代码编译无误。

* **Step 3.2 (HTTP 接口契约测试)**:
  - 新建 `internal/delivery/http/handler_reality_test.go`；
  - 验证未授权访问返回 401 Unauthorized；
  - 验证管理员访问 GET 返回 200 及 `RealitySummaryStatus` JSON 结构；
  - 验证 POST 触发成功返回 200 并更新状态；
  - 局部验证命令: `go test -race -v -run TestRealityHandler ./internal/delivery/http`
  - 预期判据: API 端点集成测试 100% 绿灯。

---

### Milestone 4: 前端界面交互 (InboundsView / DashboardView) 与构建验证 (M4)
> 目标：在入站管理与仪表盘呈现可视化状态与手动巡检能力，杜绝界面阻塞。

* **Step 4.1 (前端 API 封装与类型声明)**:
  - 新建 `web/src/api/reality.ts`，导出 `RealityCheckItem`, `RealitySummaryStatus` 类型及 API 调用函数；
  - 在 `web/src/mock/index.ts` 补充 mock 响应，确保离线或无后端环境正常展示；
  - 局部验证命令: `cd web && npx vue-tsc --noEmit`
  - 预期判据: TypeScript 静态类型检查零报错。

* **Step 4.2 (InboundsView 节点状态徽标与手动刷新)**:
  - 修改 `web/src/views/InboundsView.vue`；
  - 在页头操作区增加“检测伪装域名”按钮（带 `animate-spin` 旋转指示）；
  - 在 Reality 节点卡片内展示状态徽标：
    - 绿色 `ok`: “Reality 正常”
    - 橙色 `warning`: “Reality 预警”
    - 红色 `error`: “Reality 异常”
  - Tooltip 展示 dest、serverName、剩余证书天数及详细失败原因；
  - 局部验证命令: `cd web && npm run build`
  - 预期判据: 前端构建成功，无打包错误。

* **Step 4.3 (DashboardView 告警提示横条)**:
  - 修改 `web/src/views/DashboardView.vue`；
  - 加载仪表盘时获取 Reality 巡检摘要；
  - 若 `errorCount > 0` 或 `warningCount > 0`，在顶部渲染警告 Banner：“检测到 X 个 Reality 节点伪装域名异常/临期，点击查看详情”，点击路由至 `/inbounds`；
  - 局部验证命令: `cd web && npm run build`
  - 预期判据: 打包构建无警告错误。

---

## 3. 全局质量门禁核验 (Global Quality Gate)

* **静态检查**: `go vet ./...`
* **全量测试与并发竞态核验**: `go test -race ./...` (必须全绿)
* **后端二进制编译**: `go build .`
* **前端生产构建打包**: `cd web && npm run build`
* **核验结果预期**: 所有测试与构建命令零失败、零新增警告。

---

## 4. 七维动态风险核验与回滚预案 (Risk Matrix & Rollback Playbook)

### 4.1 七维风险核验矩阵
| 风险维度 | 评估结果 | 防御与隔离措施 |
| :--- | :--- | :--- |
| **1. Affected Files** | 低 (7 个后端文件, 3 个前端文件) | 严格限定在巡检服务与呈现组件内，不侵入 Xray 核心生成逻辑。 |
| **2. Public API & Protocol** | 低 (仅新增 2 个端点) | 完全遵循现行 RESTful 命名，现有接口契约零破坏。 |
| **3. Data Schema** | 零风险 (无数据库改动) | 状态全在内存读写锁缓存中维护，不污染 SQLite 表。 |
| **4. Auth & Security** | 低 (JWT 鉴权保护) | 接口受 Admin JWT 保护；探测超时 3s，Worker 并发受限，防 SSRF 放大。 |
| **5. Dependencies** | 零风险 (零新增依赖) | 探测完全基于 Go 标准库 `crypto/tls` 与 `net`。 |
| **6. Rollback Difficulty** | 极低 (易于回滚) | 纯增量特性，前端有空安全防护，后端直接注销路由与 Job 即可还原。 |
| **7. Blast Radius** | 零风险 (转发进程隔离) | 探针仅为外部网络探测，绝对不篡改入站配置，不重启或影响核心转发。 |

### 4.2 回滚清单 (Rollback Playbook)
1. **代码级快速回滚**：
   - 使用 `git checkout` 撤销 `main.go`、`router.go` 及相关变更文件；
   - 重新执行 `go build .` 与 `cd web && npm run build`。
2. **生产运行时应急降级**：
   - 若外网探测对宿主网络产生额外负载，可在 `main.go` 中注释 `RealitySyncJob` 的后台调度，退化为仅提供被动查询。

---

## 5. 实施偏差记录 (Deviations Log)
* [修复前端 API 响应解包与兜底]: 后端按照 spec.md 返回 `{code: 0, msg: "...", data: summary}` 结构，前端 axios 拦截器默认未穿透解包 `data` 属性导致读取 `totalChecked` 为 `undefined`（引发 Toast 显示 `undefined` 目标）。已在 `web/src/api/reality.ts`、`InboundsView.vue` 与 `DashboardView.vue` 增加自适应解包与空值安全兜底，彻底解决该问题。

---

## 6. 阶段准出签批 (Gate 3 Sign-off)
- [x] 所有分步实施项与验证断言均已就地执行并通过
- [x] 全局质量门禁（Lint / Type / Regression）全部绿灯
- [x] 变更文件与 plan.md 清单完全吻合，无越权修改
- **验收结论**: Accepted
- **验证人 / 日期**: User / 2026-09-20 15:55
