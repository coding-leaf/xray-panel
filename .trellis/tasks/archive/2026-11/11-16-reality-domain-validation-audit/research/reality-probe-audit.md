# REALITY 域名合法性校验机制与 xray-panel 探测方法深度审计报告

> 调研范围：基于本地 Go 模块缓存中的 `github.com/xtls/xray-core@v1.260327.0`、`github.com/xtls/reality@v0.0.0-20260322125925-9234c772ba8f` 真实源码，以及 `xray-panel` 后端与前端完整实现。本报告所有结论均来自本地源码直接观测，未混入未核实的外部推测。

---

## 一、Xray REALITY 协议真实机制与目标域名（`dest` / `serverNames`）要求

### 1.1 依赖版本与架构事实
- `xray-panel` 的 `go.mod`（第 1–66 行）未直接引入 `github.com/xtls/xray-core` 或 `github.com/xtls/reality`（面板通过子进程与 gRPC/Protobuf 控制 Xray 内核）。
- 本地 `GOMODCACHE`（`D:\computerRequirements\go_rep\pkg\mod\github.com\xtls`）中存在：
  - `xray-core@v1.260327.0`
  - `reality@v0.0.0-20260322125925-9234c772ba8f`

### 1.2 配置解析阶段的校验（`xray-core`）
在 `xray-core@v1.260327.0/infra/conf/transport_internet.go`（`REALITYConfig.Build`，第 807–926 行）：
1. **`target` / `dest` 解析**（第 812–843 行）：支持纯端口（补全为 `localhost:<port>`）、`host:port`（`Type = "tcp"`）、以及 `@` / `/` 开头的 Unix Domain Socket（`Type = "unix"`）。`xray-core` 配置解析阶段**不会**对 `dest` 发起网络探测或验证证书。
2. **`serverNames` 校验**（第 847–849 行）：仅校验 `len(c.ServerNames) != 0`；在 `README.md`（第 49 行）明确说明**暂不支持 `*` 通配符**（运行时使用 `map[string]bool` 精确匹配）。
3. **Apple/iCloud 风险提示**（第 912–916 行）：遍历 `config.ServerNames`，若包含 `"apple"` 或 `"icloud"` 会输出警告：`REALITY: Choosing apple, icloud, etc. as the target may get your IP blocked by the GFW`。

### 1.3 运行时服务端握手与回落机制（`xtls/reality`）
在 `reality@v0.0.0-20260322125925-9234c772ba8f/tls.go` 的 `Server` 函数（第 162–476 行）：
1. **立即拨号 `dest`**（第 168–172 行）：收到任意客户端连接后，REALITY 服务端**立即**通过 `config.DialContext(ctx, config.Type, config.Dest)` 拨号目标站点 `dest`。若拨号失败，直接关闭客户端连接并返回 `REALITY: failed to dial dest`。
2. **ClientHello 鉴权与 `serverNames` 校验**（第 207–270 行）：
   - 客户端必须使用 **TLS 1.3**（`hs.c.vers == VersionTLS13`，第 211 行）。
   - 客户端 `SNI` 必须精确命中 `config.ServerNames[hs.clientHello.serverName]`（第 211 行）。
   - 客户端 KeyShare 必须包含 `X25519`（32 字节）或 `X25519MLKEM768`（`mlkem.EncapsulationKeySize768 + 32` 字节，第 214–228 行），用于推导 `AuthKey` 并用 AES-GCM 解密 `sessionId` 中的客户端版本号、时间戳、ShortId。
   - **任一条件未通过**：`hs.c.conn` 保持为 `MirrorConn`，连接退化为**纯透明 TCP 双向转发**（第 271–286 行、第 433–447 行），由 `dest` 直接与探测者/客户端完成真实 TLS 握手。
