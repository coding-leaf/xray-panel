# 精细化路由编排与编译器放权支持

## Goal

废除 `compiler.go` 中的硬编码过滤，支持 `vlessRoute` 自定义规则编译；遵循“机制极简中立、策略完全放权”原则，系统默认保持规则池纯净，将通用安全（25 端口、BT、私有 IP）与分流策略转为纯 UI 一键向导，彻底解决跨机节点隧道（US 出口等）在入口机被粗暴截流导致出口分裂与风控的问题。

## Requirements

1. **编译器彻底放权与忠实编译（Zero Hardcoding & Faithful Compilation）**
   - 移除 `internal/adapter/xray/compiler.go` 中对 `r.VlessRoute != ""` 的静默丢弃逻辑（原 line 178 `continue`），允许用户在自定义规则中指定 `vlessRoute`。
   - 保持 Layer 1 极简中立：仅保留 Xray 进程内 gRPC 交互所必需的 `api` 规则（`inboundTag: ["api"] -> outboundTag: "api"`），绝不硬编码任何 25 端口封锁、BT 阻断、私有 IP 或域名分类。
   - 保持用户规则顺序忠实度：用户在面板配置的规则序列按照 First-Match 原则忠实映射至 Xray `routing.rules`。
   - SubRoute 自动生成的兜底规则与自定义带 `vlessRoute` 规则和谐共存，确保跨机节点隧道流量不会被入口机粗暴的全局无作用域规则截胡。

2. **零初始强加（Clean Default State）**
   - 系统初始化、空配置或重置场景下，严禁自动隐式注入任何业务阻断或分流规则。

3. **前端通用一键向导工具化（On-Demand UI Wizards）**
   - 在 `web/src/views/RoutingView.vue` 中提供通用一键预设向导（仅作为辅助填充工具，用户点击确认后方追加为普通规则）：
     - 🛡️ 广告与追踪拦截向导（`geosite:category-ads-all -> block`）
     - 🔒 局域网私有 IP 屏蔽向导（`geoip:private -> block`）
     - 🚫 BT/BitTorrent 拦截向导（`bittorrent -> block`）
     - 📧 邮件 25 端口封锁向导（`port: 25 -> block`）
     - ⚡ 国内流量分流向导（`geosite:cn`、`geoip:cn`，可自由指定出站与调整作用域）
   - 所有生成的规则均为普通规则，支持自由拖拽排序、就地编辑（包括 `vlessRoute` 绑定）和随时删除。

## Acceptance Criteria

- [x] `internal/adapter/xray/compiler.go` 不再静默过滤 `r.VlessRoute`，包含 `vlessRoute` 的自定义规则被忠实编译至 Xray 配置。
- [x] `internal/adapter/xray/compiler.go` 无任何硬编码的端口 25、BT 协议、私有 IP 或特定域名拦截。
- [x] 后端单元测试全面覆盖包含 `vlessRoute` 自定义规则与 SubRoute 兜底规则的编排逻辑，`mise x -- go test ./...` 100% 通过。
- [x] `mise x -- go vet ./...` 零 warning，`mise x -- go build .` 构建成功。
- [x] 前端 `RoutingView.vue` 支持输入和展示带 `vlessRoute` 的规则，一键向导正确生成规则并可正常持久化。
- [x] 前端生产构建 `cd web && mise x -- npm run build` 100% 成功。
