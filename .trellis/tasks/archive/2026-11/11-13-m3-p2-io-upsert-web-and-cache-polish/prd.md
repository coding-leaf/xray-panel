# M3: P2 配置单次读盘、批量 UPSERT、前端轮询瘦身与缓存基线

## Goal

收官全栈性能优化最后一块拼图：消除配置保存时的多重重复读盘与日志轮询重复解析，将每日流量归档升级为单语句批量原子 `ON CONFLICT` UPSERT 并调优 SQLite 页缓存，修复 `pkg/cache` Finalizer 自引用泄漏与监控/下载边界缺陷，补齐 SPA 静态缓存头分离与 Nginx 上游长连接/BBR 基线，并完成前端轮询防重入、定时器清理、日志视图 DOM 减负与移除空置 `pinia`。

## Confirmed Facts

- 前置依赖：M1 与 M2 已完成归拢（固定于引用 `refs/swarm-opt/m2-core` 与 `refs/swarm-opt/m2-net-edge`）。
- `SaveConfigQuietly`（`internal/service/config_service.go`）在单次执行中分别通过 `listOutboundsLocked`、`getRoutingConfigLocked`（内部再次查 `inboundRepo.ListAll`）、`getDNSConfigLocked`、`recordSnapshotBeforeWrite` 重复读取并解析 `config.json` 达 4~5 次；`ConfigManager`（`internal/adapter/xray/config_parser.go:162-178`）每次查询日志路径都重读解析 `config.json`。
- `BatchSyncTraffic`（`internal/adapter/repository/user_repo.go:158-195`）对 `traffic_logs` 逐用户执行 `SELECT + INSERT/UPDATE`（`2N` 次 SQL），而 `domain.TrafficLog` 已具备复合唯一索引 `idx_user_email_date`；`sqlite_db.go:27` 尚未配置 `temp_store(MEMORY)` 与 `cache_size(-8000)`。
- `pkg/cache`（`internal/pkg/cache/cache.go:38-43`）后台 `startJanitor` 持有 `*Cache[T]` 自身指针导致 `runtime.SetFinalizer` 永不触发；`gopsutil_monitor.go` 网卡计数器重置回绕时可能发生无符号下溢；`geodata_service.go` 文件关闭未校验 `Close()` 错误即执行 `Rename`。
- `router.go:173-199` 未分离带哈希 `/assets/*` 与 `index.html` 的 `Cache-Control`；`deploy/nginx-sample.conf` 缺少 `upstream keepalive`、`gzip` 及按需 `Connection` 升级映射。
- 前端 `web/package.json` 依赖 `pinia` 但全仓 0 Store，且残留 `web/pnpm-lock.yaml`；`RoutingView.vue` 未在 `onUnmounted` 清理 `progressTimer`；多视图 `setInterval` 缺少 `inFlight` 防重入锁；`LogsView.vue` 同时挂载桌面与移动两套最多 1000 行的 DOM 列表。

## Requirements

