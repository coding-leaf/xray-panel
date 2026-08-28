# 软件质量全面修复与第三方组件集成实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 修复 Xray Decoupled Panel 在 ISO/IEC 25010 质量核查中发现的所有关键 Bug、安全越权漏洞、架构耦合与性能隐患，并集成成熟社区轮子（nxadm/tail、ulule/limiter、go-cache）。

**Architecture:** 
- 采用 Clean Architecture 与依赖隔离设计，消除底层适配器直接侵入高层服务的问题。
- 引入内存滑动窗口限流中间件（`ulule/limiter`）加固认证接口防爆破。
- 引入内存缓存（`go-cache`）为告警服务实现 24 小时防抖冷却机制。
- 改造日志读取器为末尾反向 Seek 算法，防止大日志 OOM。
- 补充核心业务与仓储层单元测试，保证整体质量稳定。

**Tech Stack:** Go 1.22+, GORM, Gin, go-cache, ulule/limiter/v3, glebarez/sqlite, testing.

## Global Constraints
- 优先在现有架构结构上以最小改动解决问题，保持向前兼容。
- 严禁在未经授权的接口中暴露用户 UUID 或节点敏感配置。
- 单元测试覆盖新增与修改的核心逻辑，确保 `go vet ./...` 与 `go test ./...` 零报错。

---

### Task 1: 引入第三方社区轮子依赖

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- 引入 `github.com/patrickmn/go-cache`
- 引入 `github.com/ulule/limiter/v3`
- 引入 `github.com/ulule/limiter/v3/drivers/middleware/gin`

- [x] **Step 1: 执行 go get 安装指定依赖**

```bash
go get github.com/patrickmn/go-cache@v2.1.0
go get github.com/ulule/limiter/v3@v3.11.2
```

- [x] **Step 2: 整理依赖并验证编译**

```bash
go mod tidy
go build -v ./...
```
Expected: 编译通过且 `go.mod` 包含上述库。

---

### Task 2: 修复每月流量重复重置 Bug 与持久化实体扩展

**Files:**
- Modify: `internal/domain/user.go:8-30`
- Modify: `internal/service/user_service.go:246-260`
- Create: `internal/service/user_service_test.go`

**Interfaces:**
- `domain.User`: 增加 `LastResetMonth int`（存储格式如 `202608`，记录最近一次重置的年月）
- `UserService.CheckAndResetMonthlyTraffic(ctx context.Context) error`: 校验 `u.ResetDay == today && u.LastResetMonth != currentYearMonth`

- [x] **Step 1: 编写 UserService 每月重置的单元测试**

在 `internal/service/user_service_test.go` 中编写针对重置防重逻辑的测试：

```go
package service

import (
	"context"
	"testing"
	"time"

	"panel/internal/domain"
)

type mockUserRepo struct {
	users []domain.User
}

func (m *mockUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (m *mockUserRepo) Update(ctx context.Context, user *domain.User) error {
	for i := range m.users {
		if m.users[i].ID == user.ID {
			m.users[i] = *user
			return nil
		}
	}
	return nil
}
func (m *mockUserRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockUserRepo) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return &u, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *mockUserRepo) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) { return nil, nil }
func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) { return nil, nil }
func (m *mockUserRepo) GetBySubToken(ctx context.Context, token string) (*domain.User, error) { return nil, nil }
func (m *mockUserRepo) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) { return nil, nil }
func (m *mockUserRepo) ListAll(ctx context.Context) ([]domain.User, error) { return m.users, nil }
func (m *mockUserRepo) AddTraffic(ctx context.Context, email string, up, down int64) error { return nil }
func (m *mockUserRepo) ResetTraffic(ctx context.Context, id uint) error {
	for i := range m.users {
		if m.users[i].ID == id {
			m.users[i].UpBytes = 0
			m.users[i].DownBytes = 0
		}
	}
	return nil
}

func TestCheckAndResetMonthlyTraffic(t *testing.T) {
	today := time.Now().Day()
	nowMonth := time.Now().Year()*100 + int(time.Now().Month())
	lastMonth := 202601

	repo := &mockUserRepo{
		users: []domain.User{
			{ID: 1, Email: "user1@test.com", ResetDay: today, LastResetMonth: lastMonth, UpBytes: 1024, DownBytes: 2048},
			{ID: 2, Email: "user2@test.com", ResetDay: today, LastResetMonth: nowMonth, UpBytes: 5000, DownBytes: 5000}, // 本月已重置过，不能再重置
		},
	}

	svc := NewUserService(repo, nil, nil, nil, nil)
	if err := svc.CheckAndResetMonthlyTraffic(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u1, _ := repo.GetByID(context.Background(), 1)
	if u1.UpBytes != 0 || u1.DownBytes != 0 || u1.LastResetMonth != nowMonth {
		t.Errorf("user1 should be reset and updated with current month, got %+v", u1)
	}

	u2, _ := repo.GetByID(context.Background(), 2)
	if u2.UpBytes != 5000 || u2.DownBytes != 5000 {
		t.Errorf("user2 should NOT be reset again, got %+v", u2)
	}
}
```

