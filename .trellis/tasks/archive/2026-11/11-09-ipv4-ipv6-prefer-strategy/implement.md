# Implementation Plan — Add IPv4/IPv6 Prefer Strategy Options

## Ordered Implementation Checklist

1. **更新出站类型与序列化/回显逻辑 (`web/src/views/outbounds/types.ts`)**：
   - 在 `populateOutboundForm` 中：
     - 将 `ob.targetStrategy === 'AsIs'` 归一化为 `''`；
     - 当 `ob.protocol === 'freedom'` 时，`form.freedomDomainStrategy = s.domainStrategy || str.sockopt?.domainStrategy || (ob.targetStrategy && ob.targetStrategy !== 'AsIs' ? ob.targetStrategy : '') || 'UseIPv4'`，同时清空 `form.targetStrategy = ''` 并删除 `form.rawSockopt?.domainStrategy`；
   - 在 `sanitizeOutboundPayload` 中：当 `form.protocol === 'freedom'` 时确保 `targetStrategy: ''`。
2. **更新出站表单抽屉 (`web/src/views/outbounds/components/OutboundFormDrawer.vue`)**：
   - 在 Freedom 直连策略 `domainStrategy` (`lines 45-54`) 中新增 `UseIPv4v6`、`UseIPv6v4` 及 `ForceIP*` 选项，修正 `UseIP` / `UseIPv4` / `UseIPv6` 文案，并补充双栈回退需配合 DNS `UseIP` 的提示。
   - 在高级路由 `targetStrategy` (`lines 534-549`) 中增加 `v-if="form.protocol !== 'freedom'"`，修正 `UseIP` / `UseIPv4` / `UseIPv6` 文案，补充 `UseIPv4v6` / `UseIPv6v4` 的 `PreferIPv4/6` 中文说明，并新增 `ForceIPv4v6` / `ForceIPv6v4` 选项。
   - 在 `onOutboundProtocolChange` 中当切到 `freedom` 时清空 `form.value.targetStrategy = ''`。
3. **修正 DNS 页面查询策略文案 (`web/src/views/DNSView.vue`)**：
   - 将 `queryStrategy` (`lines 45-48`) 中 `UseIPv6` 的描述修正为 `UseIPv6 (仅查询 IPv6 AAAA 记录)`，将 `UseIP` 描述更新为 `UseIP (双栈同时查询 A/AAAA - 双栈优先回退必选)`。
4. **补充后端编译器单元测试 (`internal/adapter/xray/compiler_test.go`)**：
   - 验证 Freedom `domainStrategy: "UseIPv4v6"` 及 Outbound `targetStrategy: "UseIPv6v4"` 的编译输出并通过 `validateWithXrayCore`。
5. **执行全套质量门禁校验**。

## Validation Commands

```bash
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build .
cd web && mise x -- npm run build
```

## Risky Files / Rollback Points

- `web/src/views/outbounds/components/OutboundFormDrawer.vue`
- `web/src/views/outbounds/types.ts`
- `web/src/views/DNSView.vue`