- **R1 (`SaveConfigQuietly` 单次读盘与日志路径缓存)**：重构 `internal/service/config_service.go` 的配置编译前读取逻辑，单次调用 `ReadRawConfig` 同时解析出 `Outbounds`/`Routing`/`DNS`、复用已加载的 `inbounds` 切片并将原始字节直接传给快照记录；在 `internal/adapter/xray/config_parser.go` 缓存访问/错误日志路径，在 `WriteConfig`/`UpdateConfig` 时自动刷新失效。
- **R2 (`TrafficLog` 批量原子 UPSERT、SQLite Pragma 与 Cron 日志完善)**：在 `internal/adapter/repository/user_repo.go`（及 `traffic_log_repo.go`）利用 `clause.OnConflict`（基于 `(user_email, date)`）将每日流量日志落库改为按 200 条分批的原子累加 UPSERT；在 `sqlite_db.go:27` 追加 `_pragma=temp_store(MEMORY)&_pragma=cache_size(-8000)`（严禁改动既有 `WAL`、`busy_timeout(5000)`、`synchronous(NORMAL)`、`SetMaxOpenConns(1)`）；清理 `traffic_sync.go` 残留的 `_ =` 吞错并记录结构化日志。
- **R3 (`pkg/cache` Finalizer 包装与监控/GeoData 健壮性)**：将 `internal/pkg/cache/cache.go` 改为外层包装结构挂载 `SetFinalizer`、内层数据核运行 janitor，确保无显式 `Stop()` 时 GC 可回收后台 goroutine；在 `gopsutil_monitor.go` 处理网卡字节计数回绕（`cur < prev` 时速率归零）；在 `geodata_service.go` 配置合理的 `http.Transport` 连接/响应头超时并在 `os.Rename` 前显式检查 `Close()` 错误。
- **R4 (SPA 缓存头分离、Nginx `keepalive`/`gzip` 与内核 BBR/UDP 模板)**：在 `internal/delivery/http/router.go` 对 `/assets/*` 设置 `Cache-Control: public, max-age=31536000, immutable`、对 `index.html`/SPA 回退入口设置 `Cache-Control: no-cache`；优化 `deploy/nginx-sample.conf`（`upstream panel_backend` `keepalive 32`、`gzip on`、`map $http_upgrade $connection_upgrade`、`/sub/` `proxy_http_version 1.1`），并在 `deploy/` 补充可选 Linux BBR + UDP 缓冲区 `sysctl` 调优配置说明。
- **R5 (前端轮询防重入、定时器清理、`LogsView` DOM 减负与移除 `pinia`)**：
  - 卸载未使用的 `pinia`（清理 `web/package.json`、`web/package-lock.json`、`web/src/main.ts`、`web/vite.config.ts`）并删除冗余 `web/pnpm-lock.yaml`；
  - 在 `RoutingView.vue` 的 `onUnmounted` 清理 `progressTimer`；在 `DashboardView.vue`（含 `await fetchRealityAlerts()`）、`LogsView.vue`、`useUserList.ts`/`UsersView.vue`、`App.vue` 的定时轮询回调增加 `inFlight` 防重入守卫；
  - 在 `LogsView.vue` 通过响应式视口断点条件渲染（`v-if` / `v-else`）替代 CSS `hidden` 双表同挂，并使用稳定行 `:key`（严格保留 `res.lines` 终端模式与 `/logs?type=access&inbound={tag}` 导航契约）。

## Acceptance Criteria

- [ ] [R1] 单次 `SaveConfigQuietly` 仅触发 1 次 `ReadRawConfig` 与 1 次 `inboundRepo.ListAll`；`ConfigManager` 日志路径读取命中内存缓存且写盘后即时更新。
- [ ] [R2] `BatchSyncTraffic` 同日多次累加与跨用户批量写入结果精确，SQLite Pragma 生效且保留全部既有 WAL/单连接不变量。
- [ ] [R3] 单测验证丢弃外层 `*Cache` 引用并触发 `runtime.GC()` 后内层 janitor goroutine 正常退出；网卡计数回绕不产生超大异常速率。
- [ ] [R4] 单测验证 `/assets/*` 返回 `immutable` 缓存头、`/` 与前端路由回退返回 `no-cache`；`deploy/nginx-sample.conf` 上游长连接与压缩配置完整且无真实敏感地址。
- [ ] [R5] `web/package.json` 不再含 `pinia`，`web/pnpm-lock.yaml` 已移除；`cd web && mise x -- npm run typecheck` 与 `cd web && mise x -- npm run build` 零报错通过（不提交构建产物 `web/dist/**`）。
- [ ] 全局门禁 `CGO_ENABLED=1 mise x -- go test -race ./...`、`mise x -- go vet ./...`、`mise x -- go build .` 全绿。

## Out of Scope

- 不删减 `/api/logs` 的 `lines` 字段，不改动 `LogsView.vue` 跨模块 URL query 溯源逻辑。
- 不修改主工作区预先存在的未跟踪/脏文件（如 `web/public/favicon.svg`、`web/dist/favicon.svg`）。
