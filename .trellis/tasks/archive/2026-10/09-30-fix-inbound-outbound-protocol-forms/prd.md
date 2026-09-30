# 前端入站出站协议匹配与参数表单规范化重构

## Goal

针对 Xray 官方各核心协议规范，系统性补全与重构前端入站 (`InboundFormDrawer.vue`) 和出站 (`OutboundFormDrawer.vue`) 在创建/编辑时的协议专属表单项、安全与传输层互斥联动、以及数据序列化/清洗逻辑，杜绝非法组合与脏字段下发导致 Xray-core 启动异常。

---

## Background & Historical Context

此前重构主要覆盖了：
1. 后端 `internal/adapter/xray` 与 `internal/protocol` 的解耦，支持了 SS2022、VLESS Vision、Trojan 等；
2. 前端 `OutboundsView.vue` 与 `UsersView.vue` 的巨石拆解（拆分为 Table、Drawer 组件）。
但历史拆解过程中，前端表单组件只保留了 VLESS/Reality 的详细输入控件，对其他协议（Socks、HTTP、dokodemo-door、Shadowsocks 密码/加密、VMess AlterId/Security）**在 UI 模板层未完整渲染对应表单字段**，且序列化函数 `buildSettingsJSON` / `buildStreamSettingsJSON` 存在脏数据泄漏。

---

## Detailed Requirements

### 1. 入站节点协议表单补全与联动 (`InboundFormDrawer.vue` & `inbounds/types.ts`)
- **协议专属表单项呈现**：
  - **Socks**：提供认证方式（`noauth` / `password`）、用户名、密码、UDP 支持开关；
  - **HTTP**：提供可选认证用户名与密码；
  - **dokodemo-door**：提供转发目标地址 (`dokoAddress`)、转发目标端口 (`dokoPort`)、协议网络 (`tcp`, `udp`, `tcp,udp`)；
  - **Shadowsocks / SS2022**：提供加密方式选择（`2022-blake3-aes-128-gcm`、`2022-blake3-aes-256-gcm`、`aes-128-gcm`、`chacha20-poly1305` 等）与服务密码设置（支持与关联用户 Sub-Key 联动或单用户服务密码）；
  - **Trojan / VMess**：关联授权用户，并在底层正确序列化为 Trojan `{ password, email }`，VMess `{ id, email }`。
- **传输与安全层严格约束**：
  - Socks、HTTP、dokodemo-door 为应用层/端口层协议，自动隐藏或锁定 `network=tcp, security=none`，禁用 REALITY/TLS/XHTTP 配置；
  - REALITY 选项严格限定在支持的场景（如 VLESS TCP/XHTTP/gRPC）；VMess / Socks / HTTP 下禁止选择 REALITY；
  - 流控（Flow xtls-rprx-vision）严格仅在 VLESS + TCP + TLS/REALITY 下生效并允许配置，其他协议清空。
- **数据序列化与清洗 (`buildSettingsJSON` / `sanitizeInboundPayload`)**：
  - Trojan 输出正确的 `clients: [{ password, email, level: 0 }]`；
  - Socks、HTTP、dokodemo-door 序列化对应的 settings 结构，且其 `streamSettings` 清洗为合法或空对象，杜绝携带 XHTTP / Reality 字段。

### 2. 出站节点协议表单补全与联动 (`OutboundFormDrawer.vue` & `outbounds/types.ts`)
- **流控与传输安全层清洗修复**：
  - 修复 `buildStreamSettingsJSON`：不仅排除 `freedom`、`blackhole`、`wireguard`，对 `socks` 和 `http` 出站同样禁止写入 `streamSettings`（或清空为 `""`），防止残留 `reality` 脏参数导致核心崩溃；
  - VMess / Shadowsocks 出站禁止选择 REALITY（限制为 TLS / None）；
  - 增加对常用 `dns` 出站协议的支持（tag: dns-out，无额外 settings，用于 DNS 路由转发）。
- **字段双向回填 (`populateOutboundForm`) 健壮性**：
  - 正确解析各协议已保存的 `settingsJson` 与 `streamSettings`，编辑时无缝还原表单状态。

### 3. 类型安全与规范门禁
- 保持前端 TypeScript 类型严格检查，无 `any` 泛滥与未声明属性；
- UI 交互契约与现有设计系统规范一致，复用 `SectionCard`、`FormField`、`Input`、`Button` 组件。

---

## Acceptance Criteria

- [x] **入站表单全协议覆盖**：在 InboundFormDrawer 中切换 VLESS、VMess、Trojan、Shadowsocks、Socks、HTTP、dokodemo-door 时，对应专属参数配置区正确动态显示，并能正常输入与校验。
- [x] **安全/传输层互斥联动正确**：Socks/HTTP/dokodemo-door 不显示 REALITY/XHTTP；VMess/Socks 不可勾选 REALITY；VLESS Vision 仅在 TCP + TLS/REALITY 时生效。
- [x] **出站表单及清洗无脏数据**：新建 Socks/HTTP/Freedom/Blackhole/WireGuard 出站时，生成的 `streamSettings` 保持为空，不会附带多余的 REALITY/XHTTP 配置。
- [x] **数据回填与反向解析正确**：编辑已有出站/入站节点时，所有专属字段均能准确恢复。
- [x] **质量门禁全绿**：
  - `cd web && mise x -- npm run build`（包含 TypeScript 检查与 Vite 生产打包）100% 成功，零错误；
  - 后端测试 `mise x -- go test ./...` 100% 通过；
  - 静态检查 `mise x -- go vet ./...` 零警告。