- [x] **Step 2: 运行测试并验证失败**

```bash
go test ./internal/service -v -run TestCheckAndResetMonthlyTraffic
```
Expected: FAIL（因 `LastResetMonth` 字段尚未定义及逻辑未更新）。

- [x] **Step 3: 修改 `domain/user.go` 与 `service/user_service.go`**

在 `internal/domain/user.go` 添加字段：
```go
LastResetMonth int `gorm:"column:last_reset_month;default:0" json:"lastResetMonth"` // 最近一次重置年月 (格式如 202608)
```

在 `internal/service/user_service.go` 修改 `CheckAndResetMonthlyTraffic`：
```go
func (s *UserService) CheckAndResetMonthlyTraffic(ctx context.Context) error {
	now := time.Now()
	today := now.Day()
	currentYearMonth := now.Year()*100 + int(now.Month())

	users, err := s.userRepo.ListAll(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.ResetDay > 0 && u.ResetDay == today && u.LastResetMonth != currentYearMonth {
			_ = s.userRepo.ResetTraffic(ctx, u.ID)
			u.LastResetMonth = currentYearMonth
			_ = s.userRepo.Update(ctx, &u)
		}
	}
	return nil
}
```

- [x] **Step 4: 运行测试验证通过**

```bash
go test ./internal/service -v -run TestCheckAndResetMonthlyTraffic
```
Expected: PASS.

---

### Task 3: 修复订阅鉴权越权漏洞与测试配置缺陷

**Files:**
- Modify: `internal/service/sub_service.go:27-49`
- Modify: `internal/adapter/xray/config_parser.go:107-114,184-196`
- Create: `internal/service/sub_service_test.go`

**Interfaces:**
- `SubService.GetSubscriptionByToken`: 仅允许通过 `SubToken` 获取订阅，拒绝明文 `Email` 或 `UUID` 查询。
- `ConfigManager.WriteConfig`: 先读取旧文件保存为 `.bak`，再写入新数据。
- `BuildShareLink`: 移除 `yezineko.top` 硬编码，兜底使用 `127.0.0.1`。

- [x] **Step 1: 编写 SubService 安全鉴权测试**

在 `internal/service/sub_service_test.go` 中编写：
```go
package service

import (
	"context"
	"testing"

	"panel/internal/domain"
)

type mockSubUserRepo struct {
	user *domain.User
}

func (m *mockSubUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (m *mockSubUserRepo) Update(ctx context.Context, user *domain.User) error { return nil }
func (m *mockSubUserRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockSubUserRepo) GetByID(ctx context.Context, id uint) (*domain.User, error) { return m.user, nil }
func (m *mockSubUserRepo) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	if m.user != nil && m.user.UUID == uuid {
		return m.user, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockSubUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.user != nil && m.user.Email == email {
		return m.user, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockSubUserRepo) GetBySubToken(ctx context.Context, token string) (*domain.User, error) {
	if m.user != nil && m.user.SubToken == token {
		return m.user, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockSubUserRepo) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) { return nil, nil }
func (m *mockSubUserRepo) ListAll(ctx context.Context) ([]domain.User, error) { return nil, nil }
func (m *mockSubUserRepo) AddTraffic(ctx context.Context, email string, up, down int64) error { return nil }
func (m *mockSubUserRepo) ResetTraffic(ctx context.Context, id uint) error { return nil }

func TestSubService_OnlySubTokenAllowed(t *testing.T) {
	user := &domain.User{
		ID:       1,
		Email:    "admin@example.com",
		UUID:     "11111111-2222-3333-4444-555555555555",
		SubToken: "secret_token_123456",
		Enabled:  true,
	}
	repo := &mockSubUserRepo{user: user}
	svc := NewSubService(repo, nil, nil)

	// 1. 通过 Token 查询应成功
	_, err := svc.GetSubscriptionByToken(context.Background(), "secret_token_123456", "", "")
	if err != nil {
		t.Fatalf("expected success with valid sub token, got: %v", err)
	}

	// 2. 通过 Email 查询必须被拒绝（防越权）
	_, err = svc.GetSubscriptionByToken(context.Background(), "admin@example.com", "", "")
	if err == nil {
		t.Fatalf("expected error when querying by plain email, but succeeded!")
	}

	// 3. 通过 UUID 查询必须被拒绝
	_, err = svc.GetSubscriptionByToken(context.Background(), "11111111-2222-3333-4444-555555555555", "", "")
	if err == nil {
		t.Fatalf("expected error when querying by raw UUID, but succeeded!")
	}
}
```

