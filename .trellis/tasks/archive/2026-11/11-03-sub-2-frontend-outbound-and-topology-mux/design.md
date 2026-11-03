# Technical Design (Sub-2: Frontend Outbound & Topology Mux)

## Key Design Decisions

1. **Non-Destructive `streamSettings` Serialization (`web/src/views/outbounds/types.ts`)**:
   - 在 `populateOutboundForm` 时保存原始 `rawSockopt`，在构建 `streamSettings` 时合并 `dialerProxy`，若既无 `dialerProxy` 也无原 `sockopt` 才省略 `sockopt`，避免前端编辑表单时丢失高级 `sockopt` 参数。
2. **Mux Serialization & Vision Guard**:
   - 若 `protocol === 'vless' && form.vlessFlow === 'xtls-rprx-vision'`，UI 层在用户开启 `muxEnabled` 时自动提示并提供一键将 `vlessFlow` 切换为无流控 (`''`) 的交互，并在序列化时兜底防止同时输出 `xtls-rprx-vision` + `mux.enabled: true`。
