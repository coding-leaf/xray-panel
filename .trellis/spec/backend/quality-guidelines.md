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
- **Clash / Mihomo REALITY ML-KEM 与模板卫生契约**：
  - `AttachClashTLS`（`internal/protocol/helpers.go`）在 `security == "reality"` 且 `len(realityOpts) > 0` 时必须输出 `reality-opts.support-x25519mlkem768: true`，防止 Mihomo 内核默认剥离 `X25519MLKEM768` 导致 Xray `26.9.8+`/`26.11.9` REALITY 握手静默回落并全节点 `Timeout`（不得向标准 `vless://` URI 注入非标准 query 参数）；
  - `ExportClashWithParams`（`internal/sub/clash.go`）必须将虚拟信息节点（`127.0.0.1:10000` / 零 UUID）从 `AUTO`（`url-test`）组排除（仅保留于 `PROXIES` `select` 组，无真实节点时安全回落 `DIRECT`），且仅输出 `mixed-port`（省略重复 `port`/`socks-port` 避免原生 Mihomo CLI 端口冲突）。

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

---

## 12. 多节点控制面边界、远端编译一致性与凭证隔离契约 (Multi-Node Control Plane & Agent Sync Invariants)

### 1. Scope / Trigger
- **Trigger**: 任何修改 `xray.Manager`（本机 gRPC 用户增删）、`NodeService` / `ConfigService`（远端配置编译、快照缓存失效、Raw JSON 反向同步）、`XrayCompiler.BuildRemoteNodeUsers` / `agent.XrayGRPCAdapter` / `agent.UserSyncer`、`SubService.prepareInboundForNode`、`NodeService.RenderInstallScript` 或 `domain.InboundRepository` 的路径。

### 2. Signatures
- `(domain.Inbound).IsLocal() bool` (`internal/domain/inbound.go`)
- `domain.InboundRepository.FindByNodeID(ctx context.Context, nodeID uint) ([]domain.Inbound, error)` (`internal/domain/repository.go`)
- `domain.AgentSyncUser.Method string` (`json:"method,omitempty"`, `internal/domain/node.go`)
- `normalizeNodeRouting(ctx context.Context, raw domain.RoutingConfig, inbounds []domain.Inbound, enabledOnly bool) domain.RoutingConfig` (`internal/service/config_service.go`)
- `(*ConfigService).validateRawInboundsOwnershipLocked(ctx context.Context, inboundsRaw []json.RawMessage) error` (`internal/service/config_service.go`)
- `const remoteAPIPort = 8080` (`internal/adapter/xray/compiler.go`)

### 3. Contracts
- **本机 vs 远端 gRPC 硬隔离**：
  - `xray.Manager.AddUser` 与 `RemoveUser` 必须首先检查 `if !inbound.IsLocal() { return nil }`；远端入站（`NodeID > 0`）仅由对应 `xray-agent` 通过心跳 `UserSync` 管理，严禁主控机对远端 `tag` 调用本机 gRPC（否则触发 `handler not found` 导致 `UserService` Saga 回滚及 TG `/adduser` 失效）。
  - 端到端测试（如 `agent_e2e_test.go`）必须覆盖**非空 `xrayManager`** 且本机与远端入站共存的用户写操作链路。
- **远端 `SubRoute` 编译单一事实源与缓存失效**：
  - `NodeService.getOrBuildSnapshot`、`ConfigService.GetNodeCompiledConfigRaw` 与 `ConfigService.GetRoutingConfigByNode` 必须共用同一 `normalizeNodeRouting`（包含 `ensureSubRouteRules`），确保 Routing UI、ConfigView 预览与 Agent 实得 `config.json` 绝对一致。
  - `ConfigService.saveAndApplyRawConfigLocked`（覆盖 Raw 保存与 `RollbackSnapshot`）成功后必须调用 `s.invalidateAllNodeCaches()`；锁序固定为 `ConfigService.mu -> NodeService.mu`（叶锁），禁止反向持锁。
  - 主控 Raw JSON 写入与反向同步（`validateRawInboundsOwnershipLocked`）必须拦截任何与现存远端入站（`!existing.IsLocal()`）碰撞的 `tag`。
