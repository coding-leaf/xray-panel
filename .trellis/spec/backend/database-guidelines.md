# Database Guidelines

> 纯 Go SQLite 持久化、并发控制与 GORM 规范

---

## 核心技术选型

- **驱动**：`github.com/glebarez/sqlite`（纯 Go 实现，无 CGO 依赖，保证跨平台一键打包）；
  初始化位于 `internal/adapter/repository/sqlite_db.go:28`。
- **连接字符串**（`sqlite_db.go:27`）：必须携带三个 pragma：
  ```
  {dbPath}?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)
  ```
- **连接池**（`sqlite_db.go:37-38`）：
  - `sqlDB.SetMaxOpenConns(1)` 串行排队写入，杜绝 SQLite `database is locked` 异常；
  - `sqlDB.SetMaxIdleConns(1)` 保持单连接空闲复用；
  - 配合 WAL 模式，读操作在其它线程/进程中完全并发无锁。
- **GORM 日志**：`gormlogger.Default.LogMode(gormlogger.Silent)`（`sqlite_db.go:29`），
  ORM 层不打印 SQL，面板自身日志走 `log/slog`（见 logging-guidelines.md）。
- **自动迁移**：`db.AutoMigrate(...)`（`sqlite_db.go:42-51`）集中列出全部领域模型，
  新增持久化模型必须在此登记。
- **初始管理员**：库为空时自动创建 `admin / admin123`（`sqlite_db.go:56-68`）。

---

## GORM 研发规范

1. **流量字段保护更新**：
   更新用户/入站普通资料时，必须显式排除上传/下载计数器，防止并发覆盖：
   ```go
   r.db.WithContext(ctx).Model(user).Omit("up_bytes", "down_bytes").Save(user).Error
   ```
   参考 `internal/adapter/repository/user_repo.go:30`、`internal/adapter/repository/inbound_repo.go:25`。

2. **原子条件抢占 (CAS)**：
   提件码（Ticket）等高并发消费场景必须使用带条件的原子扣减：
   ```go
   r.db.WithContext(ctx).Model(&domain.Ticket{}).
       Where("code = ? AND remaining_uses > 0 AND expires_at > ?", code, now).
       Update("remaining_uses", gorm.Expr("remaining_uses - 1"))
   ```
   参考 `internal/adapter/repository/ticket_repo.go:38-39`。

3. **批量数据同步**：
   定时任务同步多用户流量时，必须走仓储的 `BatchSyncTraffic`
   （`internal/adapter/repository/user_repo.go:118-119`），
   在**单一原子事务**中批量累加用户与入站增量并写每日流量日志；
   禁止单次循环单条开启独立事务。接口定义见 `internal/domain/repository.go:55`。
