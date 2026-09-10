# 基于动态高熵凭证（Ticket）与带外中立门户的安全分发系统实施计划 (v3.1 自我审查完善版)

> **Goal:** 彻底解决节点与订阅在境内即时通讯软件（微信/QQ）分发时的脆弱与被动局面，消除明文静态链接（`/sub/:token`）与裸露节点（`vless://`）被爬虫抓取打标的风险。通过“标准 Crockford Base32 编码（无偏5-bit采样 + 全角转半角归一化） + 结构化 IP 熔断惩罚（滑动窗口 + 30分钟封禁） + 全局总流控 + CAS 条件原子扣减 + 用户有效性核验 + 双轨交付 + 白板中立门户 + Cloudflare 边缘反代闭环”模型，彻底攻克 SNI RST 阻断，实现安全、体面、免封的工业级分发体系。

**Architecture:**
- **信令与数据严格物理隔离**：微信聊天仅作为信令通知通道，流转中立域名与 6 位无特征动态码，不出现任何协议关键字、IP、端口或 UUID。
- **数学严密的标准 Crockford Base32 编码**：使用标准 32 字符集（0-9 与 A-Z 排除 I, L, O, U），以 `b & 0x1F` 纯位运算进行无偏采样生成，离散空间为 $2^{30} = 1,073,741,824$（10.74 亿）；输入端进行全角转半角（Full-width to Half-width, `r - 0xFEE0`）及混淆字符（`O/o -> 0`, `I/L/i/l -> 1`）归一化。
- **结构化双层 IP 熔断与全局速率防御**：
  - 引入 `ipFailureRecord`（区分 10 分钟失败滑动窗口与 30 分钟封禁期），连续 5 次输错封禁 30 分钟；
  - 接口配置双层限流：单 IP 限流（`5-M`）+ **真正的全局共享流控**（`NewGlobalRateLimiter("60-M", "global_portal")`，显式覆盖默认 IP KeyGetter），防止分布式代理池冲击与 SQLite 连接争抢；
  - 显式配置 Gin `r.SetTrustedProxies`，配合 Nginx 覆盖 `X-Forwarded-For`，防止客户端伪造 IP 绕过限流。
- **CAS 条件原子扣减与用户状态双重守门**：
  - 兑换时先执行原子条件更新（`UPDATE tickets SET remaining_uses = remaining_uses - 1 WHERE code = ? AND remaining_uses > 0 AND expires_at > ?`，使用 `int64` 整数时间戳根除 SQLite 时区陷阱）；
  - 仅当 `RowsAffected == 1` 时才进入节点下发；
  - 下发前严格校验关联用户的有效性（`user.Enabled` 与 `user.IsActive()`），若用户已被禁用或欠费，拒绝下发并返回错误，避免废弃账号泄漏节点。
- **前端彻底白板隔离与 401 拦截切断**：
  - 重构 `App.vue`，使 `/portal` 彻底脱离侧边栏与管理顶栏；
  - `App.vue` 中的 `fetchCoreStatus` 增加 `isBlankLayout` 守卫，挂起轮询，并在 `api/index.ts` 中排除 `/portal`，防止 401 将未登录用户在 6 秒内踢回 `/login`；
  - 微信 XWeb 剪贴板自动复制（多节点自动以 `\n` 换行拼接）+ 长按只读 `<textarea>` 兜底；
  - 深度融合 `UsersView.vue` 现有 `Share & Subscription Modal`，增加第三个 “安全提取码” 标签页；
  - 补齐前端 Mock 模式并提供 v2rayNG / Clash / Shadowrocket 客户端开启“走代理更新”的折叠指引。
- **威胁模型 5（域名 SNI 阻断）闭环**：
  - 单机裸 VPS 域名在境内移动 4G/5G 仍存在 SNI RST 风险；
  - Task 5 提供开箱即用的 **Cloudflare Worker 边缘反代脚本模板**，使境内流量直连大厂 Anycast IP，彻底斩断对 VPS 被阻断域名的直接依赖。
- **指纹全面去敏感化**：
  - 修正 `router.go:173` 后端 SPA 未构建时的兜底文案，由 `"Xray Decoupled Panel API is running."` 改为中立的 `"System API is running."`。

**Tech Stack:** Go 1.22+, GORM, Gin, go-cache, ulule/limiter/v3, glebarez/sqlite, Vue 3, Vite, Tailwind CSS, TypeScript.

---

## 实施任务列表 (Tasks)

### Task 1: 数据模型、持久化仓储与领域错误 (Domain & Repository)

