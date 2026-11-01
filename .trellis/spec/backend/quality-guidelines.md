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

## 8. 启动反向同步与显式空状态防抹除准则 (Startup Sync & Explicit-None Preservation)

- **问题背景**：Xray-core 官方协议对部分字段的“关闭”状态要求在 `config.json` 中完全省略该键（例如 VLESS `settings.flow` 仅允许 `""` 或 `"xtls-rprx-vision"`，传入 `"none"` 会导致核心报错拒绝启动），而面板数据库通过 `"flow": "none"` 区分“显式禁用流控”与“未配置（默认启用 Vision）”。
- **核心约束**：
  - 在 `ConfigService.syncFromRawJSON`（面板启动 `SyncFromFile` 或保存原生 JSON）从 `config.json` 反向同步回 SQLite 时，若 `config.json` 中 `vless` 入站的 `settings.flow` 为空且 `clients` 均未开启 `flow`，必须保留数据库中原有的 `"flow": "none"` 标记（或写回 `"flow": "none"`），**严禁直接将省略了 `flow` 的 JSON 覆盖回数据库导致 `"flow": "none"` 丢失并在紧随其后的 `RecompileAndApply` 中死灰复燃为 `xtls-rprx-vision`**；
  - 对数据库中已存在的用户（`existingUser != nil`），`syncFromRawJSON` 严禁用单个入站编译后的 `client.flow` 反向覆盖用户的全局 `user.Flow` 策略（防止用户设置的 `"none"` 或 `""` 被篡改）。

---

## 9. 订阅与分享链接导出契约 (Subscription & Share Link Export Invariants)

- **RFC 3986 URI Fragment 编码规范**：
  - 导出 `vless://`、`trojan://`、`ss://`、`hysteria2://`、`socks://` 分享链接的 `#remark` 锚点后缀时，严禁直接使用 `url.QueryEscape`（会将空格编码为 `+` 号导致 v2rayNG / NekoBox 等客户端节点名显示加号）；
  - 必须统一调用 `protocol.EscapeRemark(remark)`（将 `+` 替换为 `%20`），严格遵循 RFC 3986 百分号编码标准。
- **uTLS 指纹 (`fp`) 雙链路一致性**：
  - 聚合订阅（`resolveSubscriptionNodes`）与单节点直连分享链接（`GetUserShareInfo`）必须统一读取系统设置 `sub_default_fingerprint` 并同步至 `protocol.SetDefaultFingerprint`；
  - 优先级契约：`入站显式配置的 fingerprint (realitySettings/tlsSettings.fingerprint) > 系统默认 sub_default_fingerprint > 兜底 "chrome"`。
- **非客户端代理协议过滤**：
  - `dokodemo-door`（任意门端口转发）等非客户端代理协议在 `InboundToNodeConfig`、`InboundsToNodeConfigs` 与 `GetUserShareInfo` 中必须自动跳过，严禁导出为无效客户端订阅节点。