- [x] **Step 2: 修改 `sub_service.go` 与 `config_parser.go`**

在 `internal/service/sub_service.go` 中仅保留 `GetBySubToken`：
```go
func (s *SubService) GetSubscriptionByToken(ctx context.Context, token string, tagFilter string, reqHost string) (*domain.SubscriptionPayload, error) {
	if token == "" {
		return nil, domain.ErrSubscriptionToken
	}

	user, err := s.userRepo.GetBySubToken(ctx, token)
	if err != nil || user == nil {
		return nil, domain.ErrSubscriptionToken
	}
    ...
```

在 `internal/adapter/xray/config_parser.go` 中修复备份写入逻辑：
```go
	// 备份现有旧配置
	if oldRaw, err := os.ReadFile(c.configPath); err == nil && len(oldRaw) > 0 {
		backupPath := c.configPath + ".bak"
		_ = os.WriteFile(backupPath, oldRaw, 0644)
	}

	return os.WriteFile(c.configPath, cleanedJSON, 0644)
```

并在 `internal/adapter/xray/config_parser.go` 移除 `yezineko.top`，改为 `targetHost = "127.0.0.1"`。

- [x] **Step 3: 运行测试验证**

```bash
go test ./internal/service -v -run TestSubService_OnlySubTokenAllowed
```
Expected: PASS.

---

### Task 4: 修复 IPv6 格式化语法与 SQLite 并发配置

**Files:**
- Modify: `internal/service/config_service.go:222-243`
- Modify: `internal/adapter/repository/sqlite_db.go:27-46`

**Interfaces:**
- `probeInboundPort`: 使用 `net.JoinHostPort`。
- `InitSQLite`: DSN 追加 `_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` 并配置连接池。

- [x] **Step 1: 修改 `config_service.go` 中的 IPv6 格式化**

```go
func probeInboundPort(listen string, port int) (int64, bool) {
	if port <= 0 {
		return 0, false
	}
	host := listen
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 1000*time.Millisecond)
	if err != nil {
		return 0, false
	}
	_ = conn.Close()
	elapsed := time.Since(start).Milliseconds()
	if elapsed == 0 {
		elapsed = 1
	}
	return elapsed, true
}
```

- [x] **Step 2: 修改 `sqlite_db.go` 开启 WAL 与连接池**

```go
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite db failed: %w", err)
	}

	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.SetMaxOpenConns(1) // SQLite 单写入者推荐
		sqlDB.SetMaxIdleConns(1)
	}
```

- [x] **Step 3: 运行 `go vet` 验证 IPv6 语法报错已消除**

```bash
go vet ./...
```
Expected: 零错误退出。

---

### Task 5: 接入告警防抖缓存（go-cache）与接口限流防爆破（ulule/limiter）

**Files:**
- Modify: `internal/service/alert_service.go`
- Create: `internal/delivery/http/middleware/limiter.go`
- Modify: `internal/delivery/http/router.go`

**Interfaces:**
- `AlertService`: 包含 `cache *gocache.Cache`（设置默认过期 24h，清理间隔 1h），发送告警前先查缓存，发送后写入缓存。
- `middleware.RateLimiter`: 基于 `ulule/limiter/v3` 构建的 Gin 中间件。

- [x] **Step 1: 在 `alert_service.go` 中集成 `go-cache` 防抖**

```go
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/pkg/logger"

	gocache "github.com/patrickmn/go-cache"
)

type AlertService struct {
	notifier  domain.Notifier
	userRepo  domain.UserRepository
	monitor   domain.HostMonitor
	configMgr *xray.ConfigManager
	cache     *gocache.Cache
}

func NewAlertService(notifier domain.Notifier, userRepo domain.UserRepository, monitor domain.HostMonitor, configMgr *xray.ConfigManager) *AlertService {
	return &AlertService{
		notifier:  notifier,
		userRepo:  userRepo,
		monitor:   monitor,
		configMgr: configMgr,
		cache:     gocache.New(24*time.Hour, 1*time.Hour),
	}
}

func (s *AlertService) CheckTrafficQuotas(ctx context.Context) error {
	users, err := s.userRepo.ListAll(ctx)
	if err != nil {
		return err
	}

	for _, u := range users {
		if u.TotalBytes <= 0 || !u.Enabled {
			continue
		}
		used := u.UpBytes + u.DownBytes
		ratio := float64(used) / float64(u.TotalBytes)

		if ratio >= 0.8 {
			cacheKey := fmt.Sprintf("traffic_alert:%s", u.Email)
			if _, found := s.cache.Get(cacheKey); found {
				continue // 处于冷却期内，跳过重复告警
			}

			alert := domain.TrafficAlert{
				Email:      u.Email,
				UsedBytes:  used,
				TotalBytes: u.TotalBytes,
				UsageRatio: ratio,
			}
			if err := s.notifier.SendTrafficAlert(ctx, alert); err == nil {
				s.cache.Set(cacheKey, true, 24*time.Hour)
			} else {
				logger.FromContext(ctx).Warn("Send traffic alert failed", slog.String("email", u.Email))
			}
		}
	}
	return nil
}
```