3. **目标站点（`dest`）返回的 TLS 握手包硬性校验**（第 290–366 行）：
   REALITY 服务端读取 `dest` 对转发过去的 `ClientHello` 的响应（`s2cSaved`），并逐条校验前 7 个握手阶段记录（`Server Hello`、`Change Cipher Spec`、`Encrypted Extensions`、`Certificate`、`Certificate Verify`、`Finished`、`New Session Ticket`）：
   - **总缓冲与单条记录上限**：`size = 8192` 字节（第 140、312、334 行）。若 `dest` 响应过长超出 8192 字节，握手中断并回落转发。
   - **Record 0（ServerHello）硬性要求**（第 323–324、353–362 行）：
     - 记录层版本 `VersionTLS12`（`0x0303`），Handshake 类型为 `typeServerHello`（不能是 TLS Alert 如 `unrecognized_name`，也不能是明文 HTTP 报错）。
     - 协商版本必须为 **TLS 1.3**（`hs.hello.supportedVersion == VersionTLS13`，第 356 行）。
     - 密码套件必须是支持的 TLS 1.3 套件（`cipherSuiteTLS13ByID(hs.hello.cipherSuite) != nil`，第 357 行）。
     - **ServerShare 密钥交换组硬性要求**（第 358–359 行）：`dest` 返回的 `ServerHello` 必须直接选定 **`X25519`**（32 字节）或 **`X25519MLKEM768`**（`mlkem.CiphertextSize768 + 32` 字节）。若 `dest` 仅支持 `P-256`（`secp256r1`）或触发 `HelloRetryRequest`，则校验失败（`hs.c.out.handshakeLen[0] == 0`），REALITY 无法签发临时证书，只能回落直连。
   - **Record 1（CCS）与 Record 2+（加密握手）**：CCS 长度必须为 6（第 337 行），后续加密记录必须为 `recordTypeApplicationData`（第 326 行）。
   - **握手长度模仿与 Post-Handshake 探测**：REALITY 服务端记录 `dest` 各阶段握手记录的精确长度（`hs.c.out.handshakeLen[i]`），并在 `record_detect.go`（`DetectPostHandshakeRecordsLens`，第 21–102 行）中预先使用 `sni` + 3 种 ALPN（无 ALPN、`http/1.1`、`h2, http/1.1`）探测 `dest` 的握手后记录长度，使自身返回的 TLS 1.3 握手包长度特征与 `dest` 完全一致。

### 1.4 服务端不校验但客户端/抗探测关心的维度
- **REALITY 服务端本身不校验证书内容**：服务端没有客户端的临时私钥，无法解密 `dest` 返回的 `EncryptedExtensions` 和 `Certificate`。因此，即使 `dest` 证书过期、自签名或域名不匹配，只要 `dest` 能正常返回符合上述格式的 TLS 1.3 + X25519 `ServerHello` 及后续记录，REALITY 代理隧道本身仍能建立。
- **但抗主动探测与客户端回落爬虫强依赖以下条件**：
  1. **真实有效证书与域名匹配**：当主动探测者（无合法 ShortId）连接 REALITY 端口时，流量被透明转发给 `dest`；若 `dest` 返回过期/自签/域名不匹配证书，探测者立即可识别异常。此外，`xray-core/transport/internet/reality/reality.go`（第 104–114 行）在客户端收到真实证书（`!uConn.Verified`）时，会执行 `certs[0].Verify(opts)`（校验证书链与 `DNSName: c.ServerName`）。
  2. **HTTP/2（`h2`）支持**：`xtls/reality/README.md`（第 84 行）明确列出“通常代理用途，目标网站最低标准：国外网站，支持 TLSv1.3 与 H2，域名非跳转用（主域名可能被用于跳转到 www）”。同时客户端爬虫回落（`xray-core/transport/internet/reality/reality.go` 第 187 行）硬编码使用 `&http2.Transport{}`。
  3. **非跳转域名 & 低延迟 & 禁 Apple/iCloud**：避免 301/302 跳转域名、避免与 VPS 地理位置/延迟差异过大、避免使用已被 GFW 重点关注的 Apple/iCloud 域名。

### 1.5 REALITY 目标域名合法性要求总结表

