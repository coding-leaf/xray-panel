# Implementation Plan - Source of Truth 归正与数据一致性治理 (终极完备版)

## Goal Description
针对 `xray-panel` 当前存在的**真相源分裂 (Split-Brain)**、**`config.json` 被当作业务数据库直接读写导致的并发写覆盖**、**启动时无条件从物理文件逆向覆盖数据库导致的“僵尸用户复活与状态倒退”**以及**运行时同步错误被静默吞没**等致命数据一致性缺陷进行针对性治理。

**核心约束**：
1. **不进行大规模目录重构**，不引入抽象中继层（严格遵循 KISS 原则）；
2. 彻底确立 **Primary Database (SQLite)** 为业务状态的唯一真理源；
3. 将 `config.json` 彻底降级为由应用状态**单向编译生成的生成产物 (Compiled Artifact)**，运行期只写不读；
4. 消除普通运行流程中 DB ↔ `config.json` 的隐式双向同步；
5. 明确 BoltDB 绝不作为第二套业务数据库。

---

## 终极审查防线与关键设计决策 (Critical Defenses)

> [!CAUTION]
> ### 1. 纯内存预校验（Pre-validation）防 SQLite 毒化永久锁死
> - **致命隐患**：如果采用“先落库 SQLite ➔ 再编译落盘校验（xray -test）”的逻辑，一旦用户输入了不合规或内核不识别的参数，配置已永久驻留 SQLite；后续所有其他正常操作（新建用户、改端口、定时同步）触发编译时都会因读取到该错误配置而失败，**导致整个面板所有写操作永久性瘫痪锁死**！
> - **铁律防线**：必须实行**纯内存预检管道（Pre-validation Pipeline）**：
>   ```
>   1. 内存中构建候选状态 (Candidate State)
>   2. 纯内存单向编译：compiler.CompileToJSON(..., candidate, ...)
>   3. 内存预校验语法：configMgr.ValidateConfig(ctx, candidateJSON) (xray -test)
>   4. 只有校验 100% 通过，才提交写入 SQLite！(若校验失败直接报错中断，SQLite 零污染)
>   5. POSIX 原子落盘 WriteConfig 并 Reload
>   ```

> [!IMPORTANT]
> ### 2. 升级平滑迁移中的 JSONC 注释清洗防断裂
> - **隐秘陷阱**：现网生产中的 `config.json` 大量包含 `// api`、`/* ... */` 等注释。若从物理磁盘迁移老配置时直接调用 `json.Unmarshal`，会因语法报错判定为迁移失败，导致老用户的出站节点和路由被当成空配置覆盖抹除。
> - **铁律防线**：所有磁盘提取与迁移函数（`migrateOutboundsFromDiskLocked`、`migrateRoutingFromDiskLocked`、`migrateDNSFromDiskLocked`）**必须强制先调用 `jsonc.StripJSONC(raw)` 清洗**后再反序列化。

> [!NOTE]
> ### 3. 全新“零配置”机器上的冷启动默认值兜底（Defaults Fallback）
> - **隐秘陷阱**：在全新 VPS 部署面板时，既没有数据库，磁盘上也根本没有 `config.json` 文件（`ReadRawConfig` 返回 `os.ErrNotExist`）。若读取失败抛出错误，会导致面板初次拉起就崩溃退出。
> - **铁律防线**：迁移函数遇到文件不存在时，优雅返回标准安全默认值：
>   - Outbounds：返回空列表 `[]domain.Outbound{}`（编译器自动注入内置 direct/block）；
>   - Routing：返回默认 `&domain.RoutingConfig{DomainStrategy: "IPIfNonMatch"}`；
>   - DNS：返回标准公共 DNS（1.1.1.1, 8.8.8.8, localhost）；
>   使面板即使在无任何预置文件的裸机上，首次启动也能顺畅自举生成合法配置并拉起 Xray。

> [!IMPORTANT]
> ### 4. settings 表内部核心键隔离（防御前端表单污染）
> - 在 `handler_setting.go` 中设立 `internalSystemKeys` 白名单机制（包含 `jwt_secret`, `xray_outbounds`, `xray_routing_config`, `xray_dns_config`）；
> - `GetSettings` 排除输出，`SaveSettings` 忽略写入，杜绝 `fmt.Sprintf` 将 JSON 误转为非法 Go 语法字符串破坏数据。

