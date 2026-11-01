# 修复前后端配置失配与编译器数据丢失 - 技术设计 (Design)

## 1. 架构拓扑与分层边界

本项目遵循 Clean Architecture 与单一事实源原则。本次重构严格在已有分层契约内进行扩展，严禁跨层泄漏或引入多余抽象：

```
┌─────────────────────────────────────────────────────────────┐
│ Web Presentation Layer (Vue 3 + Tailwind CSS + TypeScript)   │
│ - views/inbounds: 补齐 remark/routeId/sniffing/transports  │
│ - views/outbounds: 补齐 tlsFingerprint/spiderX/wgMtu/trans  │
│ - views/RoutingView.vue: 补齐 rule.tag 与 rule.user 匹配     │
│ - views/SettingsView.vue: xray_geodata_dir 已有UI对齐后端   │
└──────────────────────────────┬──────────────────────────────┘
                               │ HTTP RESTful API (JSON Payload)
┌──────────────────────────────▼──────────────────────────────┐
│ Delivery / Service Layer (internal/delivery & service)      │
│ - ConfigService: DNS/入站/出站/路由配置获取与保存编排        │
│ - SubService: 利用 inbound.Remark 构建准确订阅节点名称       │
│ - GeoDataService: 注入 SettingRepository 读取配置目录       │
└──────────────────────────────┬──────────────────────────────┘
                               │ Domain & Protocol Conversion
┌──────────────────────────────▼──────────────────────────────┐
│ Protocol & Domain Layer (internal/domain & protocol)        │
│ - domain.DNSConfig: ClientIP/DisableCache/Fallback/Hosts   │
│ - domain.Inbound / Outbound / RoutingRule                   │
│ - protocol.InboundToNodeConfig: 提取 httpupgrade / mkcp     │
└──────────────────────────────┬──────────────────────────────┘
                               │ Adapter Mapping & Compilation
┌──────────────────────────────▼──────────────────────────────┐
│ Infrastructure / Xray Adapter (internal/adapter/xray)       │
│ - schema.go: XrayDNSConfig 补齐, StreamSettings 增配 kcp/up │
│ - compiler.go: compileDNS 完整映射, Inbound/Outbound 编译   │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. 详细数据流与结构变动契约

### 2.1 入站流量嗅探高级参数完备化 (R1)
- **数据结构扩展 (`web/src/views/inbounds/types.ts`)**：
  ```typescript
  export interface InboundFormData {
    // ...
    sniffingEnabled: boolean
    sniffingRouteOnly: boolean
    sniffingDestOverride: string[] // ['http', 'tls', 'quic', 'fakedns']
  }
  ```
- **序列化与反序列化**：
  ```typescript
  export function buildSniffingJSON(clean: InboundFormData): string {
    const destOverride = (clean.sniffingDestOverride && clean.sniffingDestOverride.length > 0)
      ? clean.sniffingDestOverride
      : ['http', 'tls', 'quic']
    return JSON.stringify({
      enabled: clean.sniffingEnabled,
      destOverride,
      routeOnly: clean.sniffingRouteOnly,
    }, null, 2)
  }
  ```
  在 `InboundFormDrawer.vue:initFormData` 中：
  ```typescript
  let sniff: any = {}
  try { sniff = JSON.parse(inb.sniffingJson || '{}') } catch (e) {}
  form.value.sniffingEnabled = sniff.enabled !== false
  form.value.sniffingRouteOnly = sniff.routeOnly === true
  form.value.sniffingDestOverride = Array.isArray(sniff.destOverride) && sniff.destOverride.length > 0
    ? sniff.destOverride
    : ['http', 'tls', 'quic']
  ```
- **UI 模板增强 (`InboundFormDrawer.vue:323-336`)**：
  在 `sniffingEnabled` 勾选时展开两项子控件：
  1. `form.sniffingRouteOnly` 切换开关（“仅限路由 (RouteOnly)”）；
  2. `form.sniffingDestOverride` 复选框组（提供 `http`、`tls`、`quic`、`fakedns`）。

---

### 2.2 DNS 编译器数据往返保真 (R2)
- **Xray DNS Schema 扩展 (`internal/adapter/xray/schema.go:173-177`)**：
  ```go
  type XrayDNSConfig struct {
      Tag             string                 `json:"tag,omitempty"`
      ClientIP        string                 `json:"clientIp,omitempty"`
      QueryStrategy   string                 `json:"queryStrategy,omitempty"`
      DisableCache    bool                   `json:"disableCache,omitempty"`
      DisableFallback bool                   `json:"disableFallback,omitempty"`
      Servers         []interface{}          `json:"servers,omitempty"`
      Hosts           map[string]interface{} `json:"hosts,omitempty"`
  }
  ```
- **编译器映射实现 (`internal/adapter/xray/compiler.go:112-127`)**：
  ```go
  if dns != nil {
      servers := dns.Servers
      if len(servers) == 0 {
          servers = []interface{}{"https://1.1.1.1/dns-query", "8.8.8.8", "localhost"}
      }
      qStrategy := dns.QueryStrategy
      if qStrategy == "" {
          qStrategy = "UseIP"
      }
      tag := dns.Tag
      if tag == "" {
          tag = "dns-in"
      }
      cfg.DNS = &XrayDNSConfig{
          Tag:             tag,
          ClientIP:        dns.ClientIP,
          QueryStrategy:   qStrategy,
          DisableCache:    dns.DisableCache,
          DisableFallback: dns.DisableFallback,
          Servers:         servers,
          Hosts:           dns.Hosts,
      }
  }
  ```
- **保真验证**：
  `ConfigService.GetDNSConfig` 与 `SaveDNSConfig` 之间不再丢失 `ClientIP`、`DisableCache`、`DisableFallback`、`Hosts` 与自定义 `Tag`。

---

### 2.3 入站配置项补齐与传输层协议支持 (R3)
- **节点备注与顶层路由编号**：
  - `web/src/views/inbounds/types.ts` 中 `InboundFormData` 与 `InboundPayload` 增加 `remark: string`；
  - `InboundFormDrawer.vue` 模板增加 `form.remark`（节点备注）与顶层 `form.routeId`（默认路由编号，0-65535）输入框；
  - `sub_service.go` 保持对 `inbound.Remark` 的首选使用（已有代码无需变更）。
- **传输层 Schema 扩充 (`internal/adapter/xray/schema.go:75-86`)**：
  ```go
  type XrayStreamSettings struct {
      Network             string                   `json:"network,omitempty"`
      Security            string                   `json:"security,omitempty"`
      RealitySettings     *XrayRealitySettings     `json:"realitySettings,omitempty"`
      TLSSettings         *XrayTLSSettings         `json:"tlsSettings,omitempty"`
      XHTTPSettings       *XrayXHTTPSettings       `json:"xhttpSettings,omitempty"`
      WSSettings          *XrayWSSettings          `json:"wsSettings,omitempty"`
      GRPCSettings        *XrayGRPCSettings        `json:"grpcSettings,omitempty"`
      HTTPUpgradeSettings *XrayHTTPUpgradeSettings `json:"httpupgradeSettings,omitempty"`
      KCPSettings         *XrayKCPSettings         `json:"kcpSettings,omitempty"`
      Sockopt             *XraySockopt             `json:"sockopt,omitempty"`
  }

  type XrayHTTPUpgradeSettings struct {
      Path    string            `json:"path,omitempty"`
      Host    string            `json:"host,omitempty"`
      Headers map[string]string `json:"headers,omitempty"`
  }

  type XrayKCPSettings struct {
      Mtu              int            `json:"mtu,omitempty"`
      Tti              int            `json:"tti,omitempty"`
      UplinkCapacity   int            `json:"uplinkCapacity,omitempty"`
      DownlinkCapacity int            `json:"downlinkCapacity,omitempty"`
      Congestion       bool           `json:"congestion,omitempty"`
      ReadBufferSize   int            `json:"readBufferSize,omitempty"`
      WriteBufferSize  int            `json:"writeBufferSize,omitempty"`
      Header           *XrayKCPHeader `json:"header,omitempty"`
      Seed             string         `json:"seed,omitempty"`
  }

  type XrayKCPHeader struct {
      Type string `json:"type,omitempty"` // none, srtp, utp, wechat-video, dtls, wireguard
  }
  ```
- **订阅节点转换器解析 (`internal/protocol/node_converter.go`)**：
  - 在 `inboundRawStream` 结构中增加 `HTTPUpgradeSettings` 与 `KCPSettings`；
  - 在 `switch network` 中增加对 `httpupgrade`（解析 `path` 与 `host`）和 `kcp`/`mkcp`（解析 `headerType` 与 `seed`）的支持，并写入 `NodeConfig`。
- **TLS ALPN 与 Fallback ALPN/Path**：
  - `InboundFormData` 增加 `tlsAlpn: string`（逗号隔开），序列化时切片为 `stream.tlsSettings.alpn`；
  - `InboundFormData` 增加 `fallbackAlpn: string` 与 `fallbackPath: string`，序列化时写入 `settings.fallbacks[0]`。

---

### 2.4 出站高级配置项补齐 (R4)
- **`web/src/views/outbounds/types.ts` 扩展**：
  ```typescript
  export interface OutboundFormState {
    // ...
    tlsFingerprint: string // 'chrome' | 'firefox' | 'safari' | 'ios' | 'edge' | 'random' | 'randomized'
    realitySpiderX: string
    wgMtu: number          // 默认 1280
    wgNoKernelTun: boolean // 默认 true
    httpupgradePath: string
    httpupgradeHost: string
  }
  ```
- **`OutboundFormDrawer.vue` 模板交互**：
  - 在 TLS 参数卡片中加入指纹 `<select>`，选项涵盖 `chrome`, `firefox`, `safari`, `ios`, `edge`, `random`, `randomized`；
  - 在 Reality 参数卡片中加入 SpiderX 输入框；
  - 在 WireGuard 参数卡片中加入 MTU 输入框与 NoKernelTun 开关；
  - 当协议为 `httpupgrade` 时展示 Path 与 Host 输入框。

---

### 2.5 路由规则完备化 (User & Tag) (R5)
- **`web/src/views/RoutingView.vue` 表单与状态设计**：
  - `ruleForm` 状态增加：
    ```typescript
    tag: '',
    userStr: '',
    ```
  - `openAddRuleDrawer`：重置 `tag: ''`, `userStr: ''`；
  - `editRule`：回显 `tag: r.tag || ''`, `userStr: (r.user || []).join(', ')`；
  - `saveRuleInDrawer`：
    ```typescript
    if (ruleForm.value.tag && ruleForm.value.tag.trim()) {
      newRule.tag = ruleForm.value.tag.trim()
    }
    const users = parseArray(ruleForm.value.userStr)
    if (users) {
      newRule.user = users
    }
    ```
- **规则列表与移动端卡片展示**：
  - 规则标识：在位次或出站旁渲染 `rule.tag` 徽章；
  - 规则条件：在匹配条件区域加入 `rule.user` 条件徽章（`用户: user1@xxx, ...`）。

---

### 2.6 GeoData 规则库存储目录配置实装 (R6)
- **服务接口与依赖注入 (`internal/service/geodata_service.go`)**：
  ```go
  type GeoDataService struct {
      xrayBinPath string
      manager     CoreController
      settingRepo domain.SettingRepository
      progressMu  sync.RWMutex
      progress    GeoDataProgress
  }

  func NewGeoDataService(xrayBinPath string, manager CoreController, settingRepo domain.SettingRepository) *GeoDataService {
      return &GeoDataService{
          xrayBinPath: xrayBinPath,
          manager:     manager,
          settingRepo: settingRepo,
          progress: GeoDataProgress{
              Step:       "idle",
              Percentage: 0,
              Message:    "空闲",
          },
      }
  }
  ```
- **目录获取逻辑**：
  ```go
  func (s *GeoDataService) getGeoDataDir(ctx context.Context) string {
      if s.settingRepo != nil {
          if customDir, err := s.settingRepo.Get(ctx, "xray_geodata_dir"); err == nil {
              customDir = strings.TrimSpace(customDir)
              if customDir != "" {
                  return customDir
              }
          }
      }
      dir := filepath.Dir(s.xrayBinPath)
      if runtime.GOOS == "linux" {
          if _, err := os.Stat("/usr/local/share/xray"); err == nil {
              return "/usr/local/share/xray"
          }
      }
      return dir
  }
  ```
  `GetStatus(ctx context.Context)` 与 `UpdateGeoData(ctx context.Context, target string)` 均传入 `ctx` 调用 `s.getGeoDataDir(ctx)`。
- **装配入口 (`main.go:153`)**：
  ```go
  geoSvc := service.NewGeoDataService(cfg.XrayBinPath, xrayManager, settingRepo)
  ```

---

## 3. 并发安全与性能保障
- **零锁开销配置编译**：DNS 配置、Inbound/Outbound StreamSettings 转换均为纯函数或局部变量反序列化，无全局共享可变状态。
- **仓储只读并发安全**：`settingRepo.Get` 线程安全，通过 `ctx` 支持超时取消；若配置不存在或查询出错立即降级回退至系统默认路径，不阻塞主流程。
- **零脏数据入库**：前端提交前经由 `sanitizeInboundPayload` 与 `sanitizeOutboundPayload` 进行协议白名单字段裁剪，未勾选的协议配置自动置空，杜绝脏字段混入底层 JSON。

---

## 4. 兼容性与回滚策略 (Compatibility & Rollback)
- **向后兼容性**：
  - 所有新增 JSON 字段均为 `omitempty` 可选属性；
  - 旧有配置未包含 `destOverride` 时，默认按标准 `['http', 'tls', 'quic']` 执行，不改变现有行为；
  - 未配置 `xray_geodata_dir` 时，平滑降级至 `/usr/local/share/xray` 或可执行文件目录，保持存量系统平滑运行。
- **无破坏性 Schema 变更**：
  - Inbound 表已存在 `remark` 字段（`internal/domain/inbound.go:17`）；
  - 无需运行数据库 Migration 或执行 DDL 迁移。
- **回滚方案**：
  - 本次变更属于无副作用的代码级修复；回滚时直接重置 Git 提交即可，已存储的新增 JSON 字段（如 `hosts`, `destOverride`）在旧版本代码中会被反序列化自然忽略，不会导致崩溃。
