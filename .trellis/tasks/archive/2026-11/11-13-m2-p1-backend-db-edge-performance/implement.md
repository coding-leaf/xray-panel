# M2 执行计划：P1 批量操作、SQLite 裁剪、订阅与服务性能提速

## 1. Swarm 阵型与无冲突文件边界

- **阵型**：`2 Workers (workspace:"share") + 1 Verifier (workspace:"inherit")`，全员固定 `model: "gemini-4-hydrogen-high"`。
- **前置基线同步**：每个 `share` worker 在动手修改前必须先执行 `git merge --no-edit refs/swarm-opt/m1-xray refs/swarm-opt/m1-net` 同步 M1 成果，随后在此基线上提交单笔 M2 commit。
- **`m2-core` 独占切片（R1, R2, R3, R4）**：
  - `internal/domain/repository.go`（及必要的审计常量文件如只读引用）
  - `internal/adapter/repository/config_snapshot_repo.go`（及 `*_test.go`）
  - `internal/adapter/repository/traffic_log_repo.go`（及 `*_test.go`）
  - `internal/delivery/cron/traffic_sync.go`（及 `*_test.go`）
  - `internal/service/user_service.go`、`internal/service/user_service_test.go`
  - `internal/service/sub_service.go`、`internal/service/sub_service_test.go`
  - `internal/delivery/http/router.go`、`internal/delivery/http/handler_user.go`（及对应 `*_test.go`）
  - `main.go`
  - `web/src/api/**`、`web/src/views/users/**`
- **`m2-net-edge` 独占切片（R5, R6, R7, R8）**：
  - `internal/service/config_service.go`、`internal/service/config_service_test.go`（仅修改 `ListInbounds` 及相关探测辅助函数与单测）
  - `internal/delivery/http/server.go`（及 `server_test.go`）
  - `internal/adapter/telegram/**`
  - `deploy/cloudflare-pages/_worker.js`、`deploy/cloudflare-worker-sub-proxy.js`、`deploy/gateway.test.js`
- **`m2-verifier` 审查职责**：
  - 等待双分支均完成后，用 `git show <commit>` 审查各自 M2 增量提交，核验零跨切片冲突、零规范违背、全量测试通过后 `@coordinator`。

## 2. 验证命令

```bash
CGO_ENABLED=1 mise x -- go test -race ./...
mise x -- go vet ./...
mise x -- node deploy/gateway.test.js
cd web && mise x -- npm run typecheck
```
