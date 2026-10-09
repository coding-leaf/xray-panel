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

## 6. Xray 访问日志解析契约与演进预告 (Log Parsing Contract & Evolution)

- **核心兼容契约**：
  - 访问日志流式过滤必须同时支持 `->`（明确命中分流规则）与 `>>`（规则未命中触发的默认出口 Fallback 兜底）；
  - 必须兼容识别客户端传输层前缀格式（包括原生 IP 格式以及 Xray-core v26+ 输出的 `from tcp:<ip>:<port>` / `from udp:<ip>:<port>`）；
- **架构演进路线预告**：
  - 当前采用的集中式正则表达式方案存在与 Xray 核心格式偶发脱节的隐患；后续版本计划重构流式分词解析器（Tokenizer/Scanner），淘汰硬编码正则匹配，全面提升对多版本 Xray 日志的解析鲁棒性与零堆分配性能。
