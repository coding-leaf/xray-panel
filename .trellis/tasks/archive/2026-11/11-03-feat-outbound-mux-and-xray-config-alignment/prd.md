# Outbound Mux 支持与 Xray 配置参数完整性对齐 (Parent Task)

## Goal

一次性消除 `xray-panel` 在重编译 `config.json` 时静默抹除高级 Xray 配置字段的架构缺陷，完整支持 Outbound `mux`（含 XUDP）、链式代理 `sockopt.dialerProxy`、多 IP 出口绑定 `sendThrough`、目标域名解析策略 `targetStrategy`、Inbound PROXY Protocol / Sniffing 排除规则、Routing 源 IP/端口分流等 Xray-core (`26.10.31`) 核心参数的前后端全链路读写、重编译无损保留与可视化编辑。

## Background & Confirmed Evidence

- **调研报告**：
  - `.trellis/tasks/11-03-feat-outbound-mux-and-xray-config-alignment/research/codebase-and-xray-binary-audit.md`
  - `.trellis/tasks/11-03-feat-outbound-mux-and-xray-config-alignment/research/xray-official-config-reference.md`
- **真机基准**：`./temp/xray.exe` (`Xray 26.10.31`)。
- **核心痛点**：
  1. `ConfigService.ListOutbounds` (`internal/service/config_service.go:428-435`) 使用残缺内联结构体解析 `config.json`，直接丢弃 `mux`、`sendThrough`、`targetStrategy`。
  2. `domain.Outbound` (`internal/domain/outbound.go:3-8`) 与 `XrayOutbound` (`internal/adapter/xray/schema.go:142-147`) 缺少 `Mux`、`SendThrough`、`TargetStrategy`。
  3. `XraySockopt` (`internal/adapter/xray/schema.go:127-131`) 缺少 `dialerProxy`（现代 Xray 唯一合法的链式代理字段，旧版 `outbound.proxySettings` 已报错退出码 23）、`acceptProxyProtocol`、`domainStrategy`、`interface`、`tcpKeepAliveInterval`、`tcpKeepAliveIdle`、`tcpUserTimeout`、`tcpCongestion`、`tcpMptcp`、`tcpNoDelay`。
  4. `XrayStreamSettings` (`internal/adapter/xray/schema.go:74-86`) 缺少 `tcpSettings` (`rawSettings`)、`splithttpSettings`。
  5. `XrayRealitySettings` (`internal/adapter/xray/schema.go:102-117`) 缺少 `password`、`target`、`mldsa65Seed`、`mldsa65Verify`、`limitFallbackUpload`、`limitFallbackDownload`。
  6. `XraySniffingConfig` (`internal/adapter/xray/schema.go:68-72`) 缺少 `metadataOnly`、`domainsExcluded`、`ipsExcluded`。
  7. `domain.RoutingRule` (`internal/domain/outbound.go:10-23`) 与 `XrayRoutingRule` (`internal/adapter/xray/schema.go:149-163`) 缺少 `source`、`sourcePort`、`balancerTag`、`domainMatcher`。
  8. 前端出站、拓扑、入站、路由视图缺少上述高频参数的表单控件与状态徽章展示。

## Child Task Map

| 子任务目录 | 独立交付物范围 | 依赖顺序 |
|---|---|---|
| `11-03-sub-1-backend-xray-schema-and-mux-compiler` | 后端 `domain`、`schema.go`、`compiler.go`、`config_service.go` 全量字段无损对齐、Mux/Vision 互斥校验及 `xray.exe` 真机单测 | 第 1 步（后端契约基座） |
| `11-03-sub-2-frontend-outbound-and-topology-mux` | 前端 Outbound 管理（Mux & XUDP、`dialerProxy`、`sendThrough`、`targetStrategy` 表单/表格徽章/详情抽屉）与拓扑中心快捷向导 Mux 支持 | 第 2 步（依赖 Sub-1 DTO 字段） |
| `11-03-sub-3-frontend-inbound-and-routing-alignment` | 前端 Inbound 管理（`acceptProxyProtocol`、Sniffing `metadataOnly`/`domainsExcluded`/`ipsExcluded`）与 Routing 管理（`source`/`sourcePort`/`domainMatcher`） | 第 3 步（依赖 Sub-1 DTO 字段） |

## Cross-Child Requirements

- **R1 (Backend Lossless Recompile)**: Outbound (`mux`, `sendThrough`, `targetStrategy`), StreamSettings (`tcpSettings`, `rawSettings`, `splithttpSettings`, `sockopt.*`), REALITY (`password`, `target`, `mldsa65*`, `limitFallback*`), Sniffing (`metadataOnly`, `domainsExcluded`, `ipsExcluded`), RoutingRule (`source`, `sourcePort`, `balancerTag`, `domainMatcher`) 在 `List* -> Save* -> CompileToJSON` 全链路 100% 无损保留。
- **R2 (Mux + Vision Mutual Exclusivity)**: 当 Outbound 为 VLESS 且 `flow == "xtls-rprx-vision"` 时，后端编译器自动抑制输出 `mux.enabled: true`（或剥离启用态 Mux），前端表单提供实时互斥提示与联动。
- **R3 (Frontend Outbound & Topology UX)**: 出站抽屉提供清晰的“连接多路复用 (Mux & XUDP)”与“高级链路选项 (`dialerProxy` / `sendThrough` / `targetStrategy`)”配置卡片；列表与详情抽屉直观展示 `MUX` 与链式代理徽章；拓扑向导支持导入落地节点时一键开启 Mux。
- **R4 (Frontend Inbound & Routing UX)**: 入站抽屉支持 `acceptProxyProtocol` 与高级 Sniffing（`metadataOnly`、排除域名/IP）；路由规则抽屉支持客户端源 IP (`source`)、源端口 (`sourcePort`) 与域名匹配器 (`domainMatcher`)。

## Out of Scope

- 远端服务器 `vps-jp` 的 SSH 运维操作（已确认为误粘贴）。
- 已被 Xray-core 26.10.31 官方废弃并报错的 `outbound.proxySettings`（统一采用官方新标准 `streamSettings.sockopt.dialerProxy`）。

## Acceptance Criteria

- [ ] AC1: 后端 `mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .` 全绿，且 `internal/adapter/xray` 新增的往返编译测试与 `./temp/xray.exe run -test` 真机校验全部通过。
- [ ] AC2: 前端 `cd web && mise x -- npm run build` 零错误通过（含 `vue-tsc --noEmit` 类型检查）。
- [ ] AC3: 三个子任务全部独立完成开发、通过 `trellis-check` 验收并归档。
