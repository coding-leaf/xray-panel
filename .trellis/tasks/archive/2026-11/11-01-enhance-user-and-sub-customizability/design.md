# 用户与订阅高可定制化改造 - 技术设计 (Design)

## 1. 架构边界与分层契约

本改造遵循系统的整洁分层架构，各层边界与职责契约划分如下：

```
┌────────────────────────────────────────────────────────┐
│              Web Presentation Layer                    │
│  - UserFormDrawer.vue / UserTable.vue / DetailDrawer   │
│  - InboundFormDrawer.vue / SettingsView.vue            │
└───────────────────────────┬────────────────────────────┘
                            │ HTTP JSON / REST
┌───────────────────────────▼────────────────────────────┐
│              Delivery Layer (HTTP Handlers)            │
│  - handler_user.go: Create / Update / Delete DTO 绑定  │
│  - handler_sub.go: 订阅分发、Profile 响应头注入        │
└───────────────────────────┬────────────────────────────┘
                            │ Go Interface Calls
┌───────────────────────────▼────────────────────────────┐
│              Core Orchestration Service Layer          │
│  - UserService: 用户凭证管理、唯一性守卫、Saga补偿、   │
│                 SubRoute 级联更新、Xray 内存热重载      │
│  - SubService: 确定性多级排序、虚拟信息节点生成、      │
│                系统设置参数注入                        │
└─────────────┬────────────────────────────┬─────────────┘
              │                            │
┌─────────────▼─────────────┐┌─────────────▼─────────────┐
│   Protocol & Sub Layer    ││    Adapter Layer          │
│ - node_converter.go       ││ - XrayManager (gRPC)      │
│ - protocol_adapter.go     ││ - ConfigFileSynchronizer  │
│ - sub/clash.go            ││ - Repository (GORM/SQLite)│
│ - sub/singbox.go          ││   - UserRepository        │
│                           ││   - InboundRepository     │
│                           ││   - SettingRepository     │
└───────────────────────────┘└───────────────────────────┘
```

- **Domain 实体层 (`internal/domain`)**：
  - `User` 扩充 `Remark` 字段；`UpdateUserDTO` 扩充 `Email`, `UUID`, `SubToken`, `Remark`；
  - `Inbound` 扩充 `SortOrder` 字段；`SubRoute` 扩充 `SortOrder` 字段；
  - 领域层维持纯洁性，无外部三方依赖。
- **服务编排层 (`internal/service`)**：
  - `UserService` 作为用户生命周期的唯一指挥官，维护 SQLite、Xray gRPC 内存状态、`config.json` 物理文件以及 `SubRoute.AllowedUsers` 的多态一致性；
  - `SubService` 负责编排订阅分发流水线（排序 -> 命名模板 -> 顶部信息节点 -> 格式序列化）。
- **协议转换与订阅格式化层 (`internal/protocol`, `internal/sub`)**：
  - `protocol` 负责解析单节点传输层、流控及安全参数；
  - `sub` 策略层负责将统一的 `[]*protocol.NodeConfig` 格式化为 Base64、Clash YAML 或 Sing-box JSON。
- **外部适配器层 (`internal/adapter`)**：
  - `xray.VLESSAdapter` 负责编译 Xray 核心客户端配置并对齐用户级 Flow；
  - `repository` 负责 SQLite 持久化存储。

---

## 2. 数据模型变更与迁移策略 (Database Schema Auto-Migration)

### 2.1 结构体字段变更
```go
// internal/domain/user.go
type User struct {
    // ... 原有字段保持不变
    Flow        string    `gorm:"type:varchar(32)" json:"flow"`
    SubToken    string    `gorm:"type:varchar(64);uniqueIndex" json:"subToken"`
    Remark      string    `gorm:"type:varchar(255)" json:"remark"` // 新增：用户备注
    // ...
}

// internal/domain/inbound.go
type Inbound struct {
    // ... 原有字段保持不变
    Remark         string    `gorm:"type:varchar(128)" json:"remark"`
    SortOrder      int       `gorm:"default:0" json:"sortOrder"`      // 新增：显示排序权重（越小越靠前）
    // ...
}

type SubRoute struct {
    ID           string   `json:"id"`
    Name         string   `json:"name"`
    RouteID      uint16   `json:"routeId"`
    OutboundTag  string   `json:"outboundTag"`
    Enabled      bool     `json:"enabled"`
    AllowedUsers []string `json:"allowedUsers,omitempty"`
    SortOrder    int      `json:"sortOrder"`                      // 新增：分流线路排序权重
}
```

