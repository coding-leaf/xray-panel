# Quality & Concurrency Guidelines

> 并发安全、KISS 极简规范与反过度设计红线

---

## 1. 并发与资源保护 (Concurrency Safety)

- **竞态防御**：所有被多个 goroutine 共享的内存状态（如缓存映射、流速计算器、连接池）必须有 `sync.RWMutex` 或 `sync.Mutex` 保护；
- **竞态检查**：修改并发逻辑后必须通过 `CGO_ENABLED=1 mise x -- go test -race ./...` 检验；
- **资源泄漏防御**：
  - 启动的 goroutine 必须有生命周期终止通知（通过 `ctx.Done()` 或 channel）；
  - HTTP Request Body 必须 `defer resp.Body.Close()`；
  - 定时器 `time.NewTicker` 必须 `defer ticker.Stop()`。

---

## 2. KISS 极简原则 (Keep It Simple, Stupid)

- **严禁无痛点抽象**：
  - 禁止为单一实现的 struct 凭空包装一层 interface；
  - 禁止创建没有业务痛点的泛型抽象与过度工厂模式；
  - 优先使用简单、直接、可读性高的标准库模式。
- **杜绝空转包装**：
  - 不得在两层之间引入仅作原样透传的冗余中继结构。

---

## 3. 安全防线 (Security Invariants)

- **凭证零泄漏**：严禁在代码、测试、配置或日志中出现真实公网 IP、生产域名、JWT 密钥、Telegram Token；
- **测试环境安全**：单元测试统一使用模拟数据与本地环回地址（`127.0.0.1`）。

---

## 4. 协议契约与配置真实性准则 (Protocol Invariants & Configuration Truth)

- **杜绝猜测与假兜底**：
  - 严禁在 REALITY 等安全协议中静默填补假域名占位符（如 `www.example.com`）；
  - 严禁在 `dest ↔ serverNames` 间自动跨字段猜测互推；
  - 缺少必填参数（如 `dest`、`serverNames`、`privateKey`）时必须显式返回校验错误（`domain.ErrInvalidInput`），严禁生成语义错误的“合法”假配置。
- **协议密码学套件白名单收敛**：
  - Shadowsocks 2022 多用户（sub-keys/clients 绑定）模式下，根据规范仅允许 `2022-blake3-aes-128-gcm` 与 `2022-blake3-aes-256-gcm`，禁止配置 `2022-blake3-chacha20-poly1305`。
- **废弃字段防线**：
  - 严禁向 Xray-core 生成已废弃的配置项（如 `tlsSettings.allowInsecure`），避免核心启动时触发硬校验报错。
- **策略定性边界**：
  - 明确区分“面板当前管理策略暂未开放/暂仅支持原生网络监听”与“Xray-core 原生不支持”，严禁在提示和文档中混淆归因。

---

## 5. 流量口径与统计真实性准则 (Traffic Accounting Truth)

- **入站统计口径唯一性**：
  - `/dashboard` 中的 `totalUp` 与 `totalDown` 严格定义为所有现存业务入站（含禁用入站，排除内部管理 `api` 入站）的历史累计总和；
  - 严禁将用户累计与入站累计相加或在入站为空时回退至用户累计；入站查询失败时必须显式返回错误，禁止返回伪造的零值；
  - 主机网卡物理流量（`netTotalSent`/`netTotalRecv`）与 Xray 入站代理流量严格分列呈现，严禁在逻辑或文案中模糊或混用两者。

---

## 6. Xray 访问与错误日志解析契约 (Log Parsing Contract & Performance)

