# M3 技术设计：P2 配置单次读盘、批量 UPSERT、前端轮询瘦身与缓存基线

## 1. 架构边界与跨层契约

本里程碑严格保持全部公开 HTTP API JSON 契约、SQLite 表结构及前端路由/设计令牌不变：

- **后端 (`internal/**`, `deploy/**`)** 与 **前端 (`web/**`)** 文件边界 100% 物理正交，由两名 `Gemini 4 Hydrogen` worker 独立并行推进。

---

## 2. 模块设计细节

### 2.1 `SaveConfigQuietly` 单次读盘与日志路径缓存 (`config_service.go`, `config_parser.go`)
- 在 `config_service.go` 新增内部解析结构 `parsedRawSections`（一次 `ReadRawConfig` + 一次 `StripJSONC` 反序列化提取 `Outbounds`、`Routing`、`DNS` 以及原始 `rawBytes`）：
  - `saveConfigQuietlyLocked` 先查 1 次 `inbounds` 与 `users`，调用 1 次 `readParsedSections()`（文件不存在时优雅回退默认段），将已加载的 `inbounds` 直接传入路由组装辅助函数（避免 `getRoutingConfigLocked` 二次查库），并将 `rawBytes` 直接传入 `recordSnapshotWithRaw` 写入快照（避免快照前再次 `ReadRawConfig`）。
- 在 `ConfigManager`（`config_parser.go`）新增缓存字段 `cachedAccessLog string`, `cachedErrorLog string`, `logPathsCached bool`（统一受既有 `m.mu` 保护）：
  - `GetLogPaths` 先在 `RLock` 下命中缓存直接返回；未命中时解析并升级 `Lock` 写入缓存；
  - `WriteConfig` 与 `UpdateConfig` 在 `Lock` 内重置 `logPathsCached = false`。

### 2.2 `TrafficLog` 批量 `ON CONFLICT` UPSERT 与 SQLite Pragma (`user_repo.go`, `traffic_log_repo.go`, `sqlite_db.go`, `traffic_sync.go`)
- 在 `user_repo.go` 的 `BatchSyncTraffic` 事务内：
  - 将待落库的 `[]domain.TrafficLog` 按 200 条切片，使用：
    ```go
    tx.Clauses(clause.OnConflict{
        Columns: []clause.Column{{Name: "user_email"}, {Name: "date"}},
        DoUpdates: clause.Assignments(map[string]interface{}{
            "up_bytes":   gorm.Expr("traffic_logs.up_bytes + excluded.up_bytes"),
            "down_bytes": gorm.Expr("traffic_logs.down_bytes + excluded.down_bytes"),
            "user_id":    gorm.Expr("excluded.user_id"),
        }),
    }).CreateInBatches(logsToUpsert, 200)
    ```
  - `RecordTraffic` 也可统一采用相同的单条 `clause.OnConflict` 原子语句替代 `First + Create/Updates` 两次往返。
- `sqlite_db.go`：DSN 追加 `&_pragma=temp_store(MEMORY)&_pragma=cache_size(-8000)`，严禁改动既有 `journal_mode(WAL)`、`busy_timeout(5000)`、`synchronous(NORMAL)` 与 `sqlDB.SetMaxOpenConns(1)`。

### 2.3 `pkg/cache` Finalizer 外壳包装与监控/GeoData 加固 (`cache.go`, `gopsutil_monitor.go`, `geodata_service.go`)
- `cache.go`：将实际 map、互斥锁与 `stopCh` 放入内部非导出结构 `cacheInner[T]`，`Cache[T]` 仅持有 `inner *cacheInner[T]`；`go inner.startJanitor(cleanupInterval)` 仅捕获 `inner`，`runtime.SetFinalizer(c, func( wrapper *Cache[T] ) { wrapper.Stop() })` 挂在外层 `c` 上（同时用 `sync.Once` 保证 `Stop()` 幂等不 panic）。
- `gopsutil_monitor.go`：计算网卡差值前判断 `totalRecv >= m.lastNetRecv` 与 `totalSent >= m.lastNetSent`；若发生网卡重置/容器重建导致当前计数小于上次计数，仅更新基线并返回当次速率 `0`。
- `geodata_service.go`：下载 `http.Client` 配置 `Transport`（`DialContext` 15s、`TLSHandshakeTimeout` 15s、`ResponseHeaderTimeout` 30s），保留总下载超时充足余量；写完临时文件后显式校验 `if err := out.Close(); err != nil` 再执行 `os.Rename`。

### 2.4 SPA 缓存头分离与 Nginx/Sysctl 基线 (`router.go`, `deploy/`)
- `router.go`：静态资源处理时，对 `/assets/` 前缀路径设置 `c.Header("Cache-Control", "public, max-age=31536000, immutable")`，对 `/`、`index.html` 及 SPA 路由回退 HTML 响应设置 `c.Header("Cache-Control", "no-cache")`。
- `deploy/nginx-sample.conf`：顶部 `http` 块声明 `map $http_upgrade $connection_upgrade { default upgrade; '' close; }` 与 `upstream panel_backend { server 127.0.0.1:9000; keepalive 32; }`，开启 `gzip on; gzip_types text/plain text/css application/json application/javascript image/svg+xml;`；新增 `deploy/99-xray-panel-sysctl.conf` 提供可选 BBR + UDP 收发缓冲区基线并在 `deploy/install.sh` 注释或安全提示引导。

### 2.5 前端轮询防重入、`LogsView` 单套挂载与卸载 `pinia` (`web/`)
- 依赖清理：从 `web/package.json` 移除 `"pinia"`，更新 `web/package-lock.json`，删除 `web/pnpm-lock.yaml`，移除 `web/src/main.ts` 的 `createPinia()` 与 `web/vite.config.ts` 中可能存在的 `pinia` 分包配置。
- 定时器与防重入：
  - `RoutingView.vue`：在 `onUnmounted` 中清理 `progressTimer`；
  - `DashboardView.vue`、`LogsView.vue`、`useUserList.ts`、`App.vue`：为各 `setInterval` 异步任务增加局部 `let xxxInFlight = false` `try/finally` 守卫；`DashboardView` 轮询内补齐 `await fetchRealityAlerts()`。
- `LogsView.vue`：使用响应式 `isDesktop`（`window.matchMedia('(min-width: 768px)')` 监听，卸载时移除事件监听）通过 `v-if="isDesktop"` / `v-else` 确保同一时刻仅渲染桌面表格或移动端卡片列表中的一套，并为列表项生成稳定唯一下标/内容复合 `:key`。

---

## 3. 风险与回滚预案

- **禁止提交构建产物**：`m3-web` 在 worktree 中运行 `npm run build` 验证后，若工作区 `web/dist/` 有变更（注意 `web/dist` 部分文件是否被 git 跟踪！需先执行 `git ls-files web/dist` 检查：仅修改源码与锁文件，不要改动预存的 `web/public/favicon.svg` 或无关构建产物，若 `web/dist` 未跟踪则保持干净）。