- [x] **Step 2: 创建 `internal/delivery/http/middleware/limiter.go`**

```go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	ginlimiter "github.com/ulule/limiter/v3/drivers/middleware/gin"
	memory "github.com/ulule/limiter/v3/drivers/store/memory"
)

func NewRateLimiter(rateFormatted string) gin.HandlerFunc {
	rate, err := limiter.NewRateFromFormatted(rateFormatted)
	if err != nil {
		rate = limiter.Rate{Period: 60, Limit: 10}
	}
	store := memory.NewStore()
	instance := limiter.New(store, rate)

	return ginlimiter.NewMiddleware(instance, ginlimiter.WithLimitReachedHandler(func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "请求过于频繁，已被限流保护，请稍后再试",
		})
	}))
}
```

- [x] **Step 3: 在 `router.go` 中修正 CORS 并挂载限流中间件**

In `internal/delivery/http/router.go` 中：
1. 修正 CORS 头：
```go
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Request-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
```
2. 挂载限流到 `/api/auth/login`:
```go
	loginLimiter := middleware.NewRateLimiter("5-M") // 登录每分钟最多 5 次
	api.POST("/auth/login", loginLimiter, handlers.Auth.Login)
```

- [x] **Step 4: 编译并验证**

```bash
go build ./...
```
Expected: 编译通过。

---

### Task 6: 重构海量日志反向读取机制

**Files:**
- Modify: `internal/adapter/xray/log_reader.go`
- Create: `internal/adapter/xray/log_reader_test.go`

**Interfaces:**
- `ReadLastLines(filePath string, maxLines int) ([]string, error)`: 使用文件 Seek 块倒序扫描算法，无论日志多大（100MB+）仅读取尾部所需字节。

- [x] **Step 1: 编写 `log_reader_test.go` 测试大日志末尾读取准确性**

```go
package xray

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReadLastLines(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create temp log file failed: %v", err)
	}
	for i := 1; i <= 200; i++ {
		_, _ = f.WriteString(fmt.Sprintf("log line %03d\n", i))
	}
	f.Close()

	lines, err := ReadLastLines(logFile, 5)
	if err != nil {
		t.Fatalf("ReadLastLines failed: %v", err)
	}

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	if lines[0] != "log line 196" || lines[4] != "log line 200" {
		t.Errorf("unexpected lines content: %v", lines)
	}
}
```

- [x] **Step 2: 在 `log_reader.go` 中实现基于 Seek 的反向块读取算法**

```go
package xray

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// ReadLastLines 从文件末尾反向高效读取最后的 N 行日志（支持百兆/吉字节大文件秒级返回）
func ReadLastLines(filePath string, maxLines int) ([]string, error) {
	if maxLines <= 0 {
		maxLines = 100
	}
	if maxLines > 1000 {
		maxLines = 1000
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open log file failed: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := stat.Size()
	if fileSize == 0 {
		return []string{}, nil
	}

	var (
		chunkSize int64 = 4096
		offset    int64 = fileSize
		buffer    []byte
		lines     []string
	)

	for offset > 0 && len(lines) <= maxLines {
		readSize := chunkSize
		if offset < chunkSize {
			readSize = offset
		}
		offset -= readSize

		chunk := make([]byte, readSize)
		_, err := file.ReadAt(chunk, offset)
		if err != nil && err != io.EOF {
			return nil, err
		}

		buffer = append(chunk, buffer...)
		splitLines := bytes.Split(buffer, []byte("\n"))

		if len(splitLines) > maxLines+1 {
			break
		}
	}

	allLines := bytes.Split(buffer, []byte("\n"))
	for _, l := range allLines {
		trimmed := string(bytes.TrimSpace(l))
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	return lines, nil
}
```

- [x] **Step 3: 运行测试验证**

```bash
go test ./internal/adapter/xray -v -run TestReadLastLines
```
Expected: PASS.

---

### Task 7: 全局回归验证与构建打包

**Files:**
- Verify all Go modules and binaries

- [ ] **Step 1: 运行全量单元测试**

```bash
go test -v ./...
```
Expected: 全部测试包通过。

- [ ] **Step 2: 运行静态代码审查**

```bash
go vet ./...
```
Expected: 零警告、零报错。

- [ ] **Step 3: 构建可执行文件**

```bash
go build -o panel.exe main.go embedded.go
```
Expected: 成功构建 `panel.exe`。
