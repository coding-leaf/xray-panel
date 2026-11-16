# Reality 域名合法性校验与面板探测优化 PRD

## 1. 背景与目标
通过调研 `xtls/xray-core@v1.260327.0` 与 `xtls/reality@v0.0.0-20260322125925-9234c772ba8f` 源码，明确了 REALITY 协议对伪装目标（`dest` + `serverNames`）的真实硬性要求与抗探测要求。对比 `xray-panel` 现有探测实现，发现存在多处**协议硬性条件漏报（假阴性）**与**伪装质量风险误报为硬故障（假阳性）**。

本任务旨在对齐 REALITY 真实协议机制，修复探测盲区、重构 `error` / `warning` 分级语义，并补齐前后端配置防呆与引导。

## 2. 需求范围（Scope）
### 2.1 后端探测与评估（`internal/adapter/reality`、`internal/domain`、`internal/service`）
1. **密钥交换曲线硬校验（P0）**：
   - 探测阶段采集 `tls.ConnectionState.CurveID`。
   - 仅当协商曲线为 `tls.X25519` 或 `tls.X25519MLKEM768` 时视为协议合法；否则判定为 `error (KEY_EXCHANGE_UNSUPPORTED)`。
2. **证书信任链校验（P0）**：
   - 使用对端 `PeerCertificates`（含中间证书）基于系统根证书校验信任链，自签名或不可信 CA 证书判定为 `error (CERT_UNTRUSTED)`。
3. **HTTP 状态码与重定向采集（P1）**：
   - 轻量 HTTP 探测保留 `StatusCode` 与 `Location` 响应头；若返回 `301`/`302`/`303`/`307`/`308` 跳转，判定为 `warning (HTTP_REDIRECT)`。
4. **ALPN `h2` 分级（P1）**：
   - `alpn == ""` 保持 `warning (ALPN_MISSING)`；若仅协商出 `http/1.1`（非 `h2`），判定为 `warning (ALPN_H2_MISSING)`。
5. **CDN 检测分级与扩展（P1）**：
   - 将 Cloudflare CDN 从 `error` 降级为 `warning (CDN_DETECTED)`（提示协议可用但 ASN 割裂且易触发 WAF，不推荐作为伪装目标），并补充常见 CDN 响应头识别（CloudFront `x-amz-cf-id`、Akamai `AkamaiGHost`、Fastly `x-served-by` 等）。
6. **高风险/大厂域名提醒（P1）**：
   - 当 `serverName` 或 `dest` 包含 `apple` 或 `icloud` 时判定为 `warning (HIGH_RISK_DOMAIN)`，提示存在 GFW 封锁与机房 ASN 割裂风险。
7. **IPv6 `dest` 显示修复（P1）**：
   - `BuildRealityCheckItem` 改用 `net.JoinHostPort` 格式化 `dest:port`，避免裸 IPv6 地址漏拼端口。

### 2.2 配置校验与前端引导（`internal/adapter/xray`、`web/`）
1. **禁止 `serverNames` 通配符（P2）**：
   - `compiler.go` 构建 REALITY 配置时拦截包含 `*` 的 `serverNames`（REALITY 运行时仅精确匹配）。
2. **前端表单示例与选站提示（P2）**：
   - `InboundFormDrawer.vue` 移除 `gateway.icloud.com` 示例，替换为中性示例 `example.com:443` / `example.com`，并补充选站建议提示（同地区/相近 ASN、非 CDN、非跳转、非 Apple/iCloud 等大厂站点）。

## 3. 验收标准（Acceptance Criteria）
- [ ] `DefaultRealityProber.Probe` 正确采集 `CurveID`、证书链校验结果、`StatusCode`、`Location`。
- [ ] `EvaluateRealityProbe`（或新评估入口）覆盖 `KEY_EXCHANGE_UNSUPPORTED`、`CERT_UNTRUSTED`、`CDN_DETECTED (warning)`、`HIGH_RISK_DOMAIN (warning)`、`HTTP_REDIRECT (warning)`、`ALPN_H2_MISSING (warning)`，且有完整单测覆盖。
- [ ] `compiler.go` 对含 `*` 的 `serverNames` 返回明确错误并有单测验证。
- [ ] `InboundFormDrawer.vue` 不再出现 `gateway.icloud.com` 推荐示例，且有清晰的选站引导。
- [ ] 全量门禁通过：`mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .`、`cd web && mise x -- npm run build`。
