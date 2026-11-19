# M1 Technical Design: 核心稳定性止血修复、拓扑一键发布剔除与快速去重

## 1. Architecture & Boundaries
本次 M1 严格遵守分层边界，不引入新数据库表、不破坏对外 REST/Agent API 既有兼容性：
- `internal/domain`（纯模型与仓储接口，零外部依赖）：
  - `inbound.go` 新增 `func (i Inbound) IsLocal() bool { return i.NodeID == 0 }`。
  - `node.go` 的 `AgentSyncUser` 新增可选字段 `Method string \`json:"method,omitempty"\``。
  - `repository.go` 的 `InboundRepository` 新增 `FindByNodeID(ctx context.Context, nodeID uint) ([]Inbound, error)`。
- `internal/adapter/xray`：
  - `manager.go`：`AddUser` 与 `RemoveUser` 查询入站后立刻判定 `if !inbound.IsLocal() { return nil }`，杜绝向主控本机 Xray gRPC 发送远端入站 tag。
  - `compiler.go`：`BuildRemoteNodeUsers` 从入站 `SettingsJSON` 提取 `Method`（复用 `ResolveShadowsocksMethod()`，仅对 Shadowsocks/SS-2022 填充）；提取 `const remoteAPIPort = 8080` 消除魔数。
- `internal/agent` & `cmd/agent`：
  - `xray_grpc_adapter.go`：构造合成 `domain.Inbound` 时，若 `u.Method != ""` 则序列化 `{"method": u.Method}` 写入 `SettingsJSON`（仅在需要时构造最小 JSON），使 `ResolveAdapter` 命中正确的 SS / SS-2022 适配器。
  - `quota_store.go`：删除本地重复的 `CalcTrafficByMode` 与阈值判断逻辑，直接调用 `domain.CalcTrafficByMode` / `domain.ReachedQuotaThreshold`（`domain` 仅依赖 stdlib，完全不破坏 `cmd/agent` 轻量依赖边界）。
  - `runner.go` & `cmd/agent/main.go`：将默认 `interval` 统一由 `20s` 调整为 `60s`。
- `internal/service`：
  - `config_service.go` & `node_service.go`：
    - 抽离包内共享纯/辅助函数 `normalizeNodeRouting(ctx, rawRouting, nodeInbounds)`（复用 `ensureSubRouteRules` 逻辑），在 `NodeService.getOrBuildSnapshot` 与 `ConfigService.GetNodeCompiledConfigRaw` 编译配置前统一调用，保证 UI、Preview 与 Agent Snapshot 同源。
    - `ConfigService.saveAndApplyRawConfigLocked` 末尾成功后调用 `s.invalidateAllNodeCaches()`（同时覆盖 `SaveRawConfig` 与 `RollbackSnapshot`）。
    - `ConfigService.syncFromRawJSONLocked` 反查 `existing, err := s.inboundRepo.GetByTag(ctx, tag)` 时，若 `existing != nil && !existing.IsLocal()`，返回明确的校验错误（禁止主控 Raw 编辑覆盖远端入站）。
    - 移除 `nodeInboundFinder`、`findNodeInbounds`、`findInboundsByNode`，直接调用 `inboundRepo.FindByNodeID(ctx, nodeID)`。
  - `user_service.go`：删除零调用死函数 `isXrayUserNotFoundError`。
  - `sub_service.go`：`prepareInboundForNode` 中若 `!ibCopy.IsLocal() && ibCopy.ExternalPort <= 0`，设为 `ibCopy.ExternalPort = ibCopy.Port`。
  - `node_service.go` (`RenderInstallScript`)：生成 `umask 077` / `chmod 600` 的 `/etc/xray-agent/agent.env`（含 `MASTER_URL=...` 与 `AGENT_TOKEN=...`），systemd unit 使用 `EnvironmentFile=/etc/xray-agent/agent.env`，`ExecStart=/usr/local/bin/xray-agent --spool-dir /var/lib/xray-agent --interval 60s`（不含明文 `--token`）。
- `internal/delivery/http`：
  - `handler_inbound.go` 删除内联 `nodeId` 解析逻辑，直接复用同包 `parseOptionalNodeID(c)`。
