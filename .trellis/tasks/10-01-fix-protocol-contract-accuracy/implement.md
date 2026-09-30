# 实施执行计划 (Implement Checklist) - 协议契约准确性与配置语义收敛

## 1. 实施步骤清单 (Ordered Checklist)

- [ ] **Step 1: 后端 SS2022 多用户加密算法防御校验**
  - 文件：`internal/adapter/xray/protocol_adapter.go`
  - 修改 `Shadowsocks2022Adapter.CompileClients`：检查并限制 method 只能为 `2022-blake3-aes-128-gcm` 或 `2022-blake3-aes-256-gcm`，非白名单报错。
  - 伴随测试：`internal/adapter/xray/account_builder_test.go` 添加非法算法（如 `2022-blake3-chacha20-poly1305`）在多用户时的拦截测试。

- [ ] **Step 2: 后端 REALITY 移除猜测推导并推行严格校验**
  - 文件：`internal/adapter/xray/compiler.go`
    - 移除 `Dest ↔ ServerNames` 互推及 `example.com` 兜底；
    - 增加严格校验：`Dest` 非空、`ServerNames` 非空且有值、`PrivateKey` 非空；缺少则返回 `fmt.Errorf("%w: ...", domain.ErrInvalidInput)`；
  - 文件：`internal/domain/reality_evaluator.go` 与 `internal/service/reality_monitor_service.go`
    - 移除 `dest ↔ serverNames` 互推；
  - 伴随测试：更新 `compiler_test.go`、`reality_evaluator_test.go`，并新增校验失败单测。

- [ ] **Step 3: 前端清除 allowInsecure 废弃字段**
  - 文件：`web/src/views/outbounds/types.ts`
    - 移除 `tlsAllowInsecure`；
    - 移除序列化中的 `allowInsecure`；
  - 文件：`web/src/views/outbounds/components/OutboundFormDrawer.vue`
    - 移除允许不安全证书复选框及响应式绑定。

- [ ] **Step 4: 前端 SS2022 算法白名单收敛与 REALITY 真实化**
  - 文件：`web/src/views/inbounds/components/InboundFormDrawer.vue`
    - 移除 SS 算法中的 `2022-blake3-chacha20-poly1305`；
    - 表单提交前增加 REALITY 必填校验（Target、SNI 域名、私钥）；
    - 修正 Socks/HTTP 原生协议提示文案为“面板当前管理策略暂未开放传输层流控与 TLS/REALITY 包装”；
  - 文件：`web/src/views/inbounds/types.ts`
    - 移除默认的 `www.example.com` 和 `www.example.com:443`；
  - 文件：`web/src/views/outbounds/components/OutboundFormDrawer.vue`
    - 移除 SS 算法中的 `2022-blake3-chacha20-poly1305`（若有）；
    - 优化 VMess/SS 不支持 REALITY 的提示文案为面板管理策略说明；
  - 文件：`web/src/views/outbounds/types.ts`
    - 移除 `realityServerName` 默认的 `www.example.com` 兜底。

- [ ] **Step 5: 全量质量门禁与回归验证**
  - 执行 `mise x -- go test ./...`
  - 执行 `mise x -- go vet ./...`
  - 执行 `mise x -- go build .`
  - 执行 `cd web && mise x -- npm run build`

---

## 2. 验证与门禁命令 (Quality Gates)

```powershell
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build .
cd web; mise x -- npm run build; cd ..
```
