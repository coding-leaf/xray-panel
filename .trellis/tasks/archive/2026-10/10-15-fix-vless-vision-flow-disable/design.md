# Technical Design: Fix Inbound and Outbound Vision Flow Disable Bug

## 1. 架构拓扑与数据流 (Data Flow & Architecture)

### 1.1 Inbound 数据流
```text
[InboundFormDrawer.vue]
  vlessFlow: 'none' | 'xtls-rprx-vision' | 'xtls-rprx-vision-udp443'
        │
        ▼
[sanitizeInboundPayload]
  保持 clean.vlessFlow:
  - 若 network === 'tcp' && ['reality', 'tls'].includes(security): clean.vlessFlow = raw.vlessFlow || 'none'
  - 否则: clean.vlessFlow = 'none'
        │
        ▼
[buildSettingsJSON]
  settings.flow = clean.vlessFlow (例如 "none" 或 "xtls-rprx-vision")
  clients: 仅当 flowVal !== 'none' 时注入 client.flow = flowVal；为 "none" 时不注入 flow 字段
        │
        ▼
[后端 DB Inbound.SettingsJSON]
  `{"flow":"none", ...}`
        │
        ▼
[ResolveVisionFlow / node_converter.go / 回显]
  后端识别 customFlow == "none" -> flow = "" (编译 Xray 运行时 client flow 为空，订阅链接不附带 flow)
  前端 InboundFormDrawer 回显: settings.flow === 'none' -> form.vlessFlow = 'none' -> 匹配 <option value="none">
```

### 1.2 Outbound 数据流
```text
[OutboundFormDrawer.vue]
  vlessFlow: 'none' | 'xtls-rprx-vision' | 'xtls-rprx-vision-udp443'
        │
        ▼
[buildSettingsJSON]
  - 顶层写入 flow: flowVal (若关闭写入 "none")
  - vnext[0].users[0].flow: 若 flowVal === 'none' 则写入 ""，否则写入 flowVal
        │
        ▼
[后端 DB Outbound.SettingsJSON]
  `{"flow":"none", "vnext":[{"users":[{"id":"...","encryption":"none","flow":""}]}]}`
        │
        ▼
[populateOutboundForm 回显]
  优先取 s.flow 或 u.flow：若为 "none" 或 ""，设置 form.vlessFlow = 'none'
  匹配 <option value="none">，正确回显关闭状态
```

## 2. 影响范围与契约保证 (Compatibility & Contracts)

- **Xray-core 协议兼容性**：
  - Inbound: Xray 运行时由 `internal/adapter/xray` 编译，`CompileClients` 读取 `ResolveVisionFlow`。当 `flow: "none"` 时返回空串，Xray 客户端账号无 flow 字段，完全合规。
  - Outbound: Xray-core 原生解析 `settings.vnext[].users[].flow`。空串 `""` 即代表不启用流控。顶层 `flow` 字段为面板自解释元数据，Xray 序列化忽略，绝不破坏 Xray 启动。
- **历史数据向前兼容**：
  - 对于未包含 `flow` 字段的历史入站/出站配置，回显逻辑在缺失时若为 VLESS TCP REALITY 仍保持默认 `'xtls-rprx-vision'`，不影响存量节点。

## 3. 涉及改动文件
- `web/src/views/inbounds/types.ts`
- `web/src/views/inbounds/components/InboundFormDrawer.vue`
- `web/src/views/outbounds/types.ts`
- `web/src/views/outbounds/components/OutboundFormDrawer.vue`
- `internal/protocol/node_converter.go`
- `internal/protocol/node_converter_test.go`
