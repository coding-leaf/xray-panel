# Agent 多架构前置修复与可配置性增强 (Parent PRD)

## Goal
在推进面板向 Agent 多节点架构演进前，彻底解决当前单机/解耦面板中存在的前后端配置项脱节、编译器静默丢数据、版本号硬编码、用户与订阅配置死板不可变等问题，并通过在 `temp/` 下拉取真实 Xray-core 二进制与源码契约完成端到端真实验收。

## Child Task Map
1. `11-01-fix-config-ui-and-compiler`：修复前后端配置失配与编译器数据丢失
2. `11-01-feat-dynamic-panel-version`：面板版本号动态绑定与全链路展示
3. `11-01-enhance-user-and-sub-customizability`：用户与订阅高可定制化改造
4. `11-01-verify-xray-real-binary-acceptance`：Xray 核心源码契约对齐与真实二进制联动验收

## Confirmed Facts & File Anchors
1. **入站嗅探 `routeOnly` 缺失 UI 与 `destOverride` 硬编码**：
   - `web/src/views/inbounds/types.ts:78,143,164,494` 和 `InboundFormDrawer.vue:1034` 已处理 `sniffingRouteOnly` 序列化与回显，但 `InboundFormDrawer.vue:323-336` 模板遗漏开关控件；`types.ts:493` 硬编码 `destOverride: ['http', 'tls', 'quic']`。
2. **DNS 编译丢数据**：
   - `internal/domain/dns.go:5-10` 定义了 `ClientIP`、`DisableCache`、`DisableFallback`、`Hosts`，但 `internal/adapter/xray/schema.go:173-177` 与 `compiler.go:112-124` 未包含这些字段，保存后落盘即丢失。
3. **入站/出站/路由/设置失配**：
   - 入站缺少 `remark` 输入框（`internal/domain/inbound.go:17` 影响订阅节点名称）、顶级 `routeId` 输入框；入站/出站下拉框开放了 `httpupgrade`/`mkcp`（`InboundFormDrawer.vue:105-106`、`OutboundFormDrawer.vue:229`）但 `buildStreamSettingsJSON` 未生成对应配置；TLS `alpn` 与 Fallback `alpn`/`path` 未在前端开放。
   - 出站缺少 `tlsFingerprint` UI 选择框（`OutboundFormDrawer.vue:358-368`）、Reality `spiderX` 及 WireGuard `mtu`/`noKernelTun` 控件。
   - 路由规则编辑抽屉（`RoutingView.vue:862-954`）缺少 `user`（按用户 Email 分流）与 `tag` 字段。
   - 系统设置 `xray_geodata_dir` 在 `internal/service/geodata_service.go:57-65` 中未被读取。
4. **版本号硬编码**：
   - `web/src/App.vue:20` 硬编码 `v2.6`，移动端抽屉 `App.vue:74-82` 无版本号；`main.go:32` 定义 `Version = "v2.6.6"`，但 `internal/config/config.go` 未存入 `Config`，不支持启动参数 `-panel-version` / `PANEL_VERSION` 覆盖，也无 API 返回给前端。
5. **用户与订阅受限**：
   - 用户创建时强制自动生成 `UUID` 与 `SubToken`；编辑时 `Email`、`UUID`、`SubToken` 均不可自定义修改，`Flow` 在 `UserFormDrawer.vue` 中无控件，到期时间只能按 `+天数` 累加而非自由设定具体日期或改为永不过期，无 `Remark` 备注字段。
   - 订阅导出（`internal/sub/clash.go`、`singbox.go`、`node_converter.go`）缺乏节点排序位置控制（`sortOrder`）、客户端顶部流量/到期信息虚拟节点、节点命名模板及 Clash/Sing-box 导出参数配置。
6. **Xray 真实验收基础**：
   - `.gitignore:42` 已忽略 `temp/`；`internal/adapter/xray/xray_integration_test.go:25-99` 与 `compiler_test.go:16-49` 已有 `FindXrayBinary()` 校验机制。

## Requirements & Key Decisions
- **R1 (前后端配置对齐与编译器修复)**：补齐入站 `routeOnly`（仅限路由）开关、`destOverride` 协议多选、`remark`、`routeId`、`httpupgrade`/`mkcp` 传输层、TLS `alpn`、Fallback `alpn`/`path`；修复 DNS `clientIp`/`disableCache`/`disableFallback`/`hosts` 编译丢失；补齐出站 `tlsFingerprint`/`spiderX`/WireGuard 参数；补齐路由 `user`/`tag` 字段；打通 `GeoDataService` 读取 `xray_geodata_dir`。
- **R2 (版本号全链路动态绑定 - 方案 A)**：保持 `main.Version` 编译注入契约，新增 `-panel-version` 启动参数与 `PANEL_VERSION` 环境变量覆盖机制，通过 `/api/service/status` 返回 `panelVersion`，前端桌面端顶栏与移动端抽屉实时动态渲染（以 `package.json` 版本兜底）。
- **R3 (用户与订阅高可定制化 - 方案 A1+A2+B1 扩展)**：
  - 用户侧：支持创建/编辑时自定义或随机生成 `Email`、`UUID`、`SubToken`（修改 `Email`/`UUID` 时通过 Saga 安全移除旧 Xray gRPC 实体并同步新实体及 `SubRoute.AllowedUsers`），开放用户级 `Flow` 选择器、精确到期日期/永不过期切换及用户 `Remark` 备注。
  - 订阅与客户端展示侧：为入站与子路由增加 `sortOrder` 排序权重控制客户端显示顺序；支持在订阅列表最顶部自动注入「📊 剩余流量 / ⏳ 到期时间 / 公告」信息虚拟节点；支持自定义节点命名模板（如 `{remark}`、`{remark} [{protocol}]`）及 Clash/Sing-box 核心参数（端口、LAN、分流模式、uTLS 指纹等）。
- **R4 (Xray 真实二进制与源码契约验收)**：扩展 `FindXrayBinary()` 自动探测 `temp/xray(.exe)`，在 `temp/` 拉取官方 Xray 二进制与配置契约，使全量编译测试与 gRPC 动态增删改查测试 100% 真实运行通过。

## Cross-Child Acceptance Criteria
1. `mise x -- go test ./...` 全量通过（含 `temp/xray.exe` 驱动的真实 `xray run -test -c` 配置验证与 gRPC 集成测试，零 Skip）。
2. `mise x -- go vet ./...` 零警告，`mise x -- go build .` 编译通过。
3. `cd web && mise x -- npm run build` 零类型错误与构建错误。
4. `git status` 确认 `temp/` 被 `.gitignore` 忽略，无任何敏感信息或临时二进制污染版本库。

## Out of Scope
- 分布式多机 Agent 通信协议（gRPC Agent Server/Worker）的正式开发（将在本前置整备完成后单独展开）。
- 重度全量外部 Clash YAML / Sing-box JSON 自定义脚本引擎（本次采用 B1 结构体参数化与命名/排序/信息节点控制）。
