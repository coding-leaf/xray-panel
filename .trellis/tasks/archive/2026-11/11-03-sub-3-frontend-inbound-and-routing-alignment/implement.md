# Implementation Plan (Sub-3)

## Checklist

1. [x] 更新 `web/src/views/inbounds/types.ts`、`InboundFormDrawer.vue`、`InboundDetailDrawer.vue`：支持 `acceptProxyProtocol` 与 Sniffing 扩展字段。
2. [x] 更新 `web/src/views/RoutingView.vue`：支持 `source`、`sourcePort`、`domainMatcher` 的编辑、校验与列表标签渲染。

## Validation Commands

```bash
cd web && mise x -- npm run build
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build .
```
