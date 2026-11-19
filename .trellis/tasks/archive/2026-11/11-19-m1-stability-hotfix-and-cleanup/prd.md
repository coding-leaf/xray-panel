# M1: 核心稳定性止血修复、拓扑一键发布剔除与快速去重

> **Parent Task**: `11-19-audit-today-stability-cleanup`

## Goal
在不改动异步指令通道协议（留待 M2）和整体页面信息架构（留待 M3）的前提下，一次性止血修复所有高确定性 P0/P1 稳定性与安全缺陷，按产品决策彻底移除冗余的「拓扑一键发布」功能，清理可直接消除的死代码与重复实现，并恢复前端 Prettier 与工作区干净状态。

## Requirements

### R1. 修复远端入站破坏主控用户写操作与 TG `/adduser`（P0-1）
- **证据与锚点**：`internal/adapter/xray/manager.go:36-66` 未过滤 `inbound.NodeID != 0`；`internal/service/user_service.go:101-108` 不忽略 `handler not found`；`internal/adapter/telegram/bot.go:385-395` `/adduser` 授权所有启用入站；`internal/delivery/http/agent_e2e_test.go:170,581` 传 `xrayManager = nil` 造成盲区。
- **要求**：
  - 在 `domain.Inbound` 提供纯语义方法 `IsLocal() bool`（`NodeID == 0`）。
  - `xray.Manager.AddUser` 与 `RemoveUser` 遇到 `!inbound.IsLocal()` 时直接无副作用返回 `nil`，仅对本机入站发起 gRPC 调用。
  - 保留用户与 TG `/adduser` 为远端入站授权的数据库能力（远端由 Agent `UserSync` 同步）。
  - 补充失败复现先行的单元测试与非空 `xrayManager` E2E 回归测试，验证同时存在本机 + 远端启用入站时 `CreateUser` / `UpdateUser` / `ResetTraffic` / `ResetSubToken` / `DeleteUser` 及 TG `/adduser` 均成功且绝不对远端 `tag` 调用本机 gRPC。

### R2. 统一远端配置编译的 `SubRoute` 规范化管道与缓存失效（P1-2 & P1-6）
- **证据与锚点**：`internal/service/config_service.go:1165` 读取 Routing UI 时调用 `ensureSubRouteRules`，但 `internal/service/node_service.go:886-914` (`getOrBuildSnapshot`) 与 `internal/service/config_service.go:1471-1480` (`GetNodeCompiledConfigRaw`) 缺失该补全；`config_service.go:147-157, 175-188` (`RollbackSnapshot` / `saveAndApplyRawConfigLocked`) 漏调 `invalidateAllNodeCaches()`；`config_service.go:353` (`syncFromRawJSONLocked`) 未校验 `existing.NodeID == 0`。
- **要求**：
  - 抽离共享的远端配置规范化/编译入口，使 `NodeService.getOrBuildSnapshot` 与 `ConfigService.GetNodeCompiledConfigRaw` 在编译远端全量 JSON 前统一执行 `ensureSubRouteRules`，保证 Routing UI、ConfigView 预览与 Agent 实得 `config.json` 三者完全一致。
  - `saveAndApplyRawConfigLocked`（覆盖 Raw 编辑保存与 `RollbackSnapshot`）完成后调用 `invalidateAllNodeCaches()`。
  - `syncFromRawJSONLocked` 若检测到目标 `tag` 属于远端入站（`existing.NodeID != 0`），显式报错阻断，禁止主控 Raw 编辑篡改远端入站记录。

### R3. 修复远端 Shadowsocks / SS-2022 增量热加载协议失配（P1-3）
- **证据与锚点**：`internal/adapter/xray/compiler.go:440-454` (`BuildRemoteNodeUsers`) 未透传加密 `Method`；`internal/agent/xray_grpc_adapter.go:37-53` 构造空 `SettingsJSON`，导致 `ResolveAdapter` 退化为 `aes-128-gcm`。
- **要求**：
  - 在 `domain.AgentSyncUser` 扩展向后兼容的可选字段 `Method string`（`json:"method,omitempty"`）。
  - `BuildRemoteNodeUsers` 对 Shadowsocks / SS-2022 入站解析并填充真实 `Method`。
  - `agent/xray_grpc_adapter.go` 依据 `u.Method` 构造有效 `SettingsJSON` 供 `ResolveAdapter` 正确路由 `Shadowsocks2022Adapter` 或对应加密套件的 `ShadowsocksAdapter`。

