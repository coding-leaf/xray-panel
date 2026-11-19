# M1 Implementation Plan

## Execution Slices (File-Disjoint)

### Slice A: Backend & Agent 稳定性止血与去重
**Owned Paths**: `internal/**`, `cmd/agent/**`

- [ ] **A1. 领域模型与仓储接口收敛 (R1, R3, R7)**
  - `internal/domain/inbound.go`：新增 `func (i Inbound) IsLocal() bool { return i.NodeID == 0 }`。
  - `internal/domain/node.go`：`AgentSyncUser` 增加 `Method string \`json:"method,omitempty"\``。
  - `internal/domain/repository.go`：`InboundRepository` 增加 `FindByNodeID(ctx context.Context, nodeID uint) ([]Inbound, error)`；补齐所有测试中 `InboundRepository` mock 实现。
- [ ] **A2. 修复 P0-1（远端入站破坏主控用户写操作与 TG `/adduser`）**
  - Fail-Repro：新增 `internal/adapter/xray/manager_test.go` 与 `internal/delivery/http/agent_e2e_test.go` 非空 `xrayManager` 回归测试，确认修复前失败。
  - 修复 `internal/adapter/xray/manager.go`：`AddUser` 与 `RemoveUser` 在 `!inbound.IsLocal()` 时直接返回 `nil`。
  - 确认 `internal/adapter/telegram/bot.go:385-395` 在本机 + 远端共存场景下行为正常并补回归测试。
- [ ] **A3. 修复 P1-2 & P1-6（远端 SubRoute 编译一致、缓存失效与 Raw 防串扰）**
  - Fail-Repro：新增测试复现远端 `SubRoute` 缺失、`RollbackSnapshot` 未失效缓存及 Raw 同名远端 `tag` 冲突。
  - 在 `internal/service/` 抽取共享的远端路由 `ensureSubRouteRules` 规范化函数，接入 `node_service.go` (`getOrBuildSnapshot`) 与 `config_service.go` (`GetNodeCompiledConfigRaw`)。
  - 在 `config_service.go` (`saveAndApplyRawConfigLocked`) 成功后调用 `s.invalidateAllNodeCaches()`；在 `syncFromRawJSONLocked` 拦截 `existing != nil && !existing.IsLocal()` 返回明确错误。
- [ ] **A4. 修复 P1-3（远端 SS / SS-2022 增量热加载透传 Method）**
  - Fail-Repro：新增 `compiler_test.go` 与 `agent/xray_grpc_adapter_test.go` 失败用例覆盖 SS-2022 与非默认 SS cipher。
  - `internal/adapter/xray/compiler.go`：`BuildRemoteNodeUsers` 填充 `Method`；提取 `const remoteAPIPort = 8080`。
  - `internal/agent/xray_grpc_adapter.go`：当 `u.Method != ""` 时为合成 `domain.Inbound` 构造 `SettingsJSON`。
- [ ] **A5. 修复 P1-4（远端直连 `ExternalPort==0` 回退至 `inbound.Port`）**
  - Fail-Repro：在 `internal/service/sub_service_test.go` 新增失败用例。
  - 修复 `internal/service/sub_service.go` (`prepareInboundForNode`)。
- [ ] **A6. 修复 P1-8（安装脚本 `EnvironmentFile` 0600 与默认 `60s` 间隔）**
  - 更新 `internal/service/node_service.go` (`RenderInstallScript`)：生成 `0600` `/etc/xray-agent/agent.env` + `EnvironmentFile=`，移除 `ExecStart` 明文 `--token`，使用 `--interval 60s`。
  - 同步更新 `cmd/agent/main.go:47` 与 `internal/agent/runner.go:11` 默认间隔为 `60s`，在 flag 说明注明与远端编译 gRPC 端口 `127.0.0.1:8080` 的对应关系。
  - 更新/补充 `RenderInstallScript` 单元测试。
- [ ] **A7. 后端快速冗余清理 (R7)**
  - 删除 `internal/service/user_service.go:110` (`isXrayUserNotFoundError`)。
  - 删除 `node_service.go` (`nodeInboundFinder` / `findNodeInbounds`) 与 `config_service.go` (`nodeInboundFinder` / `findInboundsByNode`)，统一使用 `inboundRepo.FindByNodeID`。
  - `internal/agent/quota_store.go` 改为直接复用 `domain.CalcTrafficByMode` / `domain.ReachedQuotaThreshold`。
  - `internal/delivery/http/handler_inbound.go` 复用同包 `parseOptionalNodeID`。

### Slice B: Frontend 去除拓扑一键发布、剪贴板去重与 Prettier 修复
**Owned Paths**: `web/**`

- [ ] **B1. 彻底移除拓扑一键发布 (R5)**
  - 删除 `web/src/views/topology/components/QuickPublishWizardModal.vue`。
  - 清理 `web/src/views/TopologyView.vue`：移除按钮、弹窗组件、import、`wizardOpen`、`wizardInitialExitTag`、`openWizard`、`nextRouteId`（若无他用）、副标题中“与 1 步发布”及无用 icon import。
  - 清理 `web/src/views/topology/components/TopologyCanvas.vue`：移除 `openWizard` emit 与按钮/转发。
  - 清理 `web/src/views/topology/components/TopologyPoolsView.vue`：移除 `openWizard` emit 与空态文案中“点击右上角⚡1步发布落地线路”。
  - 清理 `web/src/views/topology/components/OutboundNodeCard.vue`：移除 `@publish` 一键发布按钮与 emit（保留编辑/删除等操作）。
- [ ] **B2. 剪贴板复制去重 (R7)**
  - `web/src/views/PortalClaimView.vue`：删除私有 `copyToClipboard`，改为导入 `web/src/utils/clipboard.ts`。
- [ ] **B3. Prettier 格式门禁修复与构建验证 (R7)**
  - 在 `web/` 执行 `mise x -- npm run format`（或针对性格式化），确保 `mise x -- npm run format:check` 退出 `0`。
  - 执行 `mise x -- npm run typecheck`、`mise x -- npm run lint`、`mise x -- npm run build`。

### Main-Session Hygiene Step
- [ ] **H1. 清理 `.trellis/.template-hashes.json` 本地污染**
  - 在主会话执行 `git checkout -- .trellis/.template-hashes.json`，确保 `git diff` 无 `.pyc` 字节码污染。

## Validation Commands
```bash
# Backend full quality gates
mise x -- go test ./...
CGO_ENABLED=1 mise x -- go test -race ./...
mise x -- go vet ./...
mise x -- go build .
go list -deps ./cmd/agent | Select-String "gin|gorm|sqlite|telegram"

# Frontend full quality gates
cd web && mise x -- npm run typecheck
cd web && mise x -- npm run lint
cd web && mise x -- npm run format:check
cd web && mise x -- npm run build
```

## Risky Files & Rollback Points
- `internal/domain/repository.go`：修改 `InboundRepository` 会影响测试中的 mock 定义，必须在同一提交内编译全量 `./...` 测试。
- `internal/service/config_service.go`：`syncFromRawJSONLocked` 与 `saveAndApplyRawConfigLocked` 涉及全局锁，严禁在持锁期间引发重入死锁（确认 `invalidateAllNodeCaches` 锁序安全）。
- `web/dist/`：`npm run build` 会更新已跟踪的 `web/dist` 产物，检查确认只包含本次 M1 预期变更。
