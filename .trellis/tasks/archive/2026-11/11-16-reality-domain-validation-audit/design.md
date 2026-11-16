# Reality 探测与校验优化技术设计（Design）

## 1. 架构分层与改动边界
遵循项目分层边界：
- `internal/adapter/reality/prober.go`：负责网络拨号、TLS 握手、系统根证书链校验（`x509.VerifyOptions`）与轻量 HTTP/1.1 & H2 状态码/响应头采集，将原始观测字段写入 `ProbeResult`。
- `internal/domain/reality_evaluator.go`：纯业务逻辑，无外部副作用。定义评估输入结构体与错误类型常量，按确定性优先级输出 `status / errorType / details`。
- `internal/service/reality_monitor_service.go`：将 `ProbeResult` 映射为 domain 评估输入并调用评估函数。
- `internal/adapter/xray/compiler.go`：在编译 `realitySettings` 时校验 `serverNames` 不含 `*` 通配符。
- `web/src/views/inbounds/components/InboundFormDrawer.vue`：替换 `gateway.icloud.com` placeholder，补充选站提示。

## 2. 关键接口与数据结构设计

### 2.1 `ProbeResult`（`internal/adapter/reality/prober.go`）
在现有 `ProbeResult` 基础上新增字段：
```go
type ProbeResult struct {
    TCPError      error
    TLSVersion    uint16
    CurveID       tls.CurveID
    ALPN          string
    Cert          *x509.Certificate
    CertVerifyErr error
    StatusCode    int
    Location      string
    Headers       http.Header
    LatencyMs     int64
}
```
- `CurveID`：直接取自 `tlsState.CurveID`。
- `CertVerifyErr`：当 `len(tlsState.PeerCertificates) > 0` 时，构造 `Intermediates` 池（`PeerCertificates[1:]`）并调用 `tlsState.PeerCertificates[0].Verify(x509.VerifyOptions{DNSName: sni, Intermediates: pool, CurrentTime: time.Now()})`；将返回 `err` 存入 `CertVerifyErr`（保持 `tlsConfig.InsecureSkipVerify = true` 以便在 TLS 1.2 或证书异常时仍能拿到证书详情用于精准报错）。
- `StatusCode` & `Location`：在 `probeHTTP2Headers` 与 `probeHTTP1Headers` 中同时返回 `(statusCode int, headers http.Header)`，提取 `headers.Get("Location")`。

### 2.2 领域评估函数与优先级（`internal/domain/reality_evaluator.go`）
新增结构化输入 `RealityProbeInput` 与评估函数 `EvaluateRealityProbeInput(in RealityProbeInput, now time.Time)`（保留现有 `EvaluateRealityProbe` 签名作为兼容包装，避免破坏既有测试调用方）：
```go
type RealityProbeInput struct {
    TCPError      error
    TLSVersion    uint16
    CurveID       tls.CurveID
    ALPN          string
    Cert          *x509.Certificate
    CertVerifyErr error
    StatusCode    int
    Headers       http.Header
    Dest          string
    ServerName    string
}
```
新增错误类型常量：
- `ErrTypeKeyExchangeUnsupported = "KEY_EXCHANGE_UNSUPPORTED"`
- `ErrTypeCertUntrusted = "CERT_UNTRUSTED"`
- `ErrTypeHighRiskDomain = "HIGH_RISK_DOMAIN"`
- `ErrTypeHTTPRedirect = "HTTP_REDIRECT"`
- `ErrTypeALPNH2Missing = "ALPN_H2_MISSING"`

**评估优先级（Hard Error 优先于 Warning）**：
1. `TCPError != nil` → `error (TCP_UNREACHABLE)`
2. `TLSVersion < tls.VersionTLS13` → `error (TLS_VERSION_LOW)`
3. `CurveID != 0 && CurveID != tls.X25519 && CurveID != tls.X25519MLKEM768` → `error (KEY_EXCHANGE_UNSUPPORTED)`
4. `Cert == nil` → `error (CERT_EXPIRED)`；`VerifyHostname(sni)` 失败 → `error (CERT_DOMAIN_MISMATCH)`；`now.After(Cert.NotAfter)` → `error (CERT_EXPIRED)`；`CertVerifyErr != nil` → `error (CERT_UNTRUSTED)`
5. `isHighRiskRealityDomain(Dest, ServerName)`（包含 `apple` / `icloud`）→ `warning (HIGH_RISK_DOMAIN)`
6. `detectCDN(Cert, Headers)`（Cloudflare / CloudFront / Akamai / Fastly）→ `warning (CDN_DETECTED)`
7. `Cert.NotAfter.Sub(now) < 7*24*time.Hour` → `warning (CERT_EXPIRING_SOON)`
8. `StatusCode` 为 `301/302/303/307/308` → `warning (HTTP_REDIRECT)`
9. `ALPN == ""` → `warning (ALPN_MISSING)`；`ALPN != "h2"` → `warning (ALPN_H2_MISSING)`
10. 其余 → `ok`

### 2.3 IPv6 `dest` 格式化（`BuildRealityCheckItem`）
使用 `net.SplitHostPort(dest)` 判断是否已含端口；若未含端口且 `port > 0`，使用 `net.JoinHostPort(dest, strconv.Itoa(port))`。

### 2.4 `compiler.go` 通配符校验
在编译 `serverNames` 时，若任意项包含 `*`（如 `*.example.com`），返回明确校验错误：`reality serverNames 不支持通配符 (*): %s`。

## 3. 风险与回退
- `CertVerifyErr` 依赖运行环境系统根证书库；若容器精简镜像缺少 CA 证书需注意文档提示，但在标准 Linux/Windows 宿主上均内置 CA。
- 保留 `EvaluateRealityProbe` 兼容签名，确保既有测试平滑迁移。