- **核心兼容契约**：
  - 访问日志流式过滤必须同时支持 `->`（明确命中分流规则）、`>>`（规则未命中触发的默认出口 Fallback 兜底）以及单 tag `[inbound]`；
  - 必须兼容识别客户端传输层前缀格式（包括原生 IP、`from tcp:<ip>:<port>` / `from udp:<ip>:<port>` 以及 IPv6 `[::1]:port`）；
  - 必须兼容带毫秒及不带毫秒的时间戳（如 `2026/09/12 10:00:01.123` 与 `2026/09/12 10:00:01`）；
  - 错误日志必须兼容级别归一化（`WARNING` -> `WARN`）、可选 PID（如 `[12345]`）、模块冒号提取与 SmartTip 诊断规则。
- **高性能与零分配标准**：
  - 严禁在高频读取/过滤循环中使用重量级 `regexp` 正则引擎；
  - 统一采用确定性游标状态机/扫描器（Tokenizer/Scanner），`ParseLogLineFast` 必须直接在 `[]byte` 切片上定位元数据，避免字符串拷贝与堆分配开销。

---

## 7. 路由编译器与策略放权准则 (Routing Compiler & Policy Delegation)

- **机制极简中立**：
  - 后端 `XrayCompiler.Compile` 的 Layer 1 仅保留进程内核心 gRPC 管理规则（`inboundTag: ["api"] -> outboundTag: "api"`）；
  - 严禁在编译器中硬编码任何业务阻断（如 25 端口、BT 协议、私有 IP 或特定域名分类）；
- **策略完全放权**：
  - 通用安全防护与分流需求（广告、局域网私有 IP、BT、邮件 25 端口、国内分流等）一律作为前端纯客户端一键向导，由管理员按需点击生成并自由编排；
  - 系统初始状态下保持规则池纯净，严禁隐式自动塞入阻断或分流规则；
- **编译器忠实性 (Faithful Compilation)**：
  - 严禁静默过滤或丢弃用户自定义规则中的合法字段（包括 `vlessRoute` 等）；
  - 严格遵循用户在界面排定的顺序（First-Match 原则）输出 Layer 2 自定义规则池，确保跨机专属通道（`vlessRoute`）等关键策略不被无作用域的大网规则截流；
- **单一事实源与消灭隐式追加 (Single Source of Truth)**：
  - 彻底废除编译器底层遍历入站分流线路隐式追加 Layer 3 规则的逻辑，消灭“系统托管 vs 自定义”阴阳两套规则；
  - 统一路由规则池为唯一事实源（所见即所得），新入站线路显式作为普通规则默认追加至尾部，严禁产生读出 ➔ 编译 ➔ 翻倍的回环污染；
  - **SubRoute 规则全生命周期对账 (`ensureSubRouteRules`)**：每次对账不仅要补齐新增的 `vlessRoute` 兜底规则，还必须就地同步已修改的 `OutboundTag` / `AllowedUsers`，并自动清理已在入站中删除、停用或改号的残留僵尸 `vlessRoute` 纯兜底规则（通过 `isPureSubRouteFallbackRule` 精确区分纯兜底规则与用户自定义带域名/IP 条件的路由规则）。

---

## 8. VLESS Flow 优先级、逐客户端编译与启动反向同步无损契约 (VLESS Flow & Startup Sync Invariants)

### 1. Scope / Trigger
- **Trigger**: 任何修改 `VLESSAdapter`（编译/gRPC 热更新）、`StreamAccessor`、`protocol.InboundToNodeConfigWithDefaultFP`（订阅/分享链接导出）、`ConfigService.syncFromRawJSONLocked`（启动 `SyncFromFile` / 原生配置保存）或前端入站/用户流控表单回显的路径。

### 2. Signatures
- `(*StreamAccessor).SupportsVisionFlow(protocol string) bool` (`internal/adapter/xray/stream_accessor.go`)
- `(*StreamAccessor).ResolveVisionFlow(protocol string) string` (`internal/adapter/xray/stream_accessor.go`)
- `resolveVLESSClientFlow(inbound *domain.Inbound, user domain.User, accessor *StreamAccessor) string` (`internal/adapter/xray/protocol_adapter.go`)
- `InboundToNodeConfigWithDefaultFP(ib *domain.Inbound, user *domain.User, host string, defaultFP string) (*NodeConfig, error)` (`internal/protocol/node_converter.go`)
- `(*ConfigService).syncFromRawJSONLocked(ctx context.Context, rawJSON []byte) error` (`internal/service/config_service.go`)

