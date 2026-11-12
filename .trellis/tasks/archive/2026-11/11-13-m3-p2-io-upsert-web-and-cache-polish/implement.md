# M3 执行计划：P2 配置单次读盘、批量 UPSERT、前端轮询瘦身与缓存基线

## 1. Swarm 阵型与无冲突文件边界

- **阵型**：`2 Workers (workspace:"share") + 1 Verifier (workspace:"inherit")`，全员固定 `model: "gemini-4-hydrogen-high"`。
- **前置基线同步**：每个 `share` worker 动手前必须先执行 `git merge --no-edit refs/swarm-opt/m2-core refs/swarm-opt/m2-net-edge` 同步 M1+M2 全量成果，随后提交单笔 M3 增量 commit。
- **`m3-backend` 独占切片（R1, R2, R3, R4）**：
  - `internal/service/config_service.go`、`internal/service/config_service_test.go`
  - `internal/adapter/xray/config_parser.go`、`internal/adapter/xray/config_parser_test.go`
  - `internal/adapter/repository/user_repo.go`、`internal/adapter/repository/user_repo_test.go`
  - `internal/adapter/repository/traffic_log_repo.go`、`internal/adapter/repository/traffic_log_repo_test.go`
  - `internal/adapter/repository/sqlite_db.go`（及对应单测）
  - `internal/delivery/cron/traffic_sync.go`、`internal/delivery/cron/traffic_sync_test.go`
  - `internal/pkg/cache/cache.go`、`internal/pkg/cache/cache_test.go`
  - `internal/adapter/monitor/gopsutil_monitor.go`（及对应单测）
  - `internal/service/geodata_service.go`（及对应单测）
  - `internal/delivery/http/router.go`（及 `router_test.go`）
  - `deploy/nginx-sample.conf`、`deploy/install.sh`、`deploy/99-xray-panel-sysctl.conf`
- **`m3-web` 独占切片（R5）**：
  - `web/package.json`、`web/package-lock.json`、删除 `web/pnpm-lock.yaml`
  - `web/src/main.ts`、`web/vite.config.ts`
  - `web/src/App.vue`
  - `web/src/views/RoutingView.vue`、`web/src/views/DashboardView.vue`、`web/src/views/LogsView.vue`、`web/src/views/UsersView.vue`、`web/src/views/users/composables/useUserList.ts`
  - （严禁触碰 `web/public/favicon.svg` 与 `web/dist/**`！）
- **`m3-verifier` 审查职责**：
  - 审查两笔 M3 增量 commit，确认 R1~R5 全部达标、前后端构建与 `-race` 测试全绿后 `@coordinator`。

## 2. 验证命令

```bash
CGO_ENABLED=1 mise x -- go test -race ./...
mise x -- go vet ./...
mise x -- go build .
cd web && mise x -- npm run typecheck
cd web && mise x -- npm run build
```
