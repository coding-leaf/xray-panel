# Xray Panel Bugfix and Architectural Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix critical functional bugs (REALITY dest loss, user traffic overwrite, quota ghost revival, substring matching) and decouple user operations from full Xray process restarts to achieve true zero-downtime hot reloading and lifecycle consistency.

**Architecture:** Split operations into lightweight user runtime events (pure gRPC memory sync + quiet disk persistence without process restart) and structural events (inbound/outbound/routing/DNS changes requiring systemctl restart). Enforce `IsActive()` across the compiler, background cron, and recovery pipelines with DTO-based non-destructive updates.

**Tech Stack:** Go 1.26 (Go toolchain via mise), Gin, GORM + SQLite (WAL), Xray-core v1.260327.0 (gRPC API), Vue 3 + Tailwind CSS.

**Spec:** [docs/superpowers/specs/2026-09-03-xray-panel-review-and-bugfix-design.md](file:///home/yezisama/workspace/WorkSpace/xray-panel/docs/superpowers/specs/2026-09-03-xray-panel-review-and-bugfix-design.md)

## Global Constraints

- Never call `supervisor.Reload` or `supervisor.Restart` during user CRUD, batch renew, or traffic reset operations.
- All tests must pass using `mise exec -- go test -v ./...`.
- Update endpoints must never overwrite `up_bytes` or `down_bytes` in the database.
- REALITY configuration must use standard `dest` attribute while maintaining fallback compatibility for legacy `target`.

---

### Task 1: Protocol & Schema Normalization (Reality `dest`, Dynamic gRPC Port, Shadowsocks Cipher)

**Files:**
- Modify: `internal/adapter/xray/schema.go`
- Modify: `internal/adapter/xray/compiler.go`
- Modify: `internal/adapter/xray/grpc_client.go`
- Test: `internal/adapter/xray/compiler_test.go`

**Interfaces:**
- `NewXrayCompiler(grpcPort int) *XrayCompiler` (or `compiler.SetGRPCPort(port int)`)
- `XrayRealitySettings`: handles both `dest` and fallback `target`

- [ ] **Step 1: Write the failing tests in compiler_test.go**

```go
func TestCompiler_RealityDestAndDynamicGRPCPort(t *testing.T) {
	c := NewXrayCompiler(9090)

	inbounds := []domain.Inbound{
		{
			Tag:      "vless-reality",
			Listen:   "0.0.0.0",
			Port:     443,
			Protocol: "vless",
			StreamSettings: `{
				"network": "tcp",
				"security": "reality",
				"realitySettings": {
					"target": "gateway.icloud.com:443",
					"serverNames": ["gateway.icloud.com"],
					"privateKey": "testkey"
				}
			}`,
			Enabled: true,
		},
	}

	cfg, err := c.Compile(inbounds, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Verify dynamic gRPC port
	var apiInbound *XrayInbound
	for _, in := range cfg.Inbounds {
		if in.Tag == "api" {
			apiInbound = &in
			break
		}
	}
	if apiInbound == nil || apiInbound.Port != 9090 {
		t.Fatalf("expected api inbound port 9090, got %v", apiInbound)
	}

	// 2. Verify reality dest normalization from target
	var realityInbound *XrayInbound
	for _, in := range cfg.Inbounds {
		if in.Tag == "vless-reality" {
			realityInbound = &in
			break
		}
	}
	if realityInbound == nil || realityInbound.StreamSettings == nil || realityInbound.StreamSettings.RealitySettings == nil {
		t.Fatalf("expected reality settings, got nil")
	}
	if realityInbound.StreamSettings.RealitySettings.Dest != "gateway.icloud.com:443" {
		t.Errorf("expected dest gateway.icloud.com:443, got %q", realityInbound.StreamSettings.RealitySettings.Dest)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `mise exec -- go test -v ./internal/adapter/xray -run TestCompiler_RealityDestAndDynamicGRPCPort`
Expected: FAIL with compilation error or assertion failure

- [ ] **Step 3: Implement dynamic gRPC port and Reality dest normalization**

In `internal/adapter/xray/schema.go`:
Add `Target string json:"target,omitempty"` to `XrayRealitySettings`.

In `internal/adapter/xray/compiler.go`:
Add `grpcPort int` field to `XrayCompiler`. In `NewXrayCompiler(grpcPort ...int)`, default to 8080 if not provided or 0.
In `compileInbound`:
If `r.Dest == ""` && `r.Target != ""`, set `r.Dest = r.Target`.
If `r.Dest == ""`, set `r.Dest = "www.titech.ac.jp:443"`.
Clear `r.Target = ""`.

In `internal/adapter/xray/grpc_client.go`:
In `buildAccountMessage`:
Parse `inbound.SettingsJSON` to detect `method` or `cipher`. If `chacha20` or `2022-blake3` is specified, map appropriately or fallback safely.

- [ ] **Step 4: Run test to verify it passes**

Run: `mise exec -- go test -v ./internal/adapter/xray -run TestCompiler_RealityDestAndDynamicGRPCPort`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/xray/schema.go internal/adapter/xray/compiler.go internal/adapter/xray/grpc_client.go internal/adapter/xray/compiler_test.go
git commit -m "fix(xray): support dynamic grpc port and reality dest normalization"
```

---

### Task 2: User Lifecycle & Compiler Filtering (Compiler `IsActive()` Filter, TrafficSync Expiration Eviction)

**Files:**
- Modify: `internal/adapter/xray/compiler.go`
- Modify: `internal/delivery/cron/traffic_sync.go`
- Test: `internal/adapter/xray/compiler_test.go`

**Interfaces:**
- `XrayCompiler.compileInbound`: uses `u.IsActive()` instead of `u.Enabled`
- `traffic_sync.go`: checks `!user.IsActive()` during synchronization to evict expired/over-quota users

- [ ] **Step 1: Write the failing test for inactive user filtering**

```go
func TestCompiler_FilterInactiveUsers(t *testing.T) {
	c := NewXrayCompiler(8080)
	now := time.Now().UnixMilli()

	inbounds := []domain.Inbound{
		{Tag: "in-1", Port: 10001, Protocol: "vless", Enabled: true},
	}
	users := []domain.User{
		{Email: "active@test.com", UUID: "uuid-1", InboundTags: "in-1", Enabled: true},
		{Email: "disabled@test.com", UUID: "uuid-2", InboundTags: "in-1", Enabled: false},
		{Email: "expired@test.com", UUID: "uuid-3", InboundTags: "in-1", Enabled: true, ExpireTime: now - 10000},
		{Email: "overquota@test.com", UUID: "uuid-4", InboundTags: "in-1", Enabled: true, TotalBytes: 100, UpBytes: 50, DownBytes: 60},
	}

	cfg, err := c.Compile(inbounds, nil, nil, nil, users)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	var in1 *XrayInbound
	for _, in := range cfg.Inbounds {
		if in.Tag == "in-1" {
			in1 = &in
			break
		}
	}
	if in1 == nil {
		t.Fatalf("in-1 not found")
	}

	var settings struct {
		Clients []XrayClient `json:"clients"`
	}
	_ = json.Unmarshal(in1.Settings, &settings)

	if len(settings.Clients) != 1 {
		t.Fatalf("expected exactly 1 active client, got %d", len(settings.Clients))
	}
	if settings.Clients[0].Email != "active@test.com" {
		t.Errorf("expected client active@test.com, got %s", settings.Clients[0].Email)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `mise exec -- go test -v ./internal/adapter/xray -run TestCompiler_FilterInactiveUsers`
Expected: FAIL (because expired and over-quota users are currently included)

- [ ] **Step 3: Implement IsActive filtering and traffic_sync eviction**

In `internal/adapter/xray/compiler.go:232`:
Replace:
```go
		if !u.Enabled {
			continue
		}
```
With:
```go
		if !u.IsActive() {
			continue
		}
```

In `internal/delivery/cron/traffic_sync.go:120`:
Replace:
```go
				if user.IsTrafficExceeded() {
					for _, t := range user.GetInboundTagList() {
						_ = j.xrayManager.RemoveUser(ctx, t, user.Email)
					}
				}
```
With:
```go
				if !user.IsActive() {
					for _, t := range user.GetInboundTagList() {
						_ = j.xrayManager.RemoveUser(ctx, t, user.Email)
					}
				}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `mise exec -- go test -v ./internal/adapter/xray -run TestCompiler_FilterInactiveUsers`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/xray/compiler.go internal/delivery/cron/traffic_sync.go internal/adapter/xray/compiler_test.go
git commit -m "fix(lifecycle): filter inactive and expired users in compiler and traffic sync"
```

---

### Task 3: Non-Destructive User Persistence & UpdateUserDTO (DTO Isolation, Preserving Traffic Counters, Exact InboundTag Matching)

**Files:**
- Modify: `internal/domain/user.go`
- Modify: `internal/domain/repository.go`
- Modify: `internal/adapter/repository/user_repo.go`
- Modify: `internal/service/user_service.go`
- Modify: `internal/delivery/http/handler_user.go`
- Test: `internal/service/user_service_test.go`
- Test: `internal/adapter/repository/user_repo_test.go`

**Interfaces:**
- `service.UpdateUserDTO`: DTO containing only editable fields
- `UserRepository.UpdateNonTraffic(ctx, id uint, dto UpdateUserDTO) error`
- `UserRepository.ListByInboundTag`: strict tag inclusion, no substring false matches

- [ ] **Step 1: Write failing tests for UpdateUser non-destructive behavior and exact tag matching**

In `internal/adapter/repository/user_repo_test.go`:
```go
func TestUserRepo_ListByInboundTag_ExactMatch(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u1 := &domain.User{Email: "u1@test.com", UUID: "uuid-1", InboundTag: "vless-in", InboundTags: "vless-in,trojan-in"}
	u2 := &domain.User{Email: "u2@test.com", UUID: "uuid-2", InboundTag: "vless-in-2", InboundTags: "vless-in-2"}
	_ = repo.Create(ctx, u1)
	_ = repo.Create(ctx, u2)

	list, err := repo.ListByInboundTag(ctx, "vless-in")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(list) != 1 || list[0].Email != "u1@test.com" {
		t.Fatalf("expected only u1, got: %v", list)
	}
}
```

In `internal/service/user_service_test.go`:
Test that updating user metadata preserves `UpBytes` and `DownBytes`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `mise exec -- go test -v ./internal/adapter/repository -run TestUserRepo_ListByInboundTag_ExactMatch`
Expected: FAIL

- [ ] **Step 3: Implement UpdateUserDTO and exact matching**

In `internal/domain/user.go`:
Add `UpdateUserDTO` struct:
```go
type UpdateUserDTO struct {
	InboundTags []string `json:"inboundTags"`
	InboundTag  string   `json:"inboundTag"`
	Flow        string   `json:"flow"`
	TotalBytes  int64    `json:"totalBytes"`
	ExpireTime  int64    `json:"expireTime"`
	ResetDay    int      `json:"resetDay"`
	IPLimit     int      `json:"ipLimit"`
	Enabled     *bool    `json:"enabled"`
}
```

In `internal/adapter/repository/user_repo.go`:
Fix `ListByInboundTag`:
Query all or query with strict boundary check:
`WHERE inbound_tag = ? OR (',' || REPLACE(inbound_tags, ' ', '') || ',') LIKE ?`, with `"%," + tag + ",%"`.
And filter using `u.HasInbound(tag)`.

In `internal/service/user_service.go`:
Update `UpdateUser` to accept `UpdateUserDTO` (or safely merge `oldUser` preserving `UpBytes`, `DownBytes`, `UUID`, `SubToken`, `CreatedAt`).

In `internal/delivery/http/handler_user.go`:
Bind `UpdateUserDTO` in `Update(c *gin.Context)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `mise exec -- go test -v ./internal/adapter/repository ./internal/service`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/user.go internal/adapter/repository/user_repo.go internal/service/user_service.go internal/delivery/http/handler_user.go internal/adapter/repository/user_repo_test.go internal/service/user_service_test.go
git commit -m "fix(user): introduce UpdateUserDTO, protect traffic counters, and fix exact tag matching"
```

---

### Task 4: Zero-Downtime Decoupling & Auto-Restore on Traffic Reset/Renewal

**Files:**
- Modify: `internal/service/config_service.go`
- Modify: `internal/service/user_service.go`
- Test: `internal/service/user_service_test.go`

**Interfaces:**
- `ConfigService.SaveConfigQuietly(ctx context.Context, remark string) error`: recompiles and writes to disk WITHOUT calling `supervisor.Reload`
- `UserService.ResetTraffic`, `BatchResetTraffic`, `CheckAndResetMonthlyTraffic`: resets DB traffic and calls `xrayManager.AddUser` for active users

- [ ] **Step 1: Write failing test in user_service_test.go**

Test that:
1. `CreateUser`, `UpdateUser`, `DeleteUser` invoke `SaveConfigQuietly` instead of reloading Xray.
2. `ResetTraffic` checks if user becomes active and calls `xrayManager.AddUser` on authorized tags.

- [ ] **Step 2: Run test to verify it fails**

Run: `mise exec -- go test -v ./internal/service -run TestUserService_ZeroDowntimeAndAutoRestore`
Expected: FAIL

- [ ] **Step 3: Implement SaveConfigQuietly and Auto-Restore**

In `internal/service/config_service.go`:
Add `SaveConfigQuietly(ctx context.Context, remark string) error`:
Compiles to JSON and writes to disk, recording snapshot, without calling `s.supervisor.Reload(ctx)`.
In `SyncUserToFile(ctx, tags, user, isDelete)`:
Use `SaveConfigQuietly(ctx, fmt.Sprintf("静默同步用户 %s 授权", user.Email))`.

In `internal/service/user_service.go`:
In `ResetTraffic(ctx, id)`:
```go
func (s *UserService) ResetTraffic(ctx context.Context, id uint) error {
	if err := s.userRepo.ResetTraffic(ctx, id); err != nil {
		return err
	}
	user, err := s.userRepo.GetByID(ctx, id)
	if err == nil && user != nil && user.IsActive() {
		for _, t := range user.GetInboundTagList() {
			_ = s.xrayManager.AddUser(ctx, t, user)
		}
		if s.configSvc != nil {
			_ = s.configSvc.SyncUserToFile(ctx, user.GetInboundTagList(), user, false)
		}
	}
	return nil
}
```
Apply the same restore pattern to `BatchResetTraffic`, `BatchRenew`, and `CheckAndResetMonthlyTraffic`.

- [ ] **Step 4: Run test to verify it passes**

Run: `mise exec -- go test -v ./internal/service`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/config_service.go internal/service/user_service.go internal/service/user_service_test.go
git commit -m "feat(service): decouple user config writes from service restart and auto-restore users on traffic reset"
```

---

### Task 5: Frontend & Mock Mode Alignment (`InboundsView.vue` and `mock/storage.ts` Dest Compatibility)

**Files:**
- Modify: `web/src/views/InboundsView.vue`
- Modify: `web/src/mock/storage.ts`

- [ ] **Step 1: Check frontend build and mock configuration**

Verify how `realitySettings.target` is read and saved in `InboundsView.vue`:
Lines 920: `form.value.realityTarget = stream.realitySettings.dest || stream.realitySettings.target || ''`
Lines 1018: save as `dest: form.value.realityTarget || 'www.titech.ac.jp:443'`

In `web/src/mock/storage.ts`:
Change `target` to `dest`.

- [ ] **Step 2: Apply changes to InboundsView.vue and storage.ts**

Ensure `InboundsView.vue` properly handles `dest` as the primary property, and `storage.ts` uses `dest`.

- [ ] **Step 3: Run frontend build or linter check**

Run: `cd web && npm run build` (or verify TypeScript compilation if vite/npm available)

- [ ] **Step 4: Commit**

```bash
git add web/src/views/InboundsView.vue web/src/mock/storage.ts
git commit -m "fix(web): standardize realitySettings to dest with backward compatibility"
```

---

### Task 6: Main Wiring, Full Integration Test & Verification Suite

**Files:**
- Modify: `main.go`
- Test: All tests across the workspace

- [ ] **Step 1: Wire dynamic gRPC port in main.go**

In `main.go`:
Extract port from `cfg.XrayGRPCAddr` (e.g., `net.SplitHostPort` or fallback 8080) and pass to `NewXrayCompiler(grpcPort)` in `configSvc`.

- [ ] **Step 2: Run all unit tests**

Run: `mise exec -- go test -v ./...`
Expected: ALL PASS

- [ ] **Step 3: Verify go vet and build**

Run: `mise exec -- go vet ./...`
Run: `mise exec -- go build -o panel main.go embedded.go`
Expected: Build successfully exits with code 0

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat(main): wire dynamic grpc port into compiler and verify production build"
```