**Files:**
- Create: `internal/domain/ticket.go`
- Modify: `internal/domain/errors.go`
- Create: `internal/adapter/repository/ticket_repo.go`
- Modify: `internal/adapter/repository/sqlite_db.go`
- Create: `internal/adapter/repository/ticket_repo_test.go`

- [x] **Step 1: 定义领域模型与错误契约**
  在 `internal/domain/errors.go` 追加：
  ```go
  ErrTicketInvalidOrExpired = errors.New("invalid, expired or consumed ticket")
  ErrIPRateLimited          = errors.New("ip temporarily banned due to excessive failed attempts")
  ```
  在 `internal/domain/ticket.go` 中定义实体（**采用 int64 整数时间戳根除时区隐患**）：
  ```go
  type Ticket struct {
      ID            uint   `gorm:"primaryKey" json:"id"`
      Code          string `gorm:"size:16;uniqueIndex;not null" json:"code"` // 6位 Crockford Base32
      UserID        uint   `gorm:"index;not null" json:"user_id"`
      User          *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
      RemainingUses int    `gorm:"default:2;not null" json:"remaining_uses"` // 默认允许兑换 2 次
      ExpiresAt     int64  `gorm:"index;not null" json:"expires_at"`         // Unix 秒级时间戳
      CreatedAt     int64  `json:"created_at"`
  }

  type TicketClaimPayload struct {
      UserEmail       string   `json:"user_email"`
      EmergencyNodes  []string `json:"emergency_nodes"`  // VLESS 节点链接切片
      SubscriptionURL string   `json:"subscription_url"` // 隧道内自动更新订阅链接
      RemainingUses   int      `json:"remaining_uses"`
      ExpiresAt       int64    `json:"expires_at"`
  }

  type TicketRepository interface {
      Create(ctx context.Context, ticket *Ticket) error
      GetByCode(ctx context.Context, code string) (*Ticket, error)
      ConsumeAtomic(ctx context.Context, code string) (*Ticket, error) // CAS 原子扣减可用次数并返回票据
      Delete(ctx context.Context, id uint) error
      CleanExpired(ctx context.Context) error
  }
  ```

- [x] **Step 2: 实现 SQLite 仓储与 CAS 条件原子操作**
  在 `internal/adapter/repository/ticket_repo.go` 中实现：
  - `ConsumeAtomic`：
    ```go
    now := time.Now().Unix()
    result := r.db.WithContext(ctx).Model(&domain.Ticket{}).
        Where("code = ? AND remaining_uses > 0 AND expires_at > ?", code, now).
        Update("remaining_uses", gorm.Expr("remaining_uses - 1"))
    if result.Error != nil {
        return nil, result.Error
    }
    if result.RowsAffected == 0 {
        return nil, domain.ErrTicketInvalidOrExpired
    }
    var ticket domain.Ticket
    if err := r.db.WithContext(ctx).Preload("User").Where("code = ?", code).First(&ticket).Error; err != nil {
        return nil, err
    }
    return &ticket, nil
    ```
  - `CleanExpired`：`DELETE FROM tickets WHERE expires_at < ? OR remaining_uses <= 0`。

- [x] **Step 3: 注册数据库自动迁移**
  在 `internal/adapter/repository/sqlite_db.go` 的 `db.AutoMigrate` 中追加 `&domain.Ticket{}`。

- [x] **Step 4: 编写仓储层单元测试**
  在 `internal/adapter/repository/ticket_repo_test.go` 中测试：创建、读取、CAS 原子扣减、过期比较及清理。

---

### Task 2: 业务逻辑层与结构化防爆破熔断 (TicketService)

**Files:**
- Create: `internal/service/ticket_service.go`
- Create: `internal/service/ticket_service_test.go`

- [x] **Step 1: 编写标准 Crockford Base32 高熵无偏随机码生成与全角转半角归一化**
  - 字符表：`0123456789ABCDEFGHJKMNPQRSTVWXYZ`（共 32 字符，排除易混淆的 I, L, O, U）；
  - 使用 `crypto/rand.Read` 读取随机字节，对每个字符提取 `b & 0x1F`（无偏 5-bit 位运算，零模偏差）；
  - 编写 `NormalizeTicketCode(input string) string`：
    1. **全角转半角映射**：遍历字符，若 `r >= 0xFF01 && r <= 0xFF5E` 则 `r -= 0xFEE0`，若 `r == 0x3000` 则置空格；
    2. 去除所有空白字符与 `-`、`_`；
    3. 转大写；
    4. 混淆映射：`O` -> `0`, `I` -> `1`, `L` -> `1`。