> [!IMPORTANT]
> ### 5. 原生配置编辑器与快照回滚的双向对齐
> - `SaveAndApplyRawConfig` 全流程接入 `compileMu` 互斥保护；
> - `syncFromRawJSON` 补齐对 `outbounds`、`routing`、`dns` 的全量反向落库至 SQLite，杜绝下一次常规重编时手写规则被旧数据抹除。

> [!CAUTION]
> ### 6. 规避递归不可重入锁死锁 (Non-Reentrant Lock Safety)
> - 严格区分公开带锁方法与内部无锁 `Locked` 辅助方法：`loadOutboundsLocked`、`loadRoutingLocked`、`loadDNSLocked` 均为内部无锁方法，由外层公开入口统一调度，杜绝非重入死锁。

---

## 新数据流架构设计

```mermaid
flowchart TD
    subgraph ClientLayer ["1. 管理与订阅入口"]
        ADMIN["管理员请求 (Web API)"]
        CLIENT["客户端请求 (/sub 订阅)"]
    end

    subgraph PreValidation ["2. 纯内存预检管道 (Pre-validation Pipeline)"]
        CANDIDATE["内存组装候选状态 (Candidate State)"]
        TEST_COMPILE["compiler.CompileToJSON (单向编译)"]
        XRAY_TEST["configMgr.ValidateConfig (xray -test)"]
    end

    subgraph PrimaryDB ["3. 唯一真理源：Primary Database (SQLite WAL)"]
        USERS_DB[("users 表 (用户实体/配额/累计流量)")]
        INBOUNDS_DB[("inbounds 表 (入站节点/分流规则)")]
        SETTINGS_DB[("settings 表 (outbounds / routing / dns / 系统设置)")]
    end

    subgraph ArtifactRuntime ["4. 产物与运行时"]
        CONFIG_JSON[("config.json (生成产物，运行期只写不读)")]
        XRAY_PROC[["Xray-Core 守护进程 (Runtime Port gRPC 10085)"]]
    end

    %% 写流程：预检 -> 校验通过 -> 提交 DB -> 写盘
    ADMIN -->|① 发起配置修改| CANDIDATE
    CANDIDATE -->|② 内存试编译| TEST_COMPILE
    TEST_COMPILE -->|③ 内核语法预检| XRAY_TEST
    XRAY_TEST -- 校验失败 --> ERROR["直接报错返回, SQLite 零污染 🛑"]
    XRAY_TEST -- 校验通过 -->|④ 提交事务落库| PrimaryDB
    PrimaryDB -->|⑤ 原子写盘 WriteConfig| CONFIG_JSON
    CONFIG_JSON -.->|⑥ systemctl reload| XRAY_PROC

    %% 订阅与读流程：100% 走 DB
    CLIENT -->|读取节点与用户授权| USERS_DB & INBOUNDS_DB
```

---

## Proposed Changes

### 1. HTTP 接入层：系统保留键防污染隔离

#### [MODIFY] [`internal/delivery/http/handler_setting.go`](file:///home/yezisama/workspace/WorkSpace/xray-panel/internal/delivery/http/handler_setting.go)
```go
// 内部保留核心系统配置键，严禁向前端表单暴露，严禁被通用表单保存覆写
var internalSystemKeys = map[string]bool{
	"jwt_secret":          true,
	"xray_outbounds":       true,
	"xray_routing_config": true,
	"xray_dns_config":     true,
}

func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingRepo.GetAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for k := range internalSystemKeys {
		delete(settings, k)
	}
	c.JSON(http.StatusOK, settings)
}

func (h *SettingHandler) SaveSettings(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for k, v := range body {
		if v == nil || internalSystemKeys[k] {
			continue // 忽略保留键，杜绝 fmt.Sprintf 非法格式污染
		}
		strVal := fmt.Sprintf("%v", v)
		_ = h.settingRepo.Set(c.Request.Context(), k, strVal)
	}
	// 后续动态更新逻辑保持不变...
}
```

---

### 2. 核心业务服务层：预检防毒化、JSONC清洗与统一管道

#### [MODIFY] [`internal/service/config_service.go`](file:///home/yezisama/workspace/WorkSpace/xray-panel/internal/service/config_service.go)

