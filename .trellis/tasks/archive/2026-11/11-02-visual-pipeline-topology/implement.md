# 三栏动态脉冲连线可视化拓扑画布 - 实施计划 (Implement Plan)

## Ordered Implementation Checklist

### 1. 基础模型与拓扑推导引擎 (Phase 2 - Step 1)
- [x] 1.1 在 `web/src/views/topology/types.ts` 中定义完备的 TypeScript 接口：
  - `TopologyNode`、`TopologyEdge`、`TopologyGraph`、`LinkStatus`、`ChannelCategory` 等；
  - 交互状态模型：`HoverHighlightState`、`ActivePathSet` 等。
- [x] 1.2 在 `web/src/views/topology/composables/useTopologyGraph.ts` 中实现全协议流向推导核心：
  - 处理 `/inbounds`、`/outbounds`、`/routing`、`/users` 原始输入；
  - 实现 Path 1（VLESS SubRoute 线路展开与映射）；
  - 实现 Path 2（Scoped 规则与 Global 通用规则匹配与通道抽取）；
  - 实现 Path 3（默认 `direct` 兜底直出机制，消除孤岛网关）；
  - 计算右栏出口节点的 `referenceCount`，标识 `isIdle` 闲置状态；
  - 生成双向邻接索引（`upstreamMap`, `downstreamMap`, `nodeEdgesMap`）。

### 2. SVG 几何计算与动画调度 (Phase 2 - Step 2)
- [x] 2.1 在 `web/src/views/topology/composables/useSvgBezier.ts` 中实现：
  - 基于 DOM 引用的容器与卡片边界测量；
  - 左右锚点在画布坐标系中的转换算法；
  - 三次贝塞尔曲线路径字符串生成器；
  - 基于 `ResizeObserver` 与 `requestAnimationFrame` 的防抖连线重算。

### 3. 组件解耦与原子卡片开发 (Phase 2 - Step 3 ~ Step 5)
- [x] 3.1 抽离 `web/src/views/topology/components/QuickPublishWizardModal.vue`：
  - 将原 `TopologyView.vue` 中的 1 步发布落地向导弹窗与链接解析逻辑完全解耦封装；
  - 严格校验协议解析（`vless://`, `trojan://`, `ss://`）；
  - 保留自动计算 `nextRouteId` 与直接落盘能力。
- [x] 3.2 抽离 `web/src/views/topology/components/TopologyPoolsView.vue`：
  - 完整迁移原有的分流通道/接入网关/落地出口三 Tab 卡片列表视图；
  - 保留原有的一键暂停、删除、跳转编辑等交互，保障卡片池模式 100% 保真。
- [x] 3.3 实现三栏节点卡片组件：
  - `InboundNodeCard.vue`：左栏网关卡片（协议徽标、存活指示灯、端口、延迟、流量、日志直达与快捷操作）；
  - `ChannelNodeCard.vue`：中栏通道卡片（SubRoute/专属规则/全局规则/兜底直连状态、RouteID、授权用户数、原位启停与删除）；
  - `OutboundNodeCard.vue`：右栏出口卡片（协议徽标、目标端点摘要、被引用计数、闲置高亮徽标与一键发布入口）。

### 4. 画布组装与动态 SVG 脉冲层 (Phase 2 - Step 6)
- [x] 4.1 开发 `web/src/views/topology/components/TopologyCanvas.vue`：
  - 组装桌面三栏 Grid 布局（左：接入网关、中：分流通道、右：落地出口）；
  - 渲染绝对定位的全尺寸 SVG 图层，渲染 Cubic Bezier 贝塞尔曲线；
  - 实现链路状态样式渲染：
    - 启用链路：双层 SVG（底层静态虚线 + 顶层发光粒子脉冲流动）；
    - 暂停链路：暗灰色静态虚线；
    - 阻断链路（block/blackhole）：暗红色拦截虚线；
  - 实现悬停双向溯源高亮：
    - 鼠标进入卡片时计算全通路径（`activeNodeIds` & `activeEdgeIds`）；
    - 非关联节点与线条平滑应用 `opacity-25`；
  - 实现移动端响应式降级：
    - 屏幕 `< lg` 时自动隐藏 SVG 浮层；
    - 切换为紧凑的三级流式流水线卡片展示。

### 5. 主视图重构与调度整合 (Phase 2 - Step 7)
- [x] 5.1 重构 `web/src/views/TopologyView.vue`：
  - 顶层引入 Header 视图模式切换器：`拓扑画布 (Canvas Graph)` 与 `传统分组池 (Card Pools)`；
  - 并发拉取 `/inbounds`、`/outbounds`、`/routing` 与 `/users`；
  - 接入 `TopologyCanvas` 与 `TopologyPoolsView`；
  - 接入 `QuickPublishWizardModal` 响应全局发布与节点卡片唤起；
  - 完善通道启停、删除及外部路由跳转处理。

### 6. 全量验证与质量门禁 (Phase 2 - Step 8 ~ Step 9)
- [x] 6.1 前端静态类型与生产构建门禁：
  - `cd web && mise x -- npm run typecheck`
  - `cd web && mise x -- npm run build`
- [x] 6.2 后端质量门禁保障：
  - `mise x -- go test ./...`
  - `mise x -- go vet ./...`
  - `mise x -- go build .`
- [x] 6.3 交互走查验证：
  - 验证 Mock 模式下（`npm run dev:demo`）三栏连线与粒子动画流畅度；
  - 验证不同屏幕断点下的移动端自适应表现；
  - 验证悬停高亮、原位启停通道、删除通道以及 1 步发布向导全流程。

---

## 质量门禁与验证命令

在 Phase 2 实施完毕提交前，必须在终端执行并通过全部质量门禁：

```bash
# 1. 前端类型检查 (严格零错误)
cd web && mise x -- npm run typecheck

# 2. 前端 Vite 生产构建打包 (验证产物输出与 chunk 依赖)
cd web && mise x -- npm run build

# 3. 后端单元测试 (必须全绿)
mise x -- go test ./...

# 4. 后端静态检查 (无 warning)
mise x -- go vet ./...

# 5. 后端完整嵌入式构建 (确保 web/dist 正确打入单一二进制产物)
mise x -- go build .
```

---

## 回滚与防御策略 (Rollback & Safety Plan)

1. **零业务逻辑破坏与零侵入性**：
   - 本次改造仅在前端表现层进行，不更改任何后端 API 接口契约与数据库持久化格式；
   - 所有原有的入站、出站、通道管理逻辑完全保留。
2. **卡片池管理 100% 保真回退**：
   - 提取的 `TopologyPoolsView.vue` 完整承接存量功能；如果用户不习惯画布模式或遇到特定低版本浏览器兼容问题，可随时点击顶部切换至卡片池视图，不影响任何日常运维。
3. **Git 原子回滚**：
   - 若遇到不可预见的渲染性能或架构阻塞，可通过 Git 单 commit 原子回滚 `web/src/views/topology/` 与 `TopologyView.vue`。