- [x] **Step 2: 实现结构化 IP 失败惩罚机制**
  定义 IP 失败记录模型：
  ```go
  type ipFailureRecord struct {
      FailCount   int
      WindowStart time.Time
      BannedUntil time.Time
  }
  ```
  - 初始化 `failedIPCache := cache.New(1*time.Hour, 10*time.Minute)`；
  - 校验逻辑：
    - 若当前时间处于 `BannedUntil` 之前，直接返回 `domain.ErrIPRateLimited`；
    - 若当前时间距离 `WindowStart` 超过 10 分钟，重置 `FailCount = 0, WindowStart = now`；
    - 失败累加：当失败次数达到 5 次，设置 `BannedUntil = now.Add(30 * time.Minute)`；
    - 成功兑换：清空该 IP 记录。

- [x] **Step 3: 实现 TicketService 核心用例与用户状态核验**
  - 构造函数：`NewTicketService(ticketRepo domain.TicketRepository, userRepo domain.UserRepository, subSvc *SubService, settingRepo domain.SettingRepository)`；
  - `GenerateTicket(ctx context.Context, userID uint, ttlMinutes int, maxUses int) (*domain.Ticket, string, error)`：
    - 生成 6 位随机码并入库；
    - 安全异步清理：`cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)`，异步调用 `ticketRepo.CleanExpired(cleanCtx)`；
    - 严格从 `settingRepo` 优先读取 `portal_url` 或 `public_url`，未配置时降级为 reqHost，拼装安全分享文案；
  - `ClaimTicket(ctx context.Context, code string, clientIP string, reqHost string) (*domain.TicketClaimPayload, error)`：
    - 校验 clientIP 熔断状态；
    - 归一化清洗 code；
    - 调用 `ticketRepo.ConsumeAtomic(ctx, cleanCode)`：
      - 失败时累加 IP 失败计数，统一返回模糊错误 `"凭据无效、已过期或已被销毁"`；
      - 成功时清除 IP 失败计数；
    - **用户状态二次安全校验**：
      ```go
      user := ticket.User
      if user == nil || !user.Enabled {
          return nil, domain.ErrUserDisabled
      }
      if !user.IsActive() {
          return nil, domain.ErrQuotaExceeded
      }
      ```
    - 调用 `subSvc.GetUserShareInfo(ctx, user.ID, baseURL)` 获取急救节点切片与长效订阅 URL；
    - 返回 `TicketClaimPayload`。

- [x] **Step 4: 编写业务层单元测试**
  在 `internal/service/ticket_service_test.go` 中测试：随机数位运算分布、全角归一化、用户禁用拦截、CAS 并发竞争、IP 连续 5 次输错触发 30 分钟封禁与窗口重置。

---

### Task 3: HTTP 交付层、系统装配与路由 (Handler, Router & Main)

**Files:**
- Modify: `internal/delivery/http/middleware/limiter.go` (增加全局 Key 流控函数)
- Create: `internal/delivery/http/handler_ticket.go`
- Modify: `internal/delivery/http/router.go`
- Modify: `main.go`
- Modify: `internal/delivery/cron/traffic_sync.go`
- Modify: `internal/delivery/cron/traffic_sync_test.go`
- Create: `internal/delivery/http/handler_ticket_test.go`

- [x] **Step 1: 在 `middleware/limiter.go` 中支持真正的全局流控**
  实现 `NewGlobalRateLimiter(rateFormatted string, key string) gin.HandlerFunc`：
  通过 `ginlimiter.WithKeyGetter(func(c *gin.Context) string { return key })` 覆盖默认 IP 提取器，实现真正的全站共享频控。

- [x] **Step 2: 实现 Handler 接口**
  在 `internal/delivery/http/handler_ticket.go` 中：
  - `CreateTicket(c *gin.Context)`（管理员鉴权路由）：
    - 接收 `{ "ttl_minutes": 15, "max_uses": 2 }`，URL 参数获取 `id`；
    - 返回新凭据及微信分享文案；
  - `ClaimTicket(c *gin.Context)`（公开兑换路由）：
    - 接收 `{ "code": "..." }`；
    - 读取客户端真实 IP：`clientIP := c.ClientIP()`；
    - 统一模糊报错，阻断侧信道探测。