1. **结构体声明与常量键**：
```go
type ConfigService struct {
	configMgr    *xray.ConfigManager
	supervisor   ServiceSupervisor
	inboundRepo  domain.InboundRepository
	userRepo     domain.UserRepository
	snapshotRepo domain.ConfigSnapshotRepository
	settingRepo  domain.SettingRepository
	compiler     *xray.XrayCompiler
	compileMu    sync.Mutex // 保护编译管道、写盘及 Outbound/Routing/DNS 并发操作
}

const (
	settingKeyOutbounds = "xray_outbounds"
	settingKeyRouting   = "xray_routing_config"
	settingKeyDNS       = "xray_dns_config"
)
```

2. **带 JSONC 清洗与零配置默认兜底的内部读取方法**：
```go
func (s *ConfigService) migrateOutboundsFromDiskLocked(ctx context.Context) []domain.Outbound {
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil || len(raw) == 0 {
		return []domain.Outbound{} // 零配置兜底
	}
	cleaned := jsonc.StripJSONC(raw) // 强制清洗 JSONC 注释
	var root struct {
		Outbounds []domain.Outbound `json:"outbounds"`
	}
	if err := json.Unmarshal(cleaned, &root); err == nil && len(root.Outbounds) > 0 {
		bytes, _ := json.Marshal(root.Outbounds)
		if s.settingRepo != nil {
			_ = s.settingRepo.Set(ctx, settingKeyOutbounds, string(bytes))
		}
		return root.Outbounds
	}
	return []domain.Outbound{}
}

func (s *ConfigService) migrateRoutingFromDiskLocked(ctx context.Context) *domain.RoutingConfig {
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil || len(raw) == 0 {
		return &domain.RoutingConfig{DomainStrategy: "IPIfNonMatch"} // 零配置兜底
	}
	cleaned := jsonc.StripJSONC(raw) // 强制清洗 JSONC 注释
	var root struct {
		Routing domain.RoutingConfig `json:"routing"`
	}
	if err := json.Unmarshal(cleaned, &root); err == nil {
		if root.Routing.DomainStrategy == "" {
			root.Routing.DomainStrategy = "IPIfNonMatch"
		}
		bytes, _ := json.Marshal(root.Routing)
		if s.settingRepo != nil {
			_ = s.settingRepo.Set(ctx, settingKeyRouting, string(bytes))
		}
		return &root.Routing
	}
	return &domain.RoutingConfig{DomainStrategy: "IPIfNonMatch"}
}

func (s *ConfigService) migrateDNSFromDiskLocked(ctx context.Context) *domain.DNSConfig {
	raw, err := s.configMgr.ReadRawConfig()
	defaultDNS := &domain.DNSConfig{
		Servers:       []interface{}{"https://1.1.1.1/dns-query", "8.8.8.8", "localhost"},
		QueryStrategy: "UseIP",
	}
	if err != nil || len(raw) == 0 {
		return defaultDNS // 零配置兜底
	}
	cleaned := jsonc.StripJSONC(raw) // 强制清洗 JSONC 注释
	var root struct {
		DNS domain.DNSConfig `json:"dns"`
	}
	if err := json.Unmarshal(cleaned, &root); err == nil {
		if len(root.DNS.Servers) == 0 {
			root.DNS = *defaultDNS
		}
		bytes, _ := json.Marshal(root.DNS)
		if s.settingRepo != nil {
			_ = s.settingRepo.Set(ctx, settingKeyDNS, string(bytes))
		}
		return &root.DNS
	}
	return defaultDNS
}
```

3. **核心纯内存预校验与原子写盘管道（Pre-validation Pipeline）**：
```go
// candidateCompileAndValidateLocked: 纯内存组装 ➔ 内存编译 ➔ xray -test 预检
// 校验失败直接返回 error，绝不写入 SQLite，彻底杜绝真理源被毒化！
func (s *ConfigService) candidateCompileAndValidateLocked(
	ctx context.Context,
	inbounds []domain.Inbound,
	outbounds []domain.Outbound,
	routing *domain.RoutingConfig,
	dns *domain.DNSConfig,
	users []domain.User,
) ([]byte, error) {
	jsonBytes, err := s.compiler.CompileToJSON(inbounds, outbounds, routing, dns, users)
	if err != nil {
		return nil, fmt.Errorf("compile config failed: %w", err)
	}
	if err := s.configMgr.ValidateConfig(ctx, jsonBytes); err != nil {
		return nil, fmt.Errorf("pre-validation failed: %w", err)
	}
	return jsonBytes, nil
}
```

