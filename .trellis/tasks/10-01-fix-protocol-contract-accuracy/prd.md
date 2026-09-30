# 需求规范 (PRD) - 修复协议契约准确性与配置语义

## 1. 背景与目标 (Background & Goals)
在近期架构重构后，系统底层分层基本清晰，但前后端协议契约存在语义模糊、废弃字段残留与过度自动猜测等问题：
1. **文案误导**：前端将“面板暂未开放的功能策略”错误归咎为“Xray-core 原生不支持”；
2. **废弃字段风险**：前端生成已废弃的 `tlsSettings.allowInsecure=true`，可能引发新版 Xray-core 校验失败；
3. **非法密码学套件**：SS2022 Multi-user 仅支持 AES-128/256-GCM，前端误选 `2022-blake3-chacha20-poly1305` 导致核心加载失败；
4. **REALITY 过度补全**：前后端仍在使用 `example.com` 占位符静默兜底，并在 `dest ↔ serverNames` 间自动互推，掩盖配置缺失。

本次改造目标是**全面收敛协议契约**：清理废弃字段、修正语义定性、收紧合法密码学组合、根除假兜底，实现“严格校验、明确报错、忠实配置”。

---

## 2. 需求列表 (Requirements)

### R1. 文案与限制定性修正
- **R1.1**：InboundFormDrawer 与 OutboundFormDrawer 中，针对 Socks/HTTP 暂无 streamSettings、VMess/Shadowsocks 暂未搭配 REALITY 等策略，提示文案必须明确注明为“面板当前管理策略暂未开放/暂仅支持原生网络监听”，严禁写成“Xray 内核不支持”。
- **R1.2**：清理相关源码内误导性的注释文本。

### R2. 废弃字段 `allowInsecure` 彻底清理
- **R2.1**：前端出站表单 `OutboundFormDrawer.vue` 彻底移除“允许不安全证书 (allowInsecure)”勾选项。
- **R2.2**：`web/src/views/outbounds/types.ts` 移除 `tlsAllowInsecure` 字段模型及其在序列化中的 `allowInsecure` 输出。
- **R2.3**：后端生成给 Xray-core 的 StreamSettings/TLS 结构严禁包含 `allowInsecure`。

### R3. SS2022 多用户加密算法收敛
- **R3.1**：前端入站配置（`InboundFormDrawer.vue`）中，Shadowsocks 加密算法选项移除 `2022-blake3-chacha20-poly1305`，仅保留合法的多用户套件：`2022-blake3-aes-128-gcm` 与 `2022-blake3-aes-256-gcm`（以及传统单用户套件）。
- **R3.2**：后端 `Shadowsocks2022Adapter` 增加防御性校验：若处于 multi-user 模式（即编译 clients），若配置的算法非 `2022-blake3-aes-128-gcm` 或 `2022-blake3-aes-256-gcm`，必须显式抛出校验错误。

### R4. REALITY 剔除假兜底与互推，推行严格校验
- **R4.1**：后端 `compiler.go` 彻底移除 `dest ↔ serverNames` 互推逻辑，彻底移除 `www.example.com:443` 与 `www.example.com` 的静默兜底。
- **R4.2**：后端 `compiler.go` 在编译 Inbound REALITY 时执行严格校验：`dest` 必须非空且包含有效端口，`serverNames` 必须非空且至少有一项，`privateKey` 必须非空。缺失任一项时直接报错拒绝编译。
- **R4.3**：后端 `reality_evaluator.go` 与 `reality_monitor_service.go` 移除 `dest ↔ serverNames` 的互推回退。
- **R4.4**：前端 `inbounds/types.ts` 与 `outbounds/types.ts` 彻底移除 `example.com` 兜底；新建表单默认值置空；前端保存前对必填字段进行强校验阻断并给出友好的 Toast 提示。

---

## 3. 验收标准 (Acceptance Criteria)

- [ ] **AC1 (文案定位)**：前端表单提示准确反映面板策略，无“Xray 不支持”误导性表述。
- [ ] **AC2 (清理 allowInsecure)**：前端出站表单不再显示 `allowInsecure` 开关，生成的出站 JSON 中无 `allowInsecure` 字段。
- [ ] **AC3 (SS2022 加密限制)**：
  - 前端入站 SS 算法下拉列表无 `2022-blake3-chacha20-poly1305`；
  - 后端 SS2022 编译在多用户模式下配置非 AES 算法时返回明确 error。
- [ ] **AC4 (REALITY 真实性与严格校验)**：
  - 前后端无任何 `example.com` 缺省 fallback 代码；
  - Inbound 开启 REALITY 时，若未填写 dest 或 serverNames 或 privateKey，前端阻断提交，后端编译报错；
  - 后端原有单元测试与新增校验测试全绿；
- [ ] **AC5 (全套门禁)**：`go test ./...`、`go vet ./...`、`go build .`、`npm run build` 全部无 warning/error 通过。