- [x] **Step 3: 修改 router.go 配置安全反代与双层速率限制**
  - 在 `SetupRouter` 中配置受信任代理：`_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})`；
  - 挂载路由：
    - 单 IP 频控：`portalIPLimiter := middleware.NewRateLimiter("5-M")`；
    - 全局总频控：`portalGlobalLimiter := middleware.NewGlobalRateLimiter("60-M", "global_portal_claim")`；
    - 公开兑换接口：`api.POST("/portal/claim", portalGlobalLimiter, portalIPLimiter, handlers.Ticket.ClaimTicket)`；
    - 管理员路由：在 `authGroup` 下注册 `authGroup.POST("/users/:id/tickets", handlers.Ticket.CreateTicket)`；
  - 去敏感化修改：将第 173 行未构建前端时的 fallback 提示语修改为中立的 `"System API is running. Assets not found."`。

- [x] **Step 4: 修改 main.go 完成单例依赖装配**
  在 `main.go` 组装段：
  - `ticketRepo := repository.NewTicketRepository(db)`；
  - `ticketSvc := service.NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)`；
  - `ticketHandler := deliveryHTTP.NewTicketHandler(ticketSvc)`；
  - 注入到 `handlers := &deliveryHTTP.Handlers{ ..., Ticket: ticketHandler }`。

- [x] **Step 5: 修改 traffic_sync.go 与 traffic_sync_test.go**
  - `traffic_sync.go`：在 5 分钟定时维护循环中追加 `_ = j.ticketRepo.CleanExpired(ctx)`；
  - `traffic_sync_test.go`：同步更新 `NewTrafficSyncJob` 构造函数的调用入参。

---

### Task 4: 前端白板化中立伪装、401 阻断与全平台交互 (Frontend UI)

**Files:**
- Modify: `web/src/App.vue` (彻底隔离白板布局，阻断 401 级联跳转与控制台指纹)
- Modify: `web/src/api/index.ts` (修复 401 强制跳转排除 /portal)
- Modify: `web/src/router/index.ts` (配置中立路由与专属标题)
- Create: `web/src/views/PortalClaimView.vue` (中立兑换单页 + 多节点换行复制 + 客户端帮助图解 + 剪贴板兜底)
- Modify: `web/src/views/UsersView.vue` (用户管理 ShareModal 增加第三个“安全提取码” Tab)
- Modify: `web/src/mock/index.ts` (同步补充前端 Mock 桩函数)

- [x] **Step 1: 修复 `web/src/App.vue` 与 `web/src/api/index.ts` 的 401 踢出漏洞**
  - 在 `web/src/App.vue` 中定义白板页面判断：
    ```ts
    const isBlankLayout = computed(() => ['/login', '/portal'].includes(route.path) || route.meta?.layout === 'blank')
    ```
  - **阻断 401 轮询**：修改第 378 行 `fetchCoreStatus` 内部守卫：
    ```ts
    const fetchCoreStatus = async () => {
      if (isBlankLayout.value) return // 阻止白板页面发起鉴权请求！
      ...
    }
    ```
  - 在 `web/src/api/index.ts` 响应拦截器中排除 `/portal`：
    ```ts
    if (error.response && error.response.status === 401) {
      localStorage.removeItem('token')
      if (window.location.pathname !== '/login' && window.location.pathname !== '/portal') {
        window.location.href = '/login'
      }
    }
    ```

- [x] **Step 2: 配置 `web/src/router/index.ts`**
  - 注册公开路由：`{ path: '/portal', component: () => import('../views/PortalClaimView.vue') }`；
  - `afterEach` 中增加标题保护：`if (to.path === '/portal') { document.title = '安全数据交换网关' }`。

- [x] **Step 3: 编写中立凭据提取单页 `PortalClaimView.vue`**
  - **视觉设计**：极简深空黑灰配色，中立标题 `安全数据交换网关 (Secure Data Exchange Gateway)`；
  - **输入交互**：
    - 大字号 6 位输入框，支持全角自动半角化与大写转换；
    - 点击“立即提取”，展示平滑 Loading 与错误提示；
  - **双轨交付与微信剪贴板兜底**：
    - **轨道 1【急救连接节点】**：
      - 主按钮：一键复制全部 VLESS 节点（自动执行 `payload.emergency_nodes.join('\n')`，完美兼容 v2rayNG 多节点剪贴板批量导入）；
      - 兜底区域：提供只读 `<textarea>` 自动选中文本，标明“若点击未成功复制，请长按文本手动复制”；
      - 客户端快速指引折叠卡片（提供 v2rayNG / Clash / Shadowrocket 简明 3 步操作说明）；
    - **轨道 2【长效更新订阅】**：
      - 展示长效订阅链接与一键复制；
      - 折叠式指引图文：手把手教会用户在客户端中勾选“通过代理更新订阅”；
    - **安全自毁机制**：提取成功后 120 秒自动清空内存数据并重置。