- **远端 Shadowsocks / SS-2022 增量热加载契约**：
  - `BuildRemoteNodeUsers` 必须向 `AgentSyncUser.Method` 透传加密套件；`agent.XrayGRPCAdapter.AddUser` 据此构造有效 `SettingsJSON` 供 `ResolveAdapter` 命中正确的 `ShadowsocksAdapter` / `Shadowsocks2022Adapter`，`agent.UserSyncer.Reconcile` 必须将 `Method` 纳入变更比对。
- **远端直连订阅端口回退**：
  - `SubService.prepareInboundForNode` 中，若 `!ibCopy.IsLocal() && ibCopy.ExternalPort <= 0`，必须回落至 `ibCopy.Port`（仅本机入站沿用主控 `public_port` 兜底）。
- **Agent 安装脚本与计费公式单源**：
  - `RenderInstallScript` 必须将 `MASTER_URL` 与 `AGENT_TOKEN` 写入 `0600` 权限的 `/etc/xray-agent/agent.env` 并通过 systemd `EnvironmentFile=` 注入，严禁在 `ExecStart` 命令行明文传递 `--token`；默认心跳间隔在安装脚本、`cmd/agent/main.go`、`internal/agent/runner.go` 与 Master 间统一为 `60s`；安装脚本同时负责自动检测架构（`amd64`/`arm64`）、缺失 `xray` 提示以及默认从 GitHub Release 拉取 `/usr/local/bin/xray-agent`（支持 `AGENT_BIN_URL` 覆盖），`.github/workflows/release.yml` 必须同步构建发布 `xray-agent-linux-amd64` 与 `xray-agent-linux-arm64`。
  - `internal/agent/quota_store.go` 必须直接复用 `domain.CalcTrafficByMode` 与 `domain.ReachedQuotaThreshold`（`domain` 零外部依赖，`cmd/agent` 严禁引入 `gin`/`gorm`/`sqlite`/`telegram`）。

### 4. Validation & Error Matrix
- `inbound.NodeID > 0` 进入 `xray.Manager.AddUser/RemoveUser` -> 直接返回 `nil`（零本机 gRPC 调用）；
- 主控 Raw JSON 中出现与某远端入站同名的 `tag` -> `validateRawInboundsOwnershipLocked` 返回校验错误阻断写盘与反向同步；
- 远端 SS-2022 入站热增用户 -> `AgentSyncUser.Method` 非空，Agent 正确调用 `Shadowsocks2022Adapter`。

### 5. Good / Base / Bad Cases
- **Good**: 系统同时启用本机入站和远端 SS-2022 + SubRoute 入站，创建/编辑/重置用户与 TG `/adduser` 均无报错；无需打开保存 `RoutingView`，Agent 同步配置与预览即包含完整 `SubRoute` 规则；远端直连入站留空 `ExternalPort` 导出自身监听端口。
- **Base**: 本机入站 `ExternalPort == 0` 维持主控 `public_port` 回退；旧版 Agent 收到可选 `method` 字段向后兼容忽略。
- **Bad**: `xray.Manager` 漏检 `IsLocal()`、`agent_e2e_test.go` 仅传 `xrayManager = nil`、`ExecStart` 明文暴露 `--token`、或在 `agent/` 中复制一份 `CalcTrafficByMode`。