### R4. 修复远端直连入站未填 `ExternalPort` 被覆写为主控 `443`（P1-4）
- **证据与锚点**：`internal/service/sub_service.go:81-99` (`prepareInboundForNode`) 未处理远端 `ExternalPort <= 0`，导致 `internal/protocol/node_converter.go:153-158` 回退至主控 `public_port`（`443`）。
- **要求**：
  - 在 `prepareInboundForNode` 中，当 `inbound.NodeID > 0 && ibCopy.ExternalPort <= 0` 时，将 `ibCopy.ExternalPort` 设为 `inbound.Port`（本机入站继续沿用主控 `public_port` 兜底行为，不破坏现有契约）。

### R5. 彻底移除「拓扑一键发布」功能（替代 P1-5）
- **产品决策**：用户明确要求移除拓扑一键发布功能，而非仅修补 `?nodeId` 传参。
- **证据与锚点**：`web/src/views/topology/components/QuickPublishWizardModal.vue`、`web/src/views/TopologyView.vue:72,76,108,126,137-138,159,230`、`web/src/views/topology/components/TopologyCanvas.vue:210,261,386`、`web/src/views/topology/components/TopologyPoolsView.vue:166,372`、`web/src/views/topology/components/OutboundNodeCard.vue`（`@publish` 按钮/事件）。
- **要求**：
  - 删除 `QuickPublishWizardModal.vue` 文件及 `TopologyView`、`TopologyCanvas`、`TopologyPoolsView`、`OutboundNodeCard` 中全部一键发布按钮、事件、状态、无用 import 与文案；保留拓扑画布/分组卡片池浏览、新建接入网关、新建落地出口、通道切换/删除与日志跳转等正常能力。

### R6. 安装脚本凭证安全加固与默认间隔/端口一致性（P1-8）
- **证据与锚点**：`internal/service/node_service.go:685` (`RenderInstallScript`) 明文 `--token` CLI 传参且残留 `--interval 20s`；`cmd/agent/main.go:47` 与 `internal/agent/runner.go:11` 仍默认 `20s`（与 `defaultNodeSyncIntervalSec = 60` 漂移）；`internal/adapter/xray/compiler.go:403` 硬编码远端 Xray gRPC 端口 `8080`。
- **要求**：
  - `RenderInstallScript` 将 `MASTER_URL` 与 `AGENT_TOKEN` 写入权限 `0600` 的 `/etc/xray-agent/agent.env`，在 systemd service 中通过 `EnvironmentFile=/etc/xray-agent/agent.env` 注入，移除 `ExecStart` 中的 `--token` 明文参数。
  - 将 `RenderInstallScript`、`cmd/agent/main.go:47` 与 `internal/agent/runner.go:11` 的默认心跳同步间隔统一为 `60s`（与 Master 默认值一致）。
  - 在 `cmd/agent/main.go` flag 说明与安装脚本注释中明确远端编译固定使用本地 gRPC `127.0.0.1:8080`（并在 `compiler.go` 抽取命名常量 `remoteAPIPort = 8080` 与明确注释，避免魔数漂移）。

### R7. 快速冗余清理、代码去重与门禁基线修复
- **死代码删除**：删除 `internal/service/user_service.go:110` 零调用函数 `isXrayUserNotFoundError`。
- **重复逻辑收敛**：
  - 将 `FindByNodeID(ctx context.Context, nodeID uint) ([]domain.Inbound, error)` 正式纳入 `domain.InboundRepository`（`internal/adapter/repository/inbound_repo.go:56` 已实现），删除 `node_service.go:44,831-849` 与 `config_service.go:1490-1508` 两份重复的 `nodeInboundFinder` / `findNodeInbounds` / `findInboundsByNode`（同步补齐相关测试 mock）。
  - 让 `internal/agent/quota_store.go:59-83` 直接调用 `domain.CalcTrafficByMode` 与 `domain.ReachedQuotaThreshold`，删除重复公式实现（保持 `cmd/agent` 零重型依赖）。
  - 让 `internal/delivery/http/handler_inbound.go:38-51` 复用同包 `parseOptionalNodeID`（`handler_config.go:17`）。
  - 删除 `web/src/views/PortalClaimView.vue:368-394` 手写 `copyToClipboard`，改为导入复用 `web/src/utils/clipboard.ts`。