- [x] **Step 4: 升级 `UsersView.vue` 分享弹窗**
  - 在 `Share & Subscription Modal` 中增加第 3 个标签页：`activeShareTab = 'ticket'`（安全提取码）；
  - 支持选择有效时间（15m / 30m / 1h，默认 15m）与最大使用次数（默认 2 次）；
  - 点击“生成安全提取码”后，展示高亮 6 位码与微信分享卡片，提供“一键复制分享文案”按钮。

- [x] **Step 5: 同步补充前端 Mock (`web/src/mock/index.ts`)**
  - 拦截 `POST /portal/claim` 与 `POST /users/:id/tickets`，返回格式化桩数据，保证 `pnpm dev:demo` 正常运行。

---

### Task 5: 生产环境网络部署加固与 Cloudflare 边缘反代闭环 (Nginx & Cloudflare)

**Files:**
- Modify: `deploy/nginx-sample.conf`
- Create: `deploy/cloudflare-worker-sub-proxy.js`

- [x] **Step 1: 加固 VPS 443 端口兜底分流，杜绝直接探测 IP 泄露证书**
  修改 `deploy/nginx-sample.conf` 中 Stream 模块的 `default` 兜底指向 `xray_reality_node`：
  ```nginx
  map $ssl_preread_server_name $backend_upstream {
      reality.example.com    xray_reality_node;
      panel.yourdomain.com   nginx_web_tls;
      default                xray_reality_node; # 任何未匹配的 SNI 或直接探测 IP，全部透传给 REALITY 入站
  }
  ```
  在 HTTP 反代块中严格覆盖客户端 IP 头：
  ```nginx
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  ```

- [x] **Step 2: 提供开箱即用的 Cloudflare Worker 边缘反代脚本模板**
  创建 `deploy/cloudflare-worker-sub-proxy.js`，包含：
  - 微信/QQ 爬虫 UA 拦截与假白屏响应；
  - 向 VPS 反向代理中立门户 `/portal` 与兑换接口 `/api/portal/claim`；
  - 客户端握手域名为 Cloudflare 边缘分配域名，彻底终结境内移动 4G/5G 对未备案 VPS 域名的 SNI TCP RST 阻断。

---

## 验证与验收标准 (Verification Plan)

### 1. 自动化测试
```bash
# 1. 运行持久化仓储单元测试 (验证整数时间戳与 CAS 原子扣减)
go test -v ./internal/adapter/repository/... -run TestTicketRepo

# 2. 运行业务层单元测试 (验证 Crockford 编码无偏采样、全角清洗、用户禁用拦截、双层 IP 熔断与 30 分钟封禁)
go test -v ./internal/service/... -run TestTicketService

# 3. 运行 HTTP 接口测试与限流验证 (含 SetTrustedProxies 与 NewGlobalRateLimiter 全局流控)
go test -v ./internal/delivery/http/... -run TestTicketHandler

# 4. 运行定时任务与回归测试
go test -v ./internal/delivery/cron/... -run TestTrafficSyncJob

# 5. 全量编译与竞态检测
go vet ./...
go test -race ./internal/...
```

### 2. 手工与集成安全验证
1. **中立伪装与 401 拦截阻断验证**：
   - 隐身窗口直接访问 `http://<host>:<port>/portal`；
   - 静置等待 15 秒以上，确认不会触发 `fetchCoreStatus` 401 报错，绝对不会被重定向至 `/login`；
   - 确认无侧边栏与管理后台指纹，标题为 `安全数据交换网关`。
2. **防爆破与双层限流熔断验证**：
   - 连续向 `/api/portal/claim` 提交 5 次错误验证码；
   - 确认第 5 次后系统立即返回 HTTP 429 封禁 30 分钟提示，即刻输入正确码也被拒绝，验证滑动窗口熔断生效；
   - 伪造 `X-Forwarded-For` 发起请求，验证无法绕过单 IP 封禁（验证 `SetTrustedProxies` 生效）。
3. **真实端到端分发验证**：
   - 管理后台为测试用户生成提取码（例如 `7K9X2P`）；
   - 在手机移动 4G/5G 网络下，通过手机浏览器打开 `/portal`；
   - 输入全角混合小写码（例如 `７ｋ９ｘ２ｐ` 或输入包含 `o/l`），验证全角半角化与混淆字符清洗成功兑换；
   - 点击“一键复制急救节点”，粘贴至手机 v2rayNG 从剪贴板导入，验证秒级通网；
   - 在客户端勾选“通过代理更新订阅”，验证长效订阅更新畅通无阻。