### 6. Tests Required
- `TestManager_RemoteInboundShortCircuit` (`internal/adapter/xray/manager_test.go`)
- `TestAgentE2E_NonNilXrayManager_LocalAndRemoteInboundCoexistence` (`internal/delivery/http/agent_e2e_test.go`)
- `TestBotHandler_AddUser_LocalAndRemoteCoexistence` (`internal/adapter/telegram/bot_test.go`)
- `TestNodeService_Snapshot_IncludesRemoteSubRouteRulesWithoutRoutingSave` & `TestNodeService_RenderInstallScript_EnvFileAndInterval` (`internal/service/node_service_test.go`)
- `TestConfigService_SaveAndRollbackRawConfig_InvalidatesNodeCaches` & `TestConfigService_RawConfig_RejectsRemoteInboundTagCollision` (`internal/service/config_service_test.go`)
- `TestXrayCompiler_BuildRemoteNodeUsers_ShadowsocksMethod` (`internal/adapter/xray/compiler_test.go`) & `TestXrayGRPCAdapter_AddUser_ShadowsocksMethods` (`internal/agent/xray_grpc_adapter_test.go`)
- `TestSubService_RemoteInboundExternalPortFallback` (`internal/service/sub_service_test.go`)

### 7. Wrong vs Correct
#### Wrong
```go
// 错误 1：未拦截远端入站导致主控本机 gRPC 报 handler not found 并触发 Saga 回滚
inbound, err := m.inboundRepo.GetByTag(ctx, inboundTag)
if !SupportsDynamicUser(inbound.Protocol) { return nil }
return m.grpcClient.AddUser(ctx, inbound, user)

// 错误 2：systemd ExecStart 明文暴露 Agent Token
ExecStart=/usr/local/bin/xray-agent --master-url ${MASTER_URL} --token ${NODE_TOKEN} --interval 20s
```
#### Correct
```go
// 正确 1：仅本机入站操作本机 gRPC，远端入站由 Agent 心跳同步接管
inbound, err := m.inboundRepo.GetByTag(ctx, inboundTag)
if !inbound.IsLocal() || !SupportsDynamicUser(inbound.Protocol) { return nil }
return m.grpcClient.AddUser(ctx, inbound, user)

// 正确 2：凭证写入 0600 环境文件并通过 EnvironmentFile 注入，间隔统一 60s
EnvironmentFile=/etc/xray-agent/agent.env
ExecStart=/usr/local/bin/xray-agent --spool-dir /var/lib/xray-agent --interval 60s
```

---

## 13. 异步节点指令通道、流量批次清理与级联删除契约 (Async NodeCommand & Storage Hygiene Invariants)

### 1. Scope / Trigger
- **Trigger**: 任何修改远端节点指令下发/上报（`NodeService`、`handler_node`/`handler_agent`/`handler_log`/`handler_geodata`、`agent.Runner`）、前端节点命令轮询（`web/src/api/nodes.ts`、`LogsView.vue`、`RoutingView.vue`）、节点删除级联或 `TrafficSyncJob` 后台维护任务的路径。

### 2. Signatures
- `domain.AgentCommand` / `domain.AgentCommandResult` (`internal/domain/node.go`)
- `GET /api/admin/nodes/:id/commands/:commandId` (JWT 鉴权) -> `NodeCommandSnapshot`
- `POST /api/agent/commands/:id/result` (`AgentAuth` 鉴权) -> `{"status":"ok"}`
- `(*NodeService).GetNodeCommandStatus(ctx, nodeID, commandID)` & `ReportAgentCommandResult(ctx, nodeID, res)` (`internal/service/node_service.go`)
- `(*GORMNodeRepository).Delete(ctx, id)` (`internal/adapter/repository/node_repo.go`)
- `(*TrafficSyncJob).WithNodeBatchRepo(repo domain.NodeTrafficBatchRepository)` (`internal/delivery/cron/traffic_sync.go`)

### 3. Contracts
- **非阻塞异步指令状态机（严禁跨越 60s `WriteTimeout` 同步阻塞）**：
  - 远端操作统一经 `NodeCommand` 内存 TTL 注册表流转：`queued -> dispatched -> succeeded | failed | expired`（指令 TTL `180s`，终态结果保留 `180s`，惰性清理）。
  - `GET /api/logs?nodeId=` 未命中 30s 缓存时立即返回 `200` + `{pending:true, commandId, status}`；`POST /api/geodata/update?nodeId=` 立即返回 **`202 Accepted`** + `{pending:true, commandId, status}`。
  - 同一节点同类型同目标（`log_tail:<nodeID>:<service>`、`geodata_update:<nodeID>`）已有活跃指令时必须去重复用（`queued` 日志指令若新请求 `lines` 更大则原地提升）；HTTP 客户端取消或超时**严禁删除**队列中尚未下发的指令。
  - `DeleteNode` 必须同步清理该节点内存中的命令记录、去重键与日志缓存。