| 维度 | 分类 | 具体要求 | 源码依据 |
|---|---|---|---|
| TCP/Unix 可达性 | **协议硬性** | `dest` 必须可由服务端成功拨号连接 | `reality/tls.go:168-172` |
| TLS 1.3 支持 | **协议硬性** | `dest` 对指定 `SNI` 必须协商 TLS 1.3（`supportedVersion == 0x0304`），不能返回 TLS 1.2 或 Alert | `reality/tls.go:355-356` |
| X25519 / X25519MLKEM768 | **协议硬性** | `dest` 的 `ServerHello.serverShare` 必须选择 `X25519` 或 `X25519MLKEM768`（不能是 P-256 或 HRR） | `reality/tls.go:358-359` |
| 握手记录尺寸 | **协议硬性** | 初始握手响应总缓冲 ≤ 8192 字节，单条记录 ≤ 8192 字节 | `reality/tls.go:140,312,334` |
| SNI 精确匹配 | **协议硬性** | `serverNames` 非空且不支持 `*` 通配符；`dest` 需能接受该 SNI 完成握手 | `reality/tls.go:211`, `README.md:49` |
| 证书链可信与未过期 | **抗探测/回落硬性** | 自签/不可信 CA/过期/SAN 不匹配会导致探测者识别异常及客户端回落 `Verify` 失败 | `xray-core/.../reality.go:104-114` |
| ALPN 支持 `h2` | **抗探测/回落软性（强烈建议）** | 官方最低标准要求支持 H2；客户端爬虫回落硬编码使用 `http2.Transport` | `reality/README.md:84`, `xray-core/.../reality.go:187` |
| 非跳转域名（非 301/302） | **抗探测软性** | 主域名若 301 跳转 `www` 会降低伪装真实性 | `reality/README.md:84` |
| 避免 Apple/iCloud | **抗封锁软性** | `xray-core` 显式告警可能导致 VPS IP 被 GFW 封锁 | `xray-core/.../transport_internet.go:912-916` |

---

## 二、`xray-panel` 现有 Reality 域名探测实现审计

### 2.1 现有实现架构与调用链
1. **配置提取**：`internal/domain/reality_evaluator.go`（`ExtractRealityTargets`，第 17–96 行）从 Inbound 的 `StreamSettings` 提取 `dest`/`target`、端口（默认 443）与 `serverNames` 列表。
2. **网络探测**：`internal/adapter/reality/prober.go`（`DefaultRealityProber.Probe`，第 47–152 行）：
   - `net.Dialer{Timeout: 3s}` 建立 TCP 连接（第 84–96 行）。
   - Go 标准库 `crypto/tls.Client` 握手（`ServerName: sni`, `InsecureSkipVerify: true`, `NextProtos: []string{"h2", "http/1.1"}`, `MinVersion: tls.VersionTLS10`，第 100–121 行）。
   - 1 秒超时窗口内发起轻量 `GET /`（按协商 ALPN 选择 `http2.Transport.NewClientConn` 或原生 HTTP/1.1 请求，第 134–213 行）获取响应头。
3. **规则评估**：`internal/domain/reality_evaluator.go`（`EvaluateRealityProbe`，第 108–167 行）按固定优先级判定：
   - ① `tcpErr != nil` → `error (TCP_UNREACHABLE)`
   - ② `tlsVer < tls.VersionTLS13` → `error (TLS_VERSION_LOW)`
   - ③ `detectCloudflareCDN(cert, headers)` → `error (CDN_DETECTED)`
   - ④ `cert == nil` / `!cert.VerifyHostname(sni)` / 过期 → `error`；`< 7 天` → `warning (CERT_EXPIRING_SOON)`
   - ⑤ `alpn == ""` → `warning (ALPN_MISSING)`
   - ⑥ 其余 → `ok`
4. **调度与 API / 前端展示**：
   - `internal/service/reality_monitor_service.go`：并发度 5 遍历所有 `(dest, port, serverName)` 组合，更新内存缓存并在 `warning`/`error` 时触发 `AlertService.NotifyRealityAbnormal`。
   - `internal/delivery/cron/reality_sync.go`：启动异步探测一次，此后每 12 小时轮询（单轮总超时 30s）。
   - `internal/delivery/http/handler_inbound.go`（第 114–161 行）与 `router.go`（第 123–126 行）：暴露 `GET /inbounds/reality-status` 与 `POST /inbounds/reality-status/check`。
   - 前端：`web/src/api/reality.ts`、`DashboardView.vue`、`InboundTable.vue`、`InboundDetailDrawer.vue` 展示状态徽章、目标、证书剩余天数、TLS 版本、延迟与详情。