### 2.2 GORM 自动迁移机制
- 在 `internal/adapter/repository/sqlite_db.go:42-51` 中，系统启动时执行：
  ```go
  err = db.AutoMigrate(
      &domain.User{},
      &domain.Inbound{},
      // ...
  )
  ```
- **SQLite 列兼容行为**：
  - GORM 的 `AutoMigrate` 会自动检测并向 `users` 表安全添加 `remark TEXT/VARCHAR(255)` 列，向 `inbounds` 表安全添加 `sort_order INTEGER DEFAULT 0` 列；
  - `SubRoute` 作为 JSON 字符串持久化存储于 `inbounds.sub_routes_json` 字段中，无 DDL 变更需求，既有记录在反序列化时 `SortOrder` 默认取 Go 零值 `0`，完美向后兼容。
- **零破坏性升级保证**：
  - 存量数据全量保留，无需手动执行 SQL 迁移脚本；
  - 回滚时即便回退到旧版本二进制，旧代码仅会忽略未声明的新列，不会导致数据库读取报错。

---

## 3. 用户身份变更的 Saga / gRPC 同步机制 (Identity Mutation & Saga Flow)

### 3.1 核心问题剖析
在 Xray-core 原生 gRPC `app/proxyman/command` 中：
- `RemoveUser(inboundTag, email)`：以 `email` 标识用户凭据并注销；
- `AddUser(inboundTag, user)`：以 `user.Email` 作为核心标识。
若用户修改了 `Email` 或 `UUID`：
1. 若直接用 `updatedUser.Email` 调用 `RemoveUser`，由于该邮箱在 Xray 内存中根本不存在，Xray 会返回 `handler not found` 或忽略，而原 `oldUser.Email` 仍在 Xray 内存中继续霸占端口；
2. 若 `Email` 发生变更，`SubRoute.AllowedUsers` 中登记的旧邮箱将变成死引用，导致用户瞬间丢失所有已授权的分流线路权限；
3. 若同步期间任意环节失败，必须能可靠地将数据库、Xray 内存与 SubRoute 引用全部回滚至修改前快照。

### 3.2 完备同步时序图

```
Client             UserService               userRepo            inboundRepo             xrayManager         configSvc
  │                     │                       │                     │                       │                  │
  │── UpdateUser(dto) ─►│                       │                     │                       │                  │
  │                     │── 1. GetByID & 快照   │                     │                       │                  │
  │                     │── 2. 唯一性/格式校验  │                     │                       │                  │
  │                     │── 3. userRepo.Update ─┼────────────────────►│                       │                  │
  │                     │   (更新 DB 记录)      │                     │                       │                  │
  │                     │                       │                     │                       │                  │
  │                     │── 4. 若 Email 变更:   │                     │                       │                  │
  │                     │      级联更新所有     │                     │                       │                  │
  │                     │      SubRoute 引用 ───┼────────────────────►│                       │                  │
  │                     │                       │                     │                       │                  │
  │                     │── 5. Xray 内存热重载: │                     │                       │                  │
  │                     │      从 oldTags 移除旧身份 ────────────────────────────────────────►│                  │
  │                     │      (RemoveUser using oldSnapshot.Email)                           │                  │
  │                     │      向 newTags 注入新身份 (若激活) ───────────────────────────────►│                  │
  │                     │      (AddUser using updatedUser)                                    │                  │
  │                     │                       │                     │                       │                  │
  │                     │── 6. 持久化落盘:      │                     │                       │                  │
  │                     │      SyncUserToFile ──┼─────────────────────┼───────────────────────┼─────────────────►│
  │                     │                       │                     │                       │                  │
  │◄── Return User ─────│                       │                     │                       │                  │
  │                     │                       │                     │                       │                  │
  │                     │── [若 4/5/6 失败] 触发 Saga 补偿 ───────────────────────────────────►│ (逆序恢复)        │
```

