# M1 Implementation Plan: Node Model, Master Sync Control Plane & Thin Agent

## Execution Strategy & Parallel Slices

本里程碑拆分为 **2 个 100% 文件互斥的并行 Slice**（适配 Swarm `workspace:"share"`），完成后合并并由 `verifier` 执行端到端集成测试与全量门禁校验。

---

### Slice 1: Master Control Plane (`implementer-master`)

**Owned Files:**
- `internal/domain/node.go` (new)
- `internal/domain/inbound.go` (add `NodeID uint`)
- `internal/domain/repository.go` (add `NodeRepository` / `NodeTrafficBatchRepository` interfaces)
- `internal/adapter/repository/sqlite_db.go` (AutoMigrate `Node`, `NodeTrafficBatch`)
- `internal/adapter/repository/node_repo.go` (new)
- `internal/adapter/repository/node_repo_test.go` (new)
- `internal/adapter/repository/inbound_repo.go` (add `FindByNodeID`)
- `internal/adapter/xray/compiler.go` (add `CompileRemoteNode(inbounds, users)` without altering existing local `Compile` behavior)
- `internal/service/node_service.go` (new: CRUD, token SHA-256 + rotate, `sync.RWMutex` hash cache, `HandleSync` with idempotent `batch_id`, install script generator)
- `internal/service/node_service_test.go` (new)
- `internal/service/config_service.go` & `internal/service/user_service.go` (filter local compile to `NodeID == 0` + hook node cache invalidation cleanly)
- `internal/delivery/http/middleware_agent.go` (new)
- `internal/delivery/http/handler_node.go` (new)
- `internal/delivery/http/handler_agent.go` (new)
- `internal/delivery/http/handler_agent_test.go` (new)
- `internal/delivery/http/router.go` & `main.go` (wire routes & services)

**Ordered Checklist (Slice 1):**
1. 定义 `domain.Node`、`domain.NodeTrafficBatch`、`AgentSyncRequest/Response` DTO，并给 `domain.Inbound` 增加 `NodeID uint`（`default:0`）。
2. 在 `sqlite_db.go` 注册 AutoMigrate，实现 `node_repo.go`（含单个事务内 `ApplyNodeTrafficBatch` 幂等去重 + 累加 `User`/`Inbound`/`TrafficLog`/`Node`）及单元测试。
3. 扩展 `XrayCompiler` 新增 `CompileRemoteNode`（只编译目标节点的启用入站 + 授权活跃用户 + `freedom` 直连出站 + `127.0.0.1:8080` API 入站）；确保现有本机 `ConfigService` 仅编译 `NodeID == 0` 的入站。
4. 实现 `NodeService`：
   - CRUD + 32B 高熵 Token 签发/SHA-256 落库/轮换 + 禁止删除仍有入站绑定的节点；
   - `sync.RWMutex` 保护的节点 `ConfigHash`/`UsersHash` 内存快照缓存 + 失效方法；
   - `HandleAgentSync`（心跳更新 + 幂等流量记账 + hash 短路回包）；
   - `RenderInstallScript`（无硬编码真实域名/IP）。
5. 实现 `middleware_agent.go`、`handler_node.go`、`handler_agent.go` 并在 `router.go`、`main.go` 挂载；编写覆盖 Auth 401/403、hash 短路、重复 `batch_id` 幂等、节点删除冲突校验的测试。

---

### Slice 2: Standalone Thin Agent Binary (`implementer-agent`)

**Owned Files:**
- `internal/agent/protocol.go` (new — locked HTTP JSON DTO matching `design.md` §2)
- `internal/agent/spool.go` & `internal/agent/spool_test.go` (new — local `pending_batch.json` atomic persistence)
- `internal/agent/config_manager.go` & `internal/agent/config_manager_test.go` (new — `xray run -test`, `.bak` atomic swap, reload, 3s gRPC probe & auto-rollback, circuit-break silence config)
- `internal/agent/user_sync.go` & `internal/agent/user_sync_test.go` (new — in-memory diff `AddUser`/`RemoveUser` via injected `UserController` interface)
- `internal/agent/metrics.go`, `internal/agent/metrics_linux.go`, `internal/agent/metrics_other.go` (new — CPU/Mem & `/proc/net/dev` delta reader with non-Linux fallback)
- `internal/agent/client.go` & `internal/agent/runner.go` & `internal/agent/runner_test.go` (new — HTTPS short-polling loop, graceful shutdown, end-to-end mocked Master sync loop)
- `cmd/agent/main.go` (new — CLI flags + env vars wiring; MUST NOT import `gin`, `gorm`, `sqlite`, `telegram`, `web`)

**Ordered Checklist (Slice 2):**
1. 按 `design.md` §2 锁定协议编写 `internal/agent/protocol.go` 与标准库 HTTPS 客户端 `client.go`（带超时与 Bearer Token）。
2. 实现 `spool.go` 原子落盘与断点重传暂存器 + 单元测试（验证失败保留、成功清除、并发/损坏容错）。
3. 实现 `config_manager.go` 配置安全重载器（含 `xray run -test` 校验、`.bak` 自动回滚、3s gRPC 探活及 `circuit_broken` 静音配置切换）+ 单元测试。
4. 实现 `user_sync.go` 差量热更新引擎（通过窄接口封装本机 Xray gRPC `AddUser`/`RemoveUser`）+ 单元测试。
5. 实现跨平台主机与网卡采集器（Linux 读 `/proc/net/dev` 计算增量）与 `runner.go` 定时同步闭环（优雅退出、防 goroutine 泄漏）+ 单元测试。
6. 实现 `cmd/agent/main.go` 并验证 `mise x -- go build ./cmd/agent` 及依赖洁净度检查。

---

### Post-Merge Integration & Verification (`verifier`)

1. 在主工作区合并 Slice 1 与 Slice 2 后，新增/确认端到端契约集成测试（`httptest.Server` 挂载 Master `handler_agent` + `internal/agent.Client` 真实往返 `POST /api/agent/sync`：首次拉全量配置 → 二次 hash 短路 → 上报流量 + 重复 `batch_id` 幂等 → 下发 `circuit_broken`）。
2. 核查依赖树：`mise x -- go list -deps ./cmd/agent` 确认无 `gin`、`gorm`、`glebarez/sqlite`、`telegram`、`web`。
3. 执行完整质量门禁（含竞态检查）。

## Validation Commands

```bash
# 全套单元测试与竞态检查
mise x -- go test ./...
CGO_ENABLED=1 mise x -- go test -race ./internal/service/... ./internal/agent/... ./internal/delivery/http/... ./internal/adapter/repository/...

# 静态检查与双二进制构建
mise x -- go vet ./...
mise x -- go build .
mise x -- go build ./cmd/agent

# 校验 cmd/agent 极瘦依赖边界
mise x -- go list -deps ./cmd/agent
```

## Risky Files & Rollback Points

- `internal/adapter/repository/sqlite_db.go`：SQLite AutoMigrate 仅做增量列/表迁移（`NodeID` default `0`），严禁修改既有列类型。
- `internal/service/config_service.go` & `internal/adapter/xray/compiler.go`：修改本机编译过滤条件 `NodeID == 0` 时必须确保所有既有 `compiler_test.go` / `config_service_test.go` 零回归。
- `main.go` & `router.go`：新增 `/api/agent/*` 路由绝对不能绕过或污染 `/api/admin/*` 的 `JWTAuth` 保护边界。