- **Agent 后台非阻塞执行与断网兜底**：
  - `Runner.SyncOnce` 收到 `resp.Commands` 后必须在受控后台 goroutine（`cmdWG` 追踪、响应 `ctx.Done()`、按命令 ID/类型防重入）执行，严禁阻塞主心跳循环；
  - 执行完毕优先立即调用 `POST /api/agent/commands/:id/result` 上报，上报失败时暂存有界队列（上限 `32`）待下次 `SyncOnce` 随 `AgentSyncRequest.CommandResults` 补传。
- **跨节点归属校验**：
  - `GetNodeCommandStatus` 与 `ReportAgentCommandResult` 均必须校验 `rec.NodeID == nodeID`，不匹配一律返回 `domain.ErrNotFound`（HTTP `404`）。
- **SQLite 事务级联与独立清理**：
  - `GORMNodeRepository.Delete` 必须在同一事务内先删 `node_traffic_batches` 与 `node_traffic_dailies`（`Where("node_id = ?", id)`）再删 `nodes` 主记录；
  - `TrafficSyncJob` 的 `PruneBatchesBefore`（保留 7 天幂等窗口，每日单次去重，失败允许重试）必须挂在**不依赖 `xrayManager != nil`** 的后台维护协程中，并在 `main.go` 通过 `.WithNodeBatchRepo(nodeRepo)` 注入。
- **写接口 `RefreshSyncStatus` 与前端轮询清理**：
  - `CreateNode`、`UpdateNode`（在 `InvalidateNode` 后）、`SetNodeDesiredState`、`ResetNodeCycleTraffic` 返回前均须调用 `RefreshComputed` + `RefreshSyncStatus`；
  - 前端 `LogsView.vue` / `RoutingView.vue` 的 `2s` 轮询 `setInterval` 必须在 `watch(activeNodeId)`、目标切换与 `onUnmounted` 清理，并使用单调序号守卫拦截过期响应。

### 4. Validation & Error Matrix
- 跨节点查询 `GET /api/admin/nodes/2/commands/<node1-cmd>` 或跨节点 Agent 上报 -> `404 Not Found`；
- Agent 上报路径 `:id` 与 body `id` 不一致 -> `400 Bad Request`；
- 超过 `180s` 未完成指令 -> 状态自动转为 `expired` 并释放活跃去重键；
- 删除不存在的节点 -> 事务内 `RowsAffected == 0` 返回 `domain.ErrNotFound`。

### 5. Good / Base / Bad Cases
- **Good**: 60s 心跳周期下触发远端日志或 GeoData 更新，HTTP `< 100ms` 即返回 `commandId`；Agent 慢网下载 GeoData 耗时 90s 期间心跳照常上报不掉线，完成后立即回传，前端 2s 轮询自动收敛渲染。
- **Base**: 30s 内再次查询同服务且 `lines <= cached.RequestedLines` 的远端日志直接命中缓存（`cached: true, pending: false`）；`xrayManager == nil` 模式下每日定时清理仍正常修剪 7 天前 `node_traffic_batches`。
- **Bad**: 在 HTTP handler 中同步 `select` 等待 Agent 心跳（触发 25s 死循环或 60s `WriteTimeout` 断连）；在 `defer` 中删除 `pending*`；或把 `PruneBatchesBefore` 挂在 `xrayManager == nil` 会早退的 `syncOnce` 末尾。