### 3. Contracts
- **逐客户端编译铁律（严禁输出顶层 `inbounds[].settings.flow`）**：
  - Xray-core `infra/conf/vless.go` (`VLessInboundConfig.Build()`) 会将顶层 `inbounds[].settings.flow` 自动回填给所有省略了 `flow` (`""`) 的 `clients[]`；因此 `VLESSAdapter.DecorateSettings` **必须无条件 `delete(settingsMap, "flow")`**，仅在 `inbounds[].settings.clients[].flow` 中逐用户输出流控，而 SQLite `inbound.SettingsJSON` 继续保留节点默认策略。
- **全链路统一优先级（编译器 = gRPC 热更新 = 订阅导出 = 前端说明）**：
  1. **传输层能力门禁 (`SupportsVisionFlow`)**：仅当 `protocol == "vless" && network == "tcp" && (security == "tls" || security == "reality")` 时允许非空 Vision 流控，否则强制为 `""`；
  2. **用户显式策略优先 (`user.Flow`)**：`"none"` -> `""`；非空（如 `"xtls-rprx-vision"` / `"xtls-rprx-vision-udp443"`） -> 原样采用（即使用户所在入站默认设为 `"none"` 也允许单用户显式开启）；
  3. **入站默认策略兜底 (`ResolveVisionFlow`)**：仅当 `user.Flow == ""`（跟随节点默认）时生效：入站 `"none"` -> `""`，入站显式值 -> 显式值，入站未配置/空值 -> 默认 `"xtls-rprx-vision"`。
- **启动反向同步 (`syncFromRawJSONLocked`) 无损保留契约**：
  - 当 `rawFlow == ""` 且 `existing != nil`（已存在于 SQLite 的入站）时，必须原样保留 `existing.SettingsJSON` 中的 `flow` 键值，**严禁根据 `!hasClientWithFlow` 或 `hasClientWithFlow` 反向推导篡改入站默认策略**；仅当 `existing == nil`（首次从外部原生配置导入未知入站）且 `hasAnyClient && !hasClientWithFlow` 时才落盘 `"flow": "none"`；当 `rawFlow != ""`（原生 JSON 编辑器显式指定）时采用 `rawFlow`；
  - 当 `existing != nil` 时，合并保留编译期被刻意剥离的面板侧客户端字段（`tlsSettings.fingerprint`，以及 `privateKey` 未轮换时的 `realitySettings.{publicKey, fingerprint, spiderX}`），严禁直接用编译态 `streamSettings` 抹除数据库客户端元数据。

### 4. Validation & Error Matrix
- 非 `TCP + (TLS|REALITY)` 传输层配置任意入站或用户 Vision `flow` -> 编译结果、gRPC 账户与订阅导出均静默收敛为 `""`（前端 `getNodeFlow` 回显 `'none'`）；
- 同一 VLESS+TCP+REALITY 入站混部 `userA.Flow="none"` 与 `userB.Flow="xtls-rprx-vision"` -> 编译产物无顶层 `settings.flow`，`userA` 省略 `flow`，`userB` 输出 `"xtls-rprx-vision"`，Xray 重载后互不污染。

### 5. Good / Base / Bad Cases
- **Good**: 入站默认 `xtls-rprx-vision`，中转用户设 `flow="none"`、直连用户设 `flow=""`，反复执行 `SyncFromFile -> RecompileAndApply` 重启后，中转用户始终无流控，且数据库 `realitySettings.publicKey` 完整保留。
- **Base**: 入站默认 `flow="none"`，普通用户 `flow=""` 保持无流控，特例用户显式设 `flow="xtls-rprx-vision"` 在编译、gRPC 与订阅链接中均正常获得 Vision。
- **Bad**: 在编译产物保留顶层 `inbounds[].settings.flow`；或在 `existing != nil` 时用 `!hasClientWithFlow` 反推覆盖数据库 `settings.flow`。

