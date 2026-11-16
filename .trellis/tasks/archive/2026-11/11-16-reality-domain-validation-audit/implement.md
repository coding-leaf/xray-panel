# Reality 探测与校验优化实施计划（Implement）

## 阶段拆解（单批次实施）

### Slice 1：后端探测与领域评估（Go）
1. **先行补齐/更新失败测试（Fail-Repro First）**：
   - 在 `internal/domain/reality_evaluator_test.go`（若不存在则新建）编写单测，覆盖：
     - `CurveID` 为 `tls.CurveP256` 报 `error (KEY_EXCHANGE_UNSUPPORTED)`；`tls.X25519` / `tls.X25519MLKEM768` 通过。
     - `CertVerifyErr != nil` 报 `error (CERT_UNTRUSTED)`。
     - Cloudflare / CloudFront 等 CDN 报 `warning (CDN_DETECTED)` 而非 `error`。
     - `apple` / `icloud` 域名报 `warning (HIGH_RISK_DOMAIN)`。
     - `301`/`302` 状态码报 `warning (HTTP_REDIRECT)`。
     - `ALPN = "http/1.1"` 报 `warning (ALPN_H2_MISSING)`。
     - 裸 IPv6 `dest`（如 `2606:4700::1`）在 `BuildRealityCheckItem` 中正确格式化为 `[2606:4700::1]:443`。
   - 在 `internal/adapter/xray/compiler_test.go`（或对应测试文件）补充 `serverNames` 含 `*` 通配符的失败单测。
2. **修改 `internal/adapter/reality/prober.go`**：
   - `ProbeResult` 增加 `CurveID`、`CertVerifyErr`、`StatusCode`、`Location`。
   - 在 `Probe` 中采集 `tlsState.CurveID`，使用 `PeerCertificates` 执行 `x509.Verify` 写入 `CertVerifyErr`。
   - 更新 `probeHTTP2Headers` / `probeHTTP1Headers` 返回状态码与响应头。
3. **修改 `internal/domain/reality_evaluator.go`**：
   - 新增常量与 `RealityProbeInput`、`EvaluateRealityProbeInput`。
   - 调整 `CDN_DETECTED` 为 `warning` 并扩展常见 CDN 响应头匹配；新增 `HIGH_RISK_DOMAIN`、`HTTP_REDIRECT`、`ALPN_H2_MISSING`、`KEY_EXCHANGE_UNSUPPORTED`、`CERT_UNTRUSTED`。
   - 修复 `BuildRealityCheckItem` IPv6 端口拼接。
4. **修改 `internal/service/reality_monitor_service.go`**：
   - 将 `ProbeResult` 完整传入 `EvaluateRealityProbeInput`。
5. **修改 `internal/adapter/xray/compiler.go`**：
   - 拦截 `serverNames` 中的 `*` 通配符。

### Slice 2：前端表单示例与提示（Vue）
1. 修改 `web/src/views/inbounds/components/InboundFormDrawer.vue`：
   - 将 `gateway.icloud.com:443` / `gateway.icloud.com` 替换为 `example.com:443` / `example.com`。
   - 添加简明选站提示（推荐同地区/相近 ASN、非 CDN、非跳转、支持 TLS 1.3 + X25519 + H2 的网站，避免 Apple/iCloud 与公共 CDN）。

### 验证门禁
- `mise x -- go test ./...`
- `mise x -- go vet ./...`
- `mise x -- go build .`
- `cd web && mise x -- npm run build`
