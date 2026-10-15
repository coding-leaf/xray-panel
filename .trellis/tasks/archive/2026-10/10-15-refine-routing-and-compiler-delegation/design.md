# 技术设计方案 (Technical Design)

## 1. 架构定位与设计哲学

- **KISS 极简规范与反过度设计**：
  系统坚持“机制极简中立、策略完全放权”。后端编译器仅保障系统核心正常运行与数据面语法合规，绝不越俎代庖替用户做业务阻断或特定域名的特殊兜底（严禁在编译器中硬编码 25 端口、BT 协议、特定大厂域名补丁）。
- **忠实编译器（Faithful Compiler）**：
  用户在 Web 面板维护的规则列表是第一事实源。编译器忠实按序输出，废除静默丢弃自定义字段（如 `vlessRoute`）的黑盒行为。

---

## 2. 核心问题与设计方案

### 2.1 根因消除：废除 `vlessRoute` 自定义规则过滤
在 `internal/adapter/xray/compiler.go` 中，原逻辑：
```go
// 过滤掉已被自动管理的 api 规则与 vlessRoute 规则
if len(r.InboundTag) > 0 && r.InboundTag[0] == "api" {
    continue
}
if r.VlessRoute != "" {
    continue // ❌ 严重断层：导致用户前端配置的 vlessRoute 规则被静默丢弃
}
```
**改进设计**：
- 仅跳过内部 `api` 规则（因为 Layer 1 已经强制编排了 `api` 通道）。
- 彻底移除 `if r.VlessRoute != "" { continue }`，将用户配置的 `VlessRoute` 原样映射到 `XrayRoutingRule.VlessRoute`。
- 允许用户自由设定：
  1. 节点级穿透规则：如 `{ vlessRoute: "2", outboundTag: "out-us" }`，排在全局分流前，彻底免疫本地规则截流；
  2. 节点级定制分流：如 `{ vlessRoute: "1", domain: ["geosite:cn"], outboundTag: "warp-out" }`，使分流规则精准绑定目标节点。

### 2.2 编排层次结构 (Compiler Layering Hierarchy)

```text
Layer 1: 核心系统管理规则 (不可变)
  └── inboundTag: ["api"] -> outboundTag: "api" (gRPC 控制面通信)

Layer 2: 用户自定义规则池 (严格按用户前端编排顺序输出)
  ├── 规则 1 (如用户自定义 vlessRoute: 2 -> out-us 直通隧道)
  ├── 规则 2 (如用户点击一键向导生成的 bittorrent -> block)
  ├── 规则 3 (如用户点击一键向导生成的 geosite:cn -> direct)
  └── ... (保留用户设定的 domain, ip, port, network, protocol, vlessRoute, inboundTag)

Layer 3: 接入网关专属通道兜底规则 (SubRoute Auto Fallbacks)
  └── 遍历所有 Inbound 的 SubRoutes (sr.Enabled && sr.RouteID > 0)
      生成 inboundTag: [inb.Tag], vlessRoute: sr.RouteID -> sr.OutboundTag (含 AllowedUsers)
```

### 2.3 前端一键向导（Wizards / Presets）设计
- 纯客户端工具函数，不向后端发送硬编码请求，不修改数据库默认初始化。
- 位于 `web/src/views/RoutingView.vue`，向导弹窗中清晰展示将要生成的规则明细与预选出口。
- 点击“确认添加”后，将规则 push 到当前 `rules` 列表中，用户可继续调整排序或编辑字段，点击“保存更改”统一提交。

---

## 3. 边界与契约 (Contracts & Compatibility)

- **API 兼容性**：
  `domain.RoutingRule` 结构已原生包含 `VlessRoute string` 字段，且 `compiler.go` 的 `XrayRoutingRule` 也已定义 `VlessRoute` 字段。改动对数据库模型、REST API 完全向后兼容，无需迁移数据库 Schema。
- **Xray 内核兼容性**：
  Xray-core 官方原生支持 `routing.rules[].vlessRoute`，编译生成的 JSON 可直接通过 `xray -test -config` 语法检查。

---

## 4. 容灾与回滚 (Rollback Strategy)

- 若修改引起分流非预期行为，因规则持久化在 `settings` 表的 `routing` 字段中，通过面板直接删除或重新排序自定义规则即可立即恢复；
- 代码级别变更仅涉及 `compiler.go` 与 `compiler_test.go`，Git 提交原子化，随时支持单 commit 回滚。