### 3.3 补偿逻辑 (Saga Compensation)
```go
rollback := func() {
    compCtx, cancel := createCompensatingContext(ctx, 15*time.Second)
    defer cancel()
    
    // 1. 从所有新标签中清理新用户凭证
    if s.xrayManager != nil {
        for _, t := range newTags {
            _ = s.xrayManager.RemoveUser(compCtx, t, updatedUser.Email)
        }
        // 2. 将旧快照凭证重新灌入原节点
        if oldSnapshot.IsActive() {
            for _, t := range oldTags {
                _ = s.xrayManager.RemoveUser(compCtx, t, oldSnapshot.Email)
                _ = s.xrayManager.AddUser(compCtx, t, &oldSnapshot)
            }
        }
    }
    
    // 3. 恢复入站配置中 SubRoute.AllowedUsers 的邮箱引用
    if emailChanged && s.inboundRepo != nil {
        s.revertSubRouteAllowedUsers(compCtx, updatedUser.Email, oldSnapshot.Email)
    }
    
    // 4. 将数据库记录恢复为旧快照
    _ = s.userRepo.Update(compCtx, &oldSnapshot)
}
```

---

## 4. 协议流控（Flow）多层解析契约 (Protocol Flow Hierarchy)

### 4.1 流控定义与优先级
用户维度的 `Flow` 允许对节点级流控进行覆盖控制：
1. **强制禁用 (`Flow == "none"`)**：
   - 场景：用户使用不支持 XTLS Vision 的旧客户端或特定移动端代理应用，需要连接到 REALITY 节点；
   - 行为：即使节点配置了 `xtls-rprx-vision`，也强行对该用户剔除 `flow` 参数。
2. **强制开启 (`Flow == "xtls-rprx-vision"`)**：
   - 场景：在兼容的 VLESS 节点上强制指定流控；
   - 行为：若底层传输层支持（TCP + REALITY 或 TLS），则应用 `xtls-rprx-vision`。
3. **跟随默认 (`Flow == ""`)**：
   - 场景：继承入站节点自身或全局流控配置（默认在 TCP+REALITY/TLS 下启用 vision）。

### 4.2 传输层物理防线
在 `internal/protocol/node_converter.go` 与 `internal/adapter/xray/protocol_adapter.go` 中设立严格守卫：
```go
// 严格物理约束：XTLS Vision 仅限 VLESS + TCP 传输 + (REALITY 或 TLS)。
// 对 xhttp, splithttp, ws, grpc, mkcp 等传输，或者非加密传输，绝对严禁附加 flow 参数。
func resolveEffectiveFlow(protocol, network, security, userFlow, inboundFlow string) string {
    if !strings.EqualFold(protocol, "vless") {
        return ""
    }
    isTcp := network == "" || strings.EqualFold(network, "tcp")
    isSecure := strings.EqualFold(security, "reality") || strings.EqualFold(security, "tls")
    if !isTcp || !isSecure {
        return ""
    }

    if strings.EqualFold(userFlow, "none") {
        return ""
    }
    if userFlow != "" {
        return userFlow
    }
    return inboundFlow
}
```

---

## 5. 订阅节点流水线设计 (Subscription Pipeline)

当客户端请求 `/sub/:token` 时，`SubService.resolveSubscriptionNodes` 执行如下流水线：

```
[全部入站 Inbounds] 
        │
        ▼ 过滤
[1. 过滤：已启用 且 用户具有该 Inbound 权限 且 (可选) 符合 tagFilter]
        │
        ▼ 确定性排序
[2. 多级排序：Inbound.SortOrder ASC ➔ Inbound.ID ASC]
        │
        ▼ 展开线路
[3. 对各 Inbound 遍历 SubRoutes（限制 VLESS 协议）：]
    - 过滤未启用或无权访问的 SubRoute
    - 按 SubRoute.SortOrder ASC 排序
    - 组装单节点或多线路节点
        │
        ▼ 命名模板渲染
[4. 节点命名模板应用：替换 {remark}, {tag}, {protocol}, {port}, {index}]
        │
        ▼ 顶部信息节点注入
[5. 若 sub_show_info_nodes == true：最前置注入虚拟信息节点（流量/到期/公告）]
        │
        ▼ 格式导出
[6. 目标格式导出器分发：Base64 编码 / Clash YAML / Sing-box JSON]
```