4. **Outbound / Routing / DNS 的预校验落库操作**：
以 `SaveOutbound` 为例（其他方法遵循相同预检提交范式）：
```go
func (s *ConfigService) SaveOutbound(ctx context.Context, outbound domain.Outbound) error {
	s.compileMu.Lock()
	defer s.compileMu.Unlock()

	// 1. 内存中构建候选状态
	existingList := s.loadOutboundsLocked(ctx)
	found := false
	var candidateList []domain.Outbound
	for _, ob := range existingList {
		if ob.Tag == outbound.Tag {
			found = true
			candidateList = append(candidateList, outbound)
		} else {
			candidateList = append(candidateList, ob)
		}
	}
	if !found {
		candidateList = append(candidateList, outbound)
	}

	// 2. 收集其余当前有效业务状态
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		inbounds, _ = s.inboundRepo.ListAll(ctx)
	}
	routing := s.loadRoutingLocked(ctx)
	dns := s.loadDNSLocked(ctx)
	var users []domain.User
	if s.userRepo != nil {
		users, _ = s.userRepo.ListAll(ctx)
	}

	// 3. 纯内存预校验：如果语法不合法在此立即报错返回，SQLite 保持绝对纯净！
	jsonBytes, err := s.candidateCompileAndValidateLocked(ctx, inbounds, candidateList, routing, dns, users)
	if err != nil {
		return err
	}

	// 4. 预检通过：正式提交至 SQLite
	bytes, _ := json.Marshal(candidateList)
	if s.settingRepo != nil {
		_ = s.settingRepo.Set(ctx, settingKeyOutbounds, string(bytes))
	}

	// 5. 写入物理磁盘并平滑重载
	s.recordSnapshotBeforeWrite(ctx, fmt.Sprintf("保存出站节点 %s", outbound.Tag))
	if err := s.configMgr.WriteConfig(ctx, jsonBytes); err != nil {
		return err
	}
	if s.supervisor != nil {
		return s.supervisor.Reload(ctx)
	}
	return nil
}
```

5. **原生配置保存 (`SaveAndApplyRawConfig`) 全量反向落库**：
```go
func (s *ConfigService) SaveAndApplyRawConfig(ctx context.Context, rawJSON []byte) error {
	s.compileMu.Lock()
	defer s.compileMu.Unlock()

	// 1. 校验裸配置
	if err := s.configMgr.ValidateConfig(ctx, rawJSON); err != nil {
		return err
	}

	s.recordSnapshotBeforeWrite(ctx, "保存并应用原生 JSON 配置")
	if err := s.configMgr.WriteConfig(ctx, rawJSON); err != nil {
		return err
	}

	// 2. 全量反向同步：不仅同步 Inbounds 和 Users，同时将 Outbounds, Routing, DNS 反向落库至 SQLite
	_ = s.syncFromRawJSON(ctx, rawJSON)
	if s.supervisor != nil {
		return s.supervisor.Reload(ctx)
	}
	return nil
}
```
并在 `syncFromRawJSON` 中提取并落库：
- `root["outbounds"]` ➔ `settingRepo.Set(ctx, settingKeyOutbounds, ...)`
- `root["routing"]` ➔ `settingRepo.Set(ctx, settingKeyRouting, ...)`
- `root["dns"]` ➔ `settingRepo.Set(ctx, settingKeyDNS, ...)`

6. **安全冷引导 `SeedFromDiskIfEmpty` 替代无条件 `SyncFromFile`**：
```go
func (s *ConfigService) SeedFromDiskIfEmpty(ctx context.Context) error {
	if s.inboundRepo != nil {
		inbounds, _ := s.inboundRepo.ListAll(ctx)
		if len(inbounds) > 0 {
			return nil // 已有节点数据，坚决不逆向覆写
		}
	}
	if s.userRepo != nil {
		users, _ := s.userRepo.ListAll(ctx)
		if len(users) > 0 {
			return nil
		}
	}
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil || len(raw) == 0 {
		return nil
	}
	return s.syncFromRawJSON(ctx, raw)
}
```

---