### 6. Tests Required
- `TestXrayCompiler_VLESSFlowOverrideAndPriority` (`internal/adapter/xray/compiler_test.go`)
- `TestBuildShareLink_FullParameterParity` (`internal/protocol/node_converter_test.go`)
- `TestConfigService_SyncFromFile_PreservesMixedUsersAndInboundFlowAndStreamSettings` (`internal/service/config_service_test.go`)

### 7. Wrong vs Correct
#### Wrong
```go
// 错误 1：保留顶层 settings.flow，导致 Xray 启动时将省略 flow 的 none 用户覆盖回 vision
if !strings.EqualFold(flow, "xtls-rprx-vision") { delete(settingsMap, "flow") }
// 错误 2：入站 flow="none" 提前短路，拦截用户显式配置的 xtls-rprx-vision
if inboundFlow := accessor.ResolveVisionFlow(inbound.Protocol); inboundFlow == "" { return "" }
```
#### Correct
```go
// 正确 1：编译产物无条件剥离顶层 settings.flow，仅逐 client 输出
delete(settingsMap, "flow")
// 正确 2：先校验传输能力，再按 User 显式设置 > Inbound 默认兜底判定
if !accessor.SupportsVisionFlow(inbound.Protocol) { return "" }
if strings.EqualFold(uFlow, "none") { return "" }
if uFlow != "" { return uFlow }
return accessor.ResolveVisionFlow(inbound.Protocol)
```

---

## 9. 订阅与分享链接导出契约 (Subscription & Share Link Export Invariants)

- **RFC 3986 URI Fragment 编码规范**：
  - 导出 `vless://`、`trojan://`、`ss://`、`hysteria2://`、`socks://` 分享链接的 `#remark` 锚点后缀时，严禁直接使用 `url.QueryEscape`（会将空格编码为 `+` 号导致 v2rayNG / NekoBox 等客户端节点名显示加号）；
  - 必须统一调用 `protocol.EscapeRemark(remark)`（将 `+` 替换为 `%20`），严格遵循 RFC 3986 百分号编码标准。
- **uTLS 指纹 (`fp`) 双链路无状态一致性**：
  - 聚合订阅（`resolveSubscriptionNodes`）与单节点直连分享链接（`GetUserShareInfo`）必须读取系统设置 `sub_default_fingerprint`，并通过无状态参数函数（`InboundToNodeConfigWithDefaultFP` / `InboundsToNodeConfigsWithDefaultFP` / `BuildShareLinkWithDefaultFP`）显式透传，严禁在并发请求路径中改写包级全局变量 `protocol.SetDefaultFingerprint`；
  - 优先级契约：`入站显式配置的 fingerprint (realitySettings/tlsSettings.fingerprint) > 系统默认 sub_default_fingerprint > 兜底 "chrome"`。
- **非客户端代理协议过滤**：
  - `dokodemo-door`（任意门端口转发）等非客户端代理协议在 `InboundToNodeConfig`、`InboundsToNodeConfigs` 与 `GetUserShareInfo` 中必须自动跳过，严禁导出为无效客户端订阅节点。

---

## 10. Outbound Mux 编解码与 Xray Schema 全链路无损保留契约 (Outbound Mux & Schema Preservation Invariants)

### 1. Scope / Trigger
- **Trigger**: 任何触发 `ConfigService.ListOutbounds`、`SaveOutbound`、`RecompileAndApply`、`SaveConfigQuietly` 或 `XrayCompiler.CompileToJSON` 的配置读写与重编译路径。