- `web/`：
  - 彻底删除 `web/src/views/topology/components/QuickPublishWizardModal.vue`，清理 `TopologyView.vue`、`TopologyCanvas.vue`、`TopologyPoolsView.vue`、`OutboundNodeCard.vue` 的调用、事件、状态、无用 icon import 与提示文案。
  - `web/src/views/PortalClaimView.vue` 删除手写 `copyToClipboard`，改为导入 `web/src/utils/clipboard`。
  - 全量运行 Prettier 格式化修复 24 个前端文件，并执行生产构建同步门禁。

## 2. Data Flow & Compatibility
- **AgentSyncUser 向后兼容**：新增 `Method string \`json:"method,omitempty"\`` 为可选 JSON 字段；旧版 Agent 会自动忽略未知字段，新版 Agent 若收到空 `Method` 则保持既有兜底逻辑。
- **Install Script 兼容性**：`cmd/agent/main.go:39-40` 已原生支持 `MASTER_URL` 与 `AGENT_TOKEN` 环境变量，systemd `EnvironmentFile=/etc/xray-agent/agent.env` 无缝生效，同时保留 CLI flags 兼容手动调试。
- **InboundRepository 接口变更影响面**：仅需确保测试包中所有实现 `domain.InboundRepository` 的 mock struct 补齐 `FindByNodeID(ctx context.Context, nodeID uint) ([]domain.Inbound, error)` 方法（编译期即能 100% 发现遗漏）。
- **SubRoute 规范化一致性**：`ensureSubRouteRules` 已有成熟实现，复用时务必确保不污染传入切片且不依赖 `ConfigService` 本机状态（对远端节点传 `nodeInbounds` 与对应 `rawRouting`）。

## 3. Fail-Repro First Testing Strategy
对每项修复严格执行先写失败用例验证复现，再编写修复代码转绿：
1. **P0-1**：在 `internal/adapter/xray/manager_test.go` 新增测试验证 `NodeID > 0` 入站调用 `AddUser`/`RemoveUser` 不触达 gRPC；在 `internal/delivery/http/agent_e2e_test.go` 新增带非空 `xrayManager` 桩的回归用例覆盖 `CreateUser`/`UpdateUser`/`ResetTraffic`/`ResetSubToken` 与 TG `/adduser`。
2. **P1-2 & P1-6**：在 `internal/service/node_service_test.go` & `config_service_test.go` 新增用例验证：① 远端入站配置 `SubRoute` 后 `getOrBuildSnapshot` 与 `GetNodeCompiledConfigRaw` 均包含路由规则；② `RollbackSnapshot` / `SaveRawConfig` 触发节点缓存失效；③ Raw JSON 写入远端入站同名 `tag` 返回错误。
3. **P1-3**：在 `internal/adapter/xray/compiler_test.go` 与 `internal/agent/xray_grpc_adapter_test.go` 新增 SS-2022 (`2022-blake3-aes-128-gcm`) 与 `chacha20-ietf-poly1305` 增量用户同步测试。
4. **P1-4**：在 `internal/service/sub_service_test.go` 新增远端直连入站 `ExternalPort=0, Port=18443` 订阅导出端口为 `18443` 的用例，并验证本机入站回退 `public_port` 不回归。
5. **P1-8**：在 `internal/service/node_service_test.go` 更新/新增 `RenderInstallScript` 断言：包含 `/etc/xray-agent/agent.env`、`chmod 600`、`EnvironmentFile=`、`--interval 60s`，且 `ExecStart` 行不包含 `--token`。

## 4. Parallel Ownership & Rollback Points
- **Slice 1 (Backend + Agent)**：独占 `internal/**` 与 `cmd/agent/**`（不碰 `web/**`）。
- **Slice 2 (Frontend)**：独占 `web/**`（不碰 `internal/**` 与 `cmd/**`）。
- **Rollback Points**：所有修改保持未提交于工作区（Phase 3.4 统一评审后再提交），任一 Slice 均可独立回滚；`.trellis/.template-hashes.json` 单独在主会话恢复，不与业务改动混杂。