### 2.2 逐项对比评估：准确点 vs 盲区与误判

#### A. 准确且有价值的检查
1. **TCP 可达性 + 分离 `dest` 与 `serverName` 探测**（`prober.go:76-105`、`reality_monitor_service.go:84-92`）：准确还原了 REALITY 拨号 `dest:port` 但发送 `SNI = serverName` 的行为，并逐一覆盖 `serverNames` 列表。
2. **TLS 1.3 强制校验且 `MinVersion: TLS 1.0`**（`prober.go:104`、`reality_evaluator.go:123-125`）：能正确区分“目标仅支持 TLS 1.2”与“握手失败”，对 `TLS 1.2` 精准报 `TLS_VERSION_LOW`。
3. **证书主机名匹配与有效期检查**（`reality_evaluator.go:142-158`）：虽然服务端不校验，但对于抗主动探测与客户端回落非常关键。

#### B. 假阴性（漏报：面板报 `ok`，但 REALITY 实际不可用或伪装极差）
1. **未校验密钥交换曲线（X25519 / X25519MLKEM768）** — **严重漏报**：
   - `prober.go:100-132` 使用 Go 标准库 `tls.Client` 且未限制 `CurvePreferences`，也未检查 `tlsState.CurveID`。
   - Go 标准库默认同时提供 `X25519MLKEM768`、`X25519`、`CurveP256`、`CurveP384`、`CurveP521`。若目标站点仅支持 `P-256`（或对 `X25519` 返回 `HelloRetryRequest`），Go `tls.Client` 仍会握手成功，面板判定为 `ok`；但真实 `xtls/reality` 服务端在 `tls.go:358-359` 会直接拒绝非 `X25519`/`X25519MLKEM768` 的 `ServerHello`，导致 REALITY 代理完全无法握手。
2. **未校验证书信任链（`InsecureSkipVerify: true` 掩盖自签/不可信证书）** — **中等漏报**：
   - `prober.go:102` 设置 `InsecureSkipVerify: true`，随后 `reality_evaluator.go:143` 仅调用 `cert.VerifyHostname(sni)`，完全未调用 `cert.Verify(x509.VerifyOptions{...})` 校验证书链。
   - 若目标返回自签名证书或未受信任 CA 签发的证书（只要 SAN 匹配 `sni`），面板仍报 `ok`，但主动探测者与 REALITY 客户端回落（`reality.go:104-114`）都会校验失败。
3. **ALPN 仅要求非空，未区分 `h2` 与 `http/1.1`** — **轻/中等漏报**：
   - `reality_evaluator.go:161-163` 仅在 `alpn == ""` 时给 `warning`。若目标仅支持 `http/1.1`，面板报 `ok`，但官方标准要求支持 `h2` 且客户端回落爬虫硬编码使用 `http2.Transport`。
4. **未检测 HTTP 重定向（301/302）与异常状态码**：
   - `prober.go:189-212` 仅读取 `resp.Header` 而丢弃 `resp.StatusCode`（如 `301`/`302` 跳转至 `www` 子域，或 `403`/`5xx`）。与官方“域名非跳转用”要求不符。
5. **CDN 检测仅覆盖 Cloudflare**：
   - `detectCloudflareCDN`（`reality_evaluator.go:170-201`）仅识别 Cloudflare 证书/响应头，对 Fastly、Akamai、CloudFront 等其他常见 CDN 完全无感知。
6. **未校验 `serverNames` 通配符与 Apple/iCloud 风险**：
   - `compiler.go:347-357` 与 `reality_evaluator.go` 均未拦截 `*.example.com` 通配符（REALITY 不支持）；
   - `web/src/views/inbounds/components/InboundFormDrawer.vue:522,525` 甚至仍将 `gateway.icloud.com` 作为表单 placeholder（与 `xray-core` 源码警告相悖）。
7. **未支持 Unix Domain Socket 形式的 `dest`**：
   - `compiler.go:338` 强制要求 `net.SplitHostPort`，`prober.go:89` 硬编码 `"tcp"` 拨号，面板暂不支持本机 Nginx/Caddy Unix Socket 回落目标。