### 6. Tests Required
- `internal/service/node_service_test.go`：异步入队去重、ctx 取消不删队、180s TTL 过期、跨节点归属 404、`DeleteNode` 内存清理、4 个写接口 `RefreshSyncStatus`。
- `internal/agent/runner_test.go`：慢速 GeoData 不阻塞并发 `SyncOnce`、即时上报与失败后心跳 `CommandResults` 补传、`-race` 零 goroutine 泄漏。
- `internal/adapter/repository/node_repo_test.go` & `internal/delivery/cron/traffic_sync_test.go`：事务级联删子表、`xrayManager == nil` 每日清理。
- `internal/delivery/http/handler_geodata_test.go`、`handler_agent_test.go`、`agent_e2e_test.go`：`202 Accepted`、跨节点 404 与 Master-Agent 端到端闭环。

### 7. Wrong vs Correct
#### Wrong
```go
// 错误：同步等待超过 WriteTimeout=60s，且超时 defer 删除未下发指令造成 60s 心跳死循环
defer func() { delete(s.pendingLogReqs, nodeID) }()
select { case res := <-waitCh: ... case <-time.After(25 * time.Second): ... }
```
#### Correct
```go
// 正确：非阻塞入队/复用带 180s TTL 的 NodeCommand，立即返回 commandId 由前端 2s 轮询；Agent 后台执行完毕即时 POST 上报
cmd := s.enqueueOrReuseCommandLocked(nodeID, domain.NodeCommandTypeLogTail, svc, lines, now)
return &RemoteNodeLogsResponse{NodeID: nodeID, Service: svc, Pending: true, CommandID: cmd.ID, Status: cmd.Status}, nil
```

---

## 14. 节点仓储 KISS 统一、纯标准库 `internal/pkg/geodata` 与 Portal 单源化契约

- **单一 `domain.NodeRepository` 与零运行时接口断言**：
  - 节点 CRUD、流量批次、配额与配置持久化统一收敛于 `domain.NodeRepository`（禁止拆出 `NodeTrafficBatchRepository`/`NodeQuotaRepository`/`NodeConfigRepository` 等碎片子接口，禁止在 `service` 层对仓储或编译器做运行时 `type assertion`）；测试局部 mock 通过匿名嵌入 `domain.NodeRepository` 零样板实现。
  - 远端全量编译方法 `CompileRemoteNodeFullToJSON` 统一归属 `RemoteNodeCompiler` / `ConfigCompiler`，禁止再拆出 `RemoteNodeFullCompiler`。
- **纯标准库 `internal/pkg/geodata` 共享下载原语**：
  - Master (`GeoDataService`) 与 Agent (`HTTPGeoDataUpdater`) 必须共用 `internal/pkg/geodata` 维护的镜像 URL、`DefaultMinFileSize`（`100KB` HTML 错误页拦截）、`NewHTTPClient` 与 `DownloadWithFallback`；
  - `internal/pkg/geodata` **严禁导入任何第三方或非 stdlib 依赖**，确保 `cmd/agent` 依赖树保持零 `gin`/`gorm`/`sqlite`/`telegram` 泄漏。
- **Cloudflare Pages 静态 Portal 单源化**：
  - 静态提取门户 HTML 的唯一权威源文件为 `deploy/cloudflare-pages/portal/index.html`（严禁维护逐字节重复的 `portal.html` / `standalone-portal.html` 副本）；
  - `deploy/cloudflare-pages/_worker.js` 将 `/portal`、`/portal/`、`/portal.html`、`/portal/index.html` 统一改写至 `/portal/`（保留 query string）后转交 `env.ASSETS.fetch`，并由 `deploy/gateway.test.js` 持续回归验证。

---

## 15. 远端 Agent 唯一权威配置源、零断流落盘与运行态自愈契约 (Agent SSOT & Runtime Self-Healing Invariants)

### 1. Scope / Trigger
- **Trigger**: 任何修改 Master↔Agent 同步协议（`domain.AgentSyncRequest`、`agent.SyncRequest`、`NodeService.HandleAgentSync` / `getOrBuildSnapshot`）、Agent 磁盘配置与本地状态持久化（`agent.ConfigManager`、`agent.SyncStateStore`）、Xray gRPC 观测原语（`internal/adapter/xray/proto/{proxyman,stats}.proto`、`grpc_client.go`）或 `agent.UserSyncer` / `agent.Runner` 收敛逻辑的路径。