- **前端格式与工作区卫生**：
  - 对 `web/` 执行 Prettier 格式化修复 24 个文件格式失败，使 `npm run format:check` 100% 通过。
  - 恢复未提交的 `.trellis/.template-hashes.json`（清除 20 条 `__pycache__/*.pyc` 哈希污染）。

## Out of Scope
- 异步 `NodeCommand` 通道重构（P0-2 & P1-1）与流量表定时清理/级联删除（P1-7）及 P2-1 — 归属 **M2**。
- 全局节点切换器作用域、节点运维抽屉、GeoData 迁出 `RoutingView`、`useNodes`/`useActiveNode` 合并、Portal HTML 单源化、`NodeRepository` 4 接口收敛及 P2-2/3/4 — 归属 **M3**。

## Acceptance Criteria
1. **AC-1 (P0-1)**：存在已启用远端入站 (`NodeID > 0`) 与本机入站 (`NodeID == 0`) 时，启用非空 `xrayManager` 执行 `CreateUser`、`UpdateUser`、`ResetTraffic`、`ResetSubToken`、`DeleteUser` 及 TG `/adduser` 均成功，且本机 gRPC 仅收到 `NodeID == 0` 的 tag 调用。
2. **AC-2 (P1-2 & P1-6)**：为远端入站配置 `SubRoute` 后，无需打开/保存 `RoutingView`，`NodeService` 下发快照与 `ConfigView` 预览均包含对应 `SubRoute` 路由规则；主控执行 Raw JSON 保存或快照回滚后，远端快照缓存失效重建；Raw JSON 包含与远端入站同名 `tag` 时报错拒绝覆盖。
3. **AC-3 (P1-3)**：远端 `shadowsocks`（含 SS-2022 如 `2022-blake3-aes-128-gcm` 与 `chacha20-ietf-poly1305`）用户同步负载携带 `Method`，Agent `XrayGRPCAdapter` 正确解析对应协议适配器而非退化为 `aes-128-gcm`。
4. **AC-4 (P1-4)**：远端入站 `ExternalPort == 0` 且 `Port == 18443` 时，订阅生成端口为 `18443`；远端入站显式设置 `ExternalPort == 2053` 时为 `2053`；本机入站 `ExternalPort == 0` 时维持原有 `public_port` 回退逻辑。
5. **AC-5 (R5)**：`QuickPublishWizardModal.vue` 已删除，全项目 `web/src/` 中无 `QuickPublishWizardModal` / `openWizard` / `1步发布` 残留，拓扑画布与分组卡片池正常渲染构建无告警。
6. **AC-6 (P1-8)**：安装脚本生成 `0600` `/etc/xray-agent/agent.env` + `EnvironmentFile=`，`ExecStart` 不含 `--token` 明文，且安装脚本、`cmd/agent`、`internal/agent/runner.go` 默认间隔均为 `60s`。
7. **AC-7 (R7 & Gates)**：`isXrayUserNotFoundError`、`findNodeInbounds`/`findInboundsByNode`、`quota_store` 重复计费公式、`PortalClaimView` 私有复制函数均已移除/去重；`.trellis/.template-hashes.json` 无 `.pyc` 污染；以下全量门禁全绿：
   - `mise x -- go test ./...`
   - `CGO_ENABLED=1 mise x -- go test -race ./...`
   - `mise x -- go vet ./...`
   - `mise x -- go build .`
   - `go list -deps ./cmd/agent | Select-String "gin|gorm|sqlite|telegram"` 输出为空
   - `cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run format:check && mise x -- npm run build`