#### C. 假阳性 / 语义偏差（面板报 `error`，但 REALITY 协议本身可用）
1. **将 Cloudflare CDN 直接判定为 `error (CDN_DETECTED)`**（`reality_evaluator.go:128-131`）：
   - 套用 Cloudflare 的站点通常支持 TLS 1.3 + X25519 + H2，REALITY 协议在多数情况下仍可建立连接；其主要风险在于共享 Anycast IP 伪装度下降、回落可能触发 CF WAF/403 或因域名解析不一致被识别。直接定性为硬性 `error`（与 TCP 不可达、TLS 1.2 同级）并触发异常告警，容易对已稳定运行的节点产生误报；更合理的分级应为 `warning`。
2. **IPv6 `dest` 显示格式化小缺陷**：
   - `reality_evaluator.go:240` 使用 `if port > 0 && !strings.Contains(dest, ":")` 拼接端口，若 `dest` 为裸 IPv6 地址（如 `2606:4700::1`）会因包含 `:` 而漏拼端口（应使用 `net.JoinHostPort`）。

---

## 三、可落地的改进建议（供后续 Milestone 参考）

1. **P0（协议准确性修复）**：
   - 在 `DefaultRealityProber.Probe` 中限制 `tls.Config.CurvePreferences`（或读取 `tlsState.CurveID`），显式校验协商曲线必须为 `tls.X25519` 或 `tls.X25519MLKEM768`；若不是，直接判定为 `error`（如 `KEY_EXCHANGE_UNSUPPORTED`），消除最大假阴性盲区。
   - 增加系统根证书链校验（使用 `tlsConn.ConnectionState().VerifiedChains` 或 `PeerCertificates` 构造 `Intermediates` 调用 `cert.Verify`），对自签/不可信证书至少给出 `warning` 或 `error (CERT_UNTRUSTED)`。
2. **P1（评估分级与抗探测指标对齐）**：
   - 将 `CDN_DETECTED` 从 `error` 调整为 `warning`（说明伪装质量风险而非协议不可用），并补充常见 CDN 头特征（如 `Server: AkamaiGHost`、`Via`、`x-amz-cf-id` 等）。
   - 对 `ALPN == "http/1.1"` 给出 `warning`（提示建议支持 `h2` 以匹配官方最低标准与客户端回落）。
   - 在 `ProbeResult` 中记录 HTTP 状态码与 `Location` 头，若为 `301`/`302`/`307`/`308` 跳转给出 `warning (HTTP_REDIRECT)`。
   - 修复 `BuildRealityCheckItem` 中 IPv6 `dest` 端口拼接问题（改用 `net.JoinHostPort`）。
3. **P2（配置引导与防呆）**：
   - 在 `compiler.go` / `ExtractRealityTargets` 校验 `serverNames` 不包含 `*` 通配符。
   - 将 `InboundFormDrawer.vue` 中 `gateway.icloud.com` 示例替换为非 Apple/iCloud 的常规 TLS 1.3 + H2 示例域名，并在检测到 `apple`/`icloud` 时给出前端提示。

---

## 四、关联 `.trellis/spec/` 规范索引

后续进入设计与实现阶段（编写 `implement.jsonl` / `check.jsonl`）需加载以下项目规范：
- **后端规范**：
  - `.trellis/spec/backend/index.md`
  - `.trellis/spec/backend/directory-structure.md`（`internal/domain`、`internal/adapter/reality`、`internal/service` 分层边界）
  - `.trellis/spec/backend/error-handling.md`（显式错误传播与 HTTP 映射）
  - `.trellis/spec/backend/quality-guidelines.md`（并发安全与 KISS 规范）
- **前端规范**：
  - `.trellis/spec/frontend/index.md`
  - `.trellis/spec/frontend/api-and-dataflow.md`
  - `.trellis/spec/frontend/components-and-routing.md`
  - `.trellis/spec/frontend/quality-guidelines.md`
- **跨层与评审规范**：
  - `.trellis/spec/guides/cross-layer-thinking-guide.md`
  - `.trellis/spec/guides/review-3pass-guide.md`