### 2. Signatures
- `domain.AgentSyncRequest.DiskConfigHash string` / `agent.SyncRequest.DiskConfigHash string` (`json:"diskConfigHash,omitempty"`)
- `(*ConfigManager).Persist(ctx context.Context, configJSON string) error` (`internal/agent/config_manager.go`)
- `(*ConfigManager).HashCurrentConfig() (string, error)` & `InspectConfigInbounds() ([]ConfigInboundMeta, error)` (`internal/agent/config_manager.go`)
- `(*SyncStateStore).Load() (SyncState, bool, error)` & `Save(state SyncState) error` (`internal/agent/sync_state.go`)
- `(*xray.GRPCClient).GetInboundUsers(ctx context.Context, inboundTag string) ([]RuntimeInboundUser, error)` & `GetSysUptime(ctx context.Context) (uint32, error)` (`internal/adapter/xray/grpc_client.go`)
- `(*UserSyncer).ReconcileWithRuntime(ctx context.Context, desired []RemoteUser, dynamicTags []string, staticEmails map[string]struct{}) (added int, removed int, err error)` & `ErrInboundHandlerMissing` (`internal/agent/user_sync.go`)

### 3. Contracts
- **唯一权威配置源（SSOT）**：Master SQLite DB 经 `XrayCompiler` 编译的 `cachedNodeSnapshot`（`ConfigJSON` + `FullConfigHash` + `ConfigHash` + `Users` + `UsersHash`）是全集群唯一权威配置源。**严禁** Agent 本地拼接或 Patch `config.json`（会破坏 SSOT 并误删 `relay-auto:<tag>@internal` 与 `<tag>@trojan.local` 静态客户端）。
- **两级指纹分离与双向滚动兼容**：
  - `ConfigHash = sha256(structuralBytes)` 仍唯一控制 `ConfigUnchanged`（是否需要 reload/restart Xray）与 `Node.RefreshSyncStatus`；
  - `FullConfigHash = sha256(fullConfigBytes)` 与 `req.DiskConfigHash`（Agent 实测磁盘 `config.json` 的 SHA-256，文件缺失上报 `"missing"`）比对：仅当 `!configUnchanged || (req.DiskConfigHash != "" && req.DiskConfigHash != snap.FullConfigHash)` 时 Master 下发 `resp.ConfigJSON`。旧 Agent 省略 `diskConfigHash` 时线协议 100% 保持原样；新 Agent 对接旧 Master（`ConfigJSON == ""`）仍完成 gRPC 热更新与运行态自愈。
- **零断流落盘顺序**：`ConfigUnchanged == true && resp.ConfigJSON != ""` 时，必须先在 `ConfigManager.mu` 保护下执行 `Persist`（临时文件 -> `xray run -test` -> `.bak` -> 原子 `Rename`，**绝不调用 `reloadService`**），成功后再执行 gRPC 运行态收敛；任何一步失败均不得推进 `usersHash` 与 `sync_state.json`。
- **上游 Xray-core gRPC 线协议精确对齐**：
  - `HandlerService.GetInboundUsers`：`/xray.app.proxyman.command.HandlerService/GetInboundUsers`，`GetInboundUserRequest{tag=1, email=2}` -> `GetInboundUserResponse{users=1}`；
  - `StatsService.GetSysStats`：`/xray.app.stats.command.StatsService/GetSysStats`，`SysStatsRequest{}` -> `SysStatsResponse{..., Uptime=10}`。
- **静态客户端保护与运行态自愈**：`ReconcileWithRuntime` 必须跳过 `relay-auto:*@internal`、`*@trojan.local` 及磁盘权威配置中的静态客户端（含 `len(desired) == 0` 场景）；遇到 `ErrInboundHandlerMissing` 触发 `ReloadAndProbe` / 结构恢复；遇到 `codes.Unimplemented` 优雅降级为 `GetSysStats` `Uptime` 下降检测 + `ResetBaseline(nil)` 补齐。
- **Agent 健康重启零断流**：启动时从 `{spoolDir}/sync_state.json` 恢复状态（与 `QuotaStore.LocalCircuitBroken` 对齐），仅当实测磁盘 SHA-256 匹配 `FullConfigHash` 时复用哈希，避免重启 `xray-agent` 触发无意义 Xray reload。