### 5.1 顶部虚拟信息节点结构 (Top Info Virtual Nodes)
- 协议类型：`trojan`
- 节点主机名与端口：`127.0.0.1:10000`
- 密码 / 凭证：`00000000-0000-0000-0000-000000000000`
- 节点命名规范：
  - 流量：`📊 剩余流量: 45.20 GB / 100.00 GB`（若无限制：`📊 已用流量: 12.30 GB (不限配额)`）
  - 到期：`⏳ 到期时间: 2026-12-31`（若永不过期：`⏳ 到期时间: 长期有效`）
  - 公告：`📢 公告: <sub_custom_announcement>`（仅在管理员配置了公告内容时展示）
- 兼容性：所有现代客户端（Clash, Shadowrocket, Sing-box, v2rayN）将此节点当做普通节点渲染在顶部列表，供用户直接查阅自身配额与公告。在自动测速分组（url-test）中，由于该节点指向 127.0.0.1，会自动测速失败并静默下沉，不干扰正常代理使用。

---

## 6. 客户端参数注入与 HTTP 响应契约 (Export Parameters Injection)

### 6.1 系统配置字典 (SettingRepository Keys)
| Key | 类型 | 默认值 | 作用说明 |
| :--- | :--- | :--- | :--- |
| `sub_show_info_nodes` | boolean | `true` | 是否在聚合订阅顶部生成状态与公告提示节点 |
| `sub_custom_announcement` | string | `""` | 订阅顶部自定义公告文本（留空不生成公告节点） |
| `sub_node_name_template` | string | `"{remark}"` | 节点显示命名模板，支持占位符渲染 |
| `sub_profile_title` | string | `"Xray Panel"` | 订阅集合名称（注入 Profile-Title） |
| `sub_update_interval` | int (hours) | `24` | 客户端自动拉取更新间隔时间（小时） |
| `sub_clash_mixed_port` | int | `7890` | Clash 导出的本地 mixed-port 端口 |
| `sub_clash_allow_lan` | boolean | `false` | Clash 导出的 allow-lan 选项 |
| `sub_clash_mode` | string | `"rule"` | Clash 默认模式 (`rule`, `global`, `direct`) |
| `sub_default_fingerprint`| string | `"chrome"` | 默认客户端 uTLS 伪装指纹 |

### 6.2 HTTP 交付契约 (`handler_sub.go`)
- **Headers 注入**：
  ```http
  HTTP/1.1 200 OK
  Content-Type: application/yaml; charset=utf-8 (或 text/plain, application/json)
  Subscription-Userinfo: upload=1048576; download=10485760; total=107374182400; expire=1798761600
  Profile-Title: base64:XXXX 或 utf-8 string
  Profile-Update-Interval: 24
  Cache-Control: no-store, no-cache, must-revalidate, max-age=0
  ```

---

## 7. 重要权衡与回滚策略 (Trade-offs & Operational Considerations)

1. **Xray 原生 RemoveUser 契约制约**：
   - Xray gRPC 的 `RemoveUser` 只能按 `email` 索引定位。因此，`UserService.UpdateUser` 中绝不可提前将快照丢弃，必须保留 `oldSnapshot.Email` 直至 Xray 核心成功将其摘除。
2. **虚拟节点的协议选择**：
   - 之所以采用标准 `trojan` 伪装节点而非特定客户端专属扩展（如 `#upload` 注释），是因为标准 Trojan 链接 `trojan://dummy@127.0.0.1:10000#...` 能够 100% 跨客户端兼容，Base64/Raw 链接、Clash、Sing-box 均可原样下发，不产生解析异常。
3. **回滚与故障防御**：
   - 数据库 Schema 变更完全向前兼容，无外键强制约束；
   - 在 Saga 补偿阶段，所有反向操作均包裹在带超时且脱离原上下文的 `createCompensatingContext` 中，即便原 HTTP 请求超时取消，补偿事务依然会保证执行完毕，杜绝中间态。
