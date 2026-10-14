# Implementation Plan: Fix Inbound and Outbound Vision Flow Disable Bug

## Checklist

- [x] 1. **前端 Inbound 模块修复**
  - [x] 调整 `web/src/views/inbounds/types.ts`:
    - `InboundFormData` 的 `vlessFlow` 默认/空值策略，支持 `'none'`；
    - `sanitizeInboundPayload`: 不使用 `|| 'xtls-rprx-vision'` 覆盖已选的 `'none'`；
    - `buildSettingsJSON`: 规范输出 `settings.flow = clean.vlessFlow`（含 `'none'`），并在 clients 中仅注入非 `'none'` 的有效流控。
  - [x] 调整 `web/src/views/inbounds/components/InboundFormDrawer.vue`:
    - 下拉框 `<option value="none">none (无流控 - 适用于 XHTTP / gRPC / WS 等)</option>`；
    - 表单初始化/回显逻辑：当解析到 `settings.flow === 'none'` 时准确赋值为 `'none'`；
    - 联动监听函数（`onNetworkChange`, `onSecurityChange`, `onProtocolChange`）：尊重用户选择，避免将 `'none'` 强行复原。

- [x] 2. **前端 Outbound 模块修复**
  - [x] 调整 `web/src/views/outbounds/types.ts`:
    - `createDefaultOutboundForm`: 保持默认值或支持规范流控；
    - `populateOutboundForm`: 识别 `s.flow` 或 `u.flow` 为 `'none'` 或 `""` 的情况，将其准确还原为 `'none'`；
    - `buildSettingsJSON`: 顶层保存 `flow: form.vlessFlow`，`vnext.users.flow` 视非 none 写入。
  - [x] 调整 `web/src/views/outbounds/components/OutboundFormDrawer.vue`:
    - 下拉框 `<option value="none">none (无流控 - 适用于 XHTTP / gRPC / WS 等)</option>`；
    - 联动监听函数（`onOutboundSecurityChange`, `onOutboundProtocolChange`）：尊重 `'none'` 设置。

- [x] 3. **后端转换层保护与测试补充**
  - [x] 校验 `internal/protocol/node_converter.go`:
    - 当入站 `settings.flow == "none"` 时，确保订阅转换流控强制为空，不被 `user.Flow` 意外覆盖。
  - [x] 补充/更新单元测试 `internal/protocol/node_converter_test.go`。

- [x] 4. **质量验证门禁**
  - [x] 执行 `mise x -- go test ./...`
  - [x] 执行 `mise x -- go vet ./...`
  - [x] 执行 `mise x -- go build .`
  - [x] 执行 `cd web && mise x -- npm run typecheck`
  - [x] 执行 `cd web && mise x -- npm run build`