### 4. Validation & Error Matrix
- `xray run -test` 失败 -> `ConfigManager.Persist` 返回错误且原 `config.json` 完好，`usersHash` 不推进；
- `UserSyncer.AddUser` 遇到 `handler not found` / `failed to get handler` -> **严禁忽略**，返回 `ErrInboundHandlerMissing` 触发 `Runner` 重载/结构恢复；
- `GetInboundUsers` 返回 `codes.Unimplemented` -> 回退至 `GetSysStats` + 内存基线 `Reconcile`；
- `Runner.mu` 锁安全 -> 禁止在持锁期间发起 HTTP/gRPC/磁盘阻塞调用。

### 5. Good / Base / Bad Cases
- **Good**: 仅增删改用户时，磁盘 `config.json` 无重启原子更新至 `FullConfigHash`，Xray reload 次数为 0；Xray 外部重启或磁盘被篡改后 1 个心跳周期内自动收敛且静态客户端完好；`xray-agent` 重启且磁盘/运行态一致时 0 次 Xray reload。
- **Base**: 旧 Agent（无 `diskConfigHash`）与新 Master、新 Agent 与旧 Master（`ConfigJSON == ""`）均平滑兼容。
- **Bad**: 在 Agent 端手写 JSON Patch 改 `settings.clients`；或在 `AddUser` 中忽略 `handler not found` 将失败用户误标入 `applied`；或未验证上游 `.proto` 字段编号直接手猜 `SysStatsResponse.Uptime`。

### 6. Tests Required
- `TestGRPCClient_GetInboundUsersAndSysUptime_LoopbackRoundtrip` (`internal/adapter/xray/grpc_client_test.go`)
- `TestNodeService_HandleAgentSync_DiskConfigHashDelivery` (`internal/service/node_service_test.go`)
- `TestConfigManager_Persist_AtomicWithoutReload` (`internal/agent/config_manager_test.go`)
- `TestUserSyncer_AddUser_HandlerNotFoundNotIgnored` & runtime drift / static-client protection tests (`internal/agent/user_sync_test.go`)
- `TestRunner_FailRepro_HotUpdatePersistsDiskConfig` & restart/drift/agent-restart tests (`internal/agent/runner_test.go`)
- `TestMasterAgent_E2E_DiskPersistAndRuntimeSelfHeal` (`internal/delivery/http/agent_e2e_test.go`)

### 7. Wrong vs Correct
#### Wrong
```go
// 错误 1：仅通过 gRPC 热更新内存，不落盘 Master 权威 ConfigJSON，Xray 重启即丢用户
if !resp.UsersUnchanged {
    r.userSync.Reconcile(ctx, resp.Users)
    r.usersHash = resp.UsersHash
}

// 错误 2：AddUser 复用 RemoveUser 的忽略列表，静默吞掉 handler not found
if err := s.client.AddUser(ctx, u); err != nil && !isIgnorableXrayError(err) { ... }
```
#### Correct
```go
// 正确 1：先校验无重启原子落盘 Master 权威 ConfigJSON，再实测 Xray 运行态收敛
if resp.ConfigJSON != "" {
    if err := r.cfgMgr.Persist(ctx, resp.ConfigJSON); err != nil { return false, err }
}
added, removed, err := r.userSync.ReconcileWithRuntime(ctx, targetUsers, dynamicTags, staticEmails)

// 正确 2：拆分 AddUser / RemoveUser 容错判定，AddUser 绝不忽略 handler not found
if err := s.client.AddUser(ctx, u); err != nil && !isIgnorableAddUserError(err) {
    if isHandlerNotFoundError(err) { return ..., fmt.Errorf("%w: %v", ErrInboundHandlerMissing, err) }
    ...
}
```
