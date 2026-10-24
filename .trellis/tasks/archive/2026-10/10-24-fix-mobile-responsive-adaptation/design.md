# 移动端响应式布局与交互体验全量适配 - 技术设计 (Design)

## 1. 响应式布局策略 (Responsive Architecture)

参照 `RoutingView.vue` 与 `LogsView.vue` 的成熟经验，采用“**桌面表格 (Table-First) + 移动卡片 (Card-First)**”的双模自适应架构：
- 断点统一采用 Tailwind CSS 的 `sm` (640px)；
- 桌面端：`<div class="hidden sm:block">` 渲染高密度结构化数据表格；
- 移动端：`<div class="sm:hidden space-y-2.5">` 渲染信息层次分明的触摸卡片。

## 2. 改造点清单与设计契约

### 2.1 `UserTable.vue`
- 卡片头部：多选 Checkbox、Email（带截断保护与点击查看）、状态 Badge（`已启用` / `在线` / `到期` / `已停用`）、实时在线测速指示灯；
- 节点信息行：归属入站节点 Badges 与权限分流线路统计 `(N线)`；
- 流量配额行：已用/总量格式化，附带彩色自适应进度条；
- 有效期与自动重置提示行；
- 操作按钮栏：查看详情（Inspect）、聚合分享（Share）、流量重置、编辑、删除。

### 2.2 `InboundTable.vue`
- 卡片头部：Tag、RouteID、协议 Badge、流控 Badge、端口开关/运行指示灯；
- 传输与映射行：端口（`:443` 或外部端口）、网络模式（`TCP`/`XHTTP`）、传输安全 Badge（`REALITY` / `TLS`）及域名巡检健康标记；
- 线路与累计流量行：分流线路统计与累计上/下行流量；
- 操作按钮栏：查看详情、编辑、分享、启停开关、删除。

### 2.3 `OutboundTable.vue`
- 将原 `overflow-hidden` 改为 `overflow-x-auto`，提供水平滚动容错；
- 移动端卡片流：Tag、系统默认标识、协议 Badge、传输安全模式、目标端点地址、配置摘要与操作按钮。

### 2.4 `Drawer.vue` 与 `Modal.vue`
- 底部操作栏（Footer）增加 `pb-[max(0.875rem,env(safe-area-inset-bottom))]`，适配全面屏手势条。