### 3. 用户服务层：全生命周期清除隐式吞错

#### [MODIFY] [`internal/service/user_service.go`](file:///home/yezisama/workspace/WorkSpace/xray-panel/internal/service/user_service.go)

在 `CreateUser`、`UpdateUser`、`DeleteUser`、`ResetTraffic`、`ResetSubToken` 中，全面替换 `_ =`：
```go
// 1. gRPC 添加用户
if err := s.xrayManager.AddUser(ctx, t, user); err != nil {
	slog.Warn("Failed to sync user addition to Xray memory via gRPC",
		slog.String("email", user.Email),
		slog.String("inbound", t),
		slog.String("error", err.Error()),
	)
}

// 2. gRPC 移除用户
if err := s.xrayManager.RemoveUser(ctx, t, user.Email); err != nil {
	slog.Warn("Failed to remove user from Xray memory via gRPC",
		slog.String("email", user.Email),
		slog.String("inbound", t),
		slog.String("error", err.Error()),
	)
}

// 3. 单向编译持久化落盘
if err := s.configSvc.SyncUserToFile(ctx, tags, user, false); err != nil {
	slog.Warn("Failed to compile user to config.json",
		slog.String("email", user.Email),
		slog.String("error", err.Error()),
	)
}
```

---

### 4. 程序入口层：启动治理

#### [MODIFY] [`main.go`](file:///home/yezisama/workspace/WorkSpace/xray-panel/main.go)
```diff
-	configSvc := service.NewConfigService(configMgr, supervisor, inboundRepo, userRepo, snapshotRepo, compiler)
-	_ = configSvc.SyncFromFile(bgCtx)
-	_ = configSvc.RecompileAndApply(bgCtx, "面板启动自动同步与编译配置")
+	configSvc := service.NewConfigService(configMgr, supervisor, inboundRepo, userRepo, snapshotRepo, settingRepo, compiler)
+	_ = configSvc.SeedFromDiskIfEmpty(bgCtx)
+	_ = configSvc.RecompileAndApply(bgCtx, "面板启动自动编译配置")
```

---

### 5. 单元测试适配与补充

#### [MODIFY] [`internal/service/config_service_test.go`](file:///home/yezisama/workspace/WorkSpace/xray-panel/internal/service/config_service_test.go)
#### [MODIFY] [`internal/service/user_service_test.go`](file:///home/yezisama/workspace/WorkSpace/xray-panel/internal/service/user_service_test.go)

1. 适配 `NewConfigService` 新签名；
2. 新增防毒化与防并发单测：
   - `TestConfigService_SaveOutbound_PreValidationPreventsPoisoning`: 传入非法配置，验证接口报错中断且 SQLite **绝对未被污染**；
   - `TestConfigService_RawEditor_SyncsToSQLite`: 验证通过 `SaveAndApplyRawConfig` 保存后，SQLite 中的 Outbounds/Routing/DNS 成功落库；
   - `TestConfigService_SeedFromDiskIfEmpty_PreventsZombieUser`: 验证数据库已有数据时，绝不从磁盘逆向复活用户；
   - `TestSettingHandler_SystemKeys_FilterOut`: 验证 `/api/settings` 不泄露且不破坏 `xray_outbounds`。

---

## Verification Plan

### Automated Tests
1. **并发竞态与全量测试**：
   ```bash
   go test -v -race ./...
   ```
2. **专项回归测试**：
   ```bash
   go test -v -run TestConfigService ./internal/service/...
   go test -v -run TestUserService ./internal/service/...
   go test -v ./internal/delivery/http/...
   ```
3. **整体二进制构建**：
   ```bash
   go build -o /dev/null .
   ```

### Manual Verification
1. **防毒化测试**：故意传入一个内核无法识别的错误出站参数，验证系统拒绝保存，且后续添加正常用户仍能成功，证实 SQLite 未被锁死；
2. **设置页面防污染检查**：在前端“系统设置”页面修改任意配置并保存，查询数据库验证 `xray_outbounds` 未被冲刷或损坏；
3. **防僵尸复活验证**：在 SQLite 中删除用户，磁盘保留旧用户，重启服务确认用户未复活；
4. **原生编辑器反向同步验证**：在 Web 原生配置编辑器修改一个出站并保存，调用 Inbound 更新，确认修改后的出站依然存在且生效。