### 2. Signatures & Contracts
- `domain.Outbound` (`internal/domain/outbound.go`):
  - `SendThrough string `json:"sendThrough,omitempty"``
  - `TargetStrategy string `json:"targetStrategy,omitempty"``
  - `Mux string `json:"mux,omitempty"``
- `domain.RoutingRule` (`internal/domain/outbound.go`):
  - `Source []string `json:"source,omitempty"``
  - `SourcePort string `json:"sourcePort,omitempty"``
  - `BalancerTag string `json:"balancerTag,omitempty"``
  - `DomainMatcher string `json:"domainMatcher,omitempty"``
- `xray.XrayOutbound` / `XraySockopt` / `XrayStreamSettings` / `XrayRealitySettings` / `XraySniffingConfig` (`internal/adapter/xray/schema.go`):
  - 必须显式声明 `mux`、`sendThrough`、`targetStrategy`、`sockopt.dialerProxy`（**严禁使用已在 Xray 26+ 移除并报 Exit 23 的 `outbound.proxySettings`**）、`sockopt.acceptProxyProtocol`、KeepAlive/TFO/MPTCP、`tcpSettings` (`rawSettings`)、`splithttpSettings`、REALITY `password`/`target`/`mldsa65*`、Sniffing `metadataOnly`/`domainsExcluded`/`ipsExcluded`。

### 3. Validation & Error Matrix
- `ob.Mux` 为非法 JSON 字符串 -> `compileOutbound` 显式返回 `invalid outbound mux json` 错误，严禁静默吞错；
- `ob.Protocol == "vless"` 且 `settings.vnext[*].users[*].flow == "xtls-rprx-vision"` 同时存在 `mux.enabled == true` -> `compileOutbound` 自动抑制输出 `mux`（因 Vision 裸流拼接与 Mux 多路复用在 Xray-core 底层严格互斥）；
- `SaveOutbound` 更新既有同 Tag 出站时：
  - 若调用方未传 `Mux` (`""`)，默认保留既有 `existing.Mux`（防止未感知 `Mux` 的局部更新抹除配置）；
  - 若调用方显式传 `"{}"`、`"null"` 或 `{"enabled":false}`，由 `cleanMux` 归一化清空为 `""`，确保编译产物不残留无效 `mux` 块。

### 4. Good / Base / Bad Cases
- **Good**: VLESS + TCP + REALITY（无 Vision Flow）配置 `{"enabled":true,"concurrency":8,"xudpConcurrency":16,"xudpProxyUDP443":"reject"}`，经 `ListOutbounds -> SaveOutbound -> CompileToJSON` 往返后字节级语义无损，且通过 `xray run -test`。
- **Base**: 前端关闭既有出站的 `muxEnabled`，提交 `mux: "{}"`，后端 `SaveOutbound` 将其清除，重编译后 `mux` 键消失。
- **Bad**: 在 `ListOutbounds` 中使用残缺匿名结构体丢弃 `mux`/`sendThrough`/`targetStrategy`，或在链式代理中生成已废弃的 `outbound.proxySettings`。

### 5. Tests Required
- `TestXrayCompiler_OutboundMuxAndSchemaAlignment` & `TestXrayCompiler_MuxVisionMutualExclusivity` (`internal/adapter/xray/compiler_test.go`)；
- `TestConfigService_OutboundMuxAndSchemaAlignment_RoundTrip` (`internal/service/config_service_test.go`)。

### 6. Wrong vs Correct
#### Wrong
```go
// ListOutbounds 仅解析 4 个字段，导致重编译时 mux / sendThrough 被静默抹除
Outbounds []struct { Tag, Protocol string; Settings, StreamSettings json.RawMessage }
```
#### Correct
```go
// 完整解析 SendThrough / TargetStrategy / Mux，并在 SaveOutbound 中区分省略保留与显式清空 (cleanMux)
Outbounds []struct {
    Tag, Protocol, SendThrough, TargetStrategy string
    Settings, StreamSettings, Mux json.RawMessage
}
```

---

