# Technical Design (Sub-3: Frontend Inbound & Routing Alignment)

## Key Design Decisions

1. **Inbound `streamSettings` & `sniffing` Non-Destructive Merge**:
   - 解析入站 `streamSettings` 时提取 `sockopt.acceptProxyProtocol` 并保留其他 `sockopt` 字段；序列化时若 `acceptProxyProtocol === true` 或存在其他 `sockopt` 字段则输出 `sockopt`。
   - 解析入站 `sniffing` 时读取 `metadataOnly`、`domainsExcluded`、`ipsExcluded`，序列化时仅在非空时输出对应数组/布尔字段。
2. **Routing Rule Form Validation**:
   - 允许仅配置 `source` 或 `sourcePort` 作为路由匹配条件（与后端 `hasMatchCondition` 保持一致）。