## 11. 配置双源串行化、授信代理限流与 REALITY H2 探测契约 (Concurrency & Edge Hardening Invariants)

### 1. Scope / Trigger
- **Trigger**: 修改 `ConfigService` 配置读写重编译、`UserService` 批量变更、HTTP 真实客户端 IP 提取限流（`middleware/limiter.go`）或 `DefaultRealityProber` 伪装目标探测（`adapter/reality/prober.go`）。

### 2. Signatures & Contracts
- `ConfigService`: 业务层 `Read -> Modify -> Compile -> Write` 全程由 `ConfigService.mu` (`sync.Mutex`) 串行化，内部互调统一走 `*Locked` 私有辅助函数避免自死锁；单次重编译通过 `readParsedSectionsLocked` 一次读取解析 `config.json` 并透传原始字节写快照；批量用户操作（`BatchRenew`/`BatchResetTraffic`/`BatchSetStatus`/`BatchDelete`）在循环内仅同步 DB + gRPC，循环末尾统一调用 **1 次** `SaveConfigQuietly`。
- `middleware.GetRealClientIP(c *gin.Context) string`: 仅当直连 `RemoteAddr` 属于受信代理网段（默认 `127.0.0.1/8`、`::1/128` 及 `SetTrustedProxyCIDRs` 显式网段）时才解析 `CF-Connecting-IP` / `X-Real-IP` / `X-Forwarded-For`；非受信公网直连强制返回 socket 对端 IP；`ipLimiterShard` 单分片容量硬上限为 `4096`。
- `DefaultRealityProber.Probe`: 当 `tlsState.NegotiatedProtocol == "h2"` 时必须通过 `(&http2.Transport{}).NewClientConn(tlsConn)` 发送原生 HTTP/2 请求读取响应头，严禁向 `h2` TLS 流写入 `HTTP/1.1` 明文报文。

### 3. Validation & Error Matrix
- 非受信对端携带伪造 `CF-Connecting-IP` -> 忽略伪造头，按直连 socket IP 计入限流桶；
- `ConfigService` 内部查询 `inboundRepo.ListAll` / `userRepo.ListAll` 报错 -> 显式返回 `%w` 错误中止编译，严禁 `_ =` 吞错后将空列表覆盖写盘；
- `ListInbounds` 遇到纯 UDP 协议（`hysteria2`、纯 UDP `dokodemo-door`） -> 跳过 TCP Dial，复用核心运行状态判定存活。

### 4. Good / Base / Bad Cases
- **Good**: Nginx `stream :443` 面板分支启用 `proxy_protocol` 直达 `:8443` `real_ip_header proxy_protocol`，REALITY 分支独立剥离/直通，面板收到真实访客 IP。
- **Base**: 本机无反代直连测试（`RemoteAddr` 为公网 IP），伪造转发头不生效，单分片超 4096 IP 时自动淘汰过期/最旧桶。
- **Bad**: 在 `GRPCClient.getConn` 解锁后裸读 `c.handlerCli`、在 `Telegram.Start` 直接覆写共享 `bot.Client`、或向 ALPN `h2` 连接发送 `GET / HTTP/1.1\r\n`。

### 5. Tests Required
- `CGO_ENABLED=1 mise x -- go test -race ./internal/adapter/xray/... ./internal/service/... ./internal/delivery/http/middleware/... ./internal/adapter/reality/... ./internal/adapter/telegram/...`

### 6. Wrong vs Correct
#### Wrong
```go
// 解锁后裸读客户端指针 + 无条件信任伪造请求头
if err := c.getConn(ctx); err != nil { return err }
_, err := c.handlerCli.AlterInbound(...) // Close() 并发置 nil 时触发 race/panic
```
#### Correct
```go
// 锁内原子拷贝返回局部客户端句柄
handlerCli, _, err := c.getClients(ctx)
if err != nil { return err }
_, err = handlerCli.AlterInbound(...)
```
