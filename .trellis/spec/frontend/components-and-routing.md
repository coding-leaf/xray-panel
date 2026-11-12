# Components & Routing

> SFC 约定、路由表与导航守卫、样式与图标体系

---

## SFC 约定与组件分层

- 所有 `.vue` 一律使用 `<script setup lang="ts">`，**禁止 Options API**。
- **组件分层**：
  - **原子组件（`web/src/components/ui/`）**：强类型通用原语（如 `Button`, `Input`, `Badge`, `Drawer`, `Modal`, `Table` 系列，以及 `FormField`, `SectionCard` 容器），严格使用 `defineProps` 约束变体、尺寸与状态，基于 CSS 变量语义令牌，负责收敛 Label/Hint/Error 与居中/抽屉浮层深度，禁止散落内联 Hex 颜色。
  - **业务领域子组件（`web/src/views/<feature>/components/`）**：针对 50KB+ 的庞大视图，按领域就近聚合拆分为子组件（如 `InboundTable`, `UserTable`, `OutboundTable`, `OutboundDetailDrawer`, `OutboundFormDrawer`）；重型表单抽屉与大型弹窗（如二维码、趋势图）采用 `defineAsyncComponent` 异步懒加载以优化首屏 chunk 体积。
  - **业务调度视图（`web/src/views/`）**：遵循自包含与轻量化原则，主视图作为调度器协调过滤状态、表格与双抽屉，局部类型与参数清洗收敛至 `views/<feature>/types.ts`。
- 页面协作与模块解耦：跨层单例协作继续通过导入模块单例（`toast`、`api`）完成。
- 生命周期配合清理与轮询防重入：`onMounted` 注册 `setInterval` 或异步进度定时器（如 GeoData 更新 `progressTimer`），`onUnmounted` 必须成对清理并处理 `visibilitychange`；所有 `setInterval` 异步回调必须设置局部布尔 `inFlight`（`try/finally`）防重入守卫，杜绝慢网络高延迟下的并发请求叠加。
- 高行数响应式表格单套挂载：对于最大可达千行的视图（如 `LogsView.vue`），禁止使用 CSS `hidden md:block` / `md:hidden` 同时挂载桌面与移动两套完整列表 DOM，应通过 `window.matchMedia` 响应式断点（`v-if="isDesktop"` / `v-else`）仅渲染当前视口形态并绑定稳定行 `:key`。
- 样式主要全部走 Tailwind class 与 Design Tokens 语义类。

---

## 样式体系与 Design Tokens 规范

- Tailwind CSS + PostCSS，入口 `web/src/style.css`。
- **开发者基础设施控制台设计令牌 (Neutral Design Tokens)**：
  - 基于 CSS 语义变量（`--background`, `--foreground`, `--card`, `--muted`, `--border`, `--accent`）；
  - 全站禁止毛玻璃 `glass-card`、`backdrop-blur` 和彩色发光渐变；
  - 严禁随意内联 Hex 色值，一律收敛至 `bg-background`、`bg-card`、`border-border` 等语义令牌；
  - 圆角统一收敛：控件 `6px` (`rounded-md`)，容器/面板 `8px` (`rounded-lg`)，抽屉/弹窗 `10px/12px`；
  - 语义功能色严格收敛：仅允许绿 (正常/运行)、红 (异常/停止/危险操作)、黄 (警告/临期)、蓝/紫 (当前激活态/主操作)。
- 字体在 `web/index.html` 引入，根节点固定 `class="dark"`。

---

## 图标与第三方 UI 依赖

- 图标统一使用 `lucide-vue-next`，按需在文件内 import，
  以 `<component :is="item.icon">` 动态渲染（`App.vue:304-320,47-51`、`ToastContainer.vue:38`）。
- 二维码使用 `qrcode.vue`（`UsersView.vue:909`、`SettingsView.vue:380`）。
- 引入新 UI 依赖前需确认现有 Tailwind + lucide 能力不足，避免权重膨胀。

---

## 核心视图交互范式 (Table-First, Double Drawers & Batch Bar)

- **Table-First 布局**：主视图默认通过紧凑高密度数据表格呈现资源列表，支持多选、关键字检索与状态过滤。
- **双抽屉架构 (Double Drawers)**：
  - **Inspector Drawer**：行点击唤起右侧巡检面板（只读状态诊断、凭据复制、快捷开关与关联拓扑矩阵）；
  - **Form Drawer**：新建或编辑资源时滑出右侧结构化表单（分组收拢，告别传统几十个字段的居中弹窗墙）。
- **批量操作条 (Batch Action Bar)**：当选择项数量 `> 0` 时以中性卡片浮动呈现，提供高频批量变更与一键清空选择，操作后即时刷新列表状态。

---

## 动态可视化拓扑画布规范 (Visual Pipeline Topology Canvas)

- **全协议三栏路由流向推导 (`web/src/views/topology/composables/useTopologyGraph.ts`)**：
  - 拓扑图必须同时消费 `/inbounds`、`/outbounds` 与 `/routing` 三个数据源，覆盖全部三条真实路由路径并严格去重收拢：
    1. **Path 1 (`SubRoute` 单端口多出口与双栈聚合)**：解析 `inbounds[].subRoutes`，当多个入站（如 `ipv4入口` 与 `ipv6入口`）配置了相同的 `(routeId, outboundTag)` 时，自动聚合成单个共享 `SubRoute` 胶囊节点（自动剥离 `_v4`/`_v6` 后缀），左接全部源入站、右接目标出口，避免双栈重复卡片膨胀；
    2. **Path 2 (`Routing Rules` 路由规则去重与聚合收拢)**：解析 `routing.rules`（过滤内部 `api` 入站/出站规则以及所有带 `vlessRoute` 的 `SubRoute` 编译影子规则）；对底层全局域名/IP/端口规则按目标 `outboundTag` 聚合成单胶囊，默认仅展示指向非系统出口（如 `warp-out`）的聚合分流胶囊，其余 `direct`/`block` 底层规则通过顶栏 `显示全局规则` 开关按需展开，彻底杜绝十余条防火墙规则刷屏；
    3. **Path 3 (`Default Fallback` 默认兜底出站)**：对既无 `SubRoute`、也无整站无条件 `Scoped Rule` 的普通入站，自动推导指向默认首出站（`outbounds[0]` / `direct`）的兜底通道，确保画布零孤岛入站；
  - 右栏落地出口需统计入度引用计数（`referenceCount`），对零引用出口标记为 `闲置 (isIdle)` 并提供原位「⚡ 挂载」快捷入口；三栏卡片统一采用两行紧凑胶囊布局并垂直居中对齐（`items-center`），实现一屏尽览。
- **零第三方图形库依赖与 SVG SMIL 兼容约束 (`useSvgBezier.ts` & `TopologyCanvas.vue`)**：
  - 严禁引入大型第三方流程图/画布库（如 Vue Flow、G6、ECharts），统一使用原生 SVG 三次贝塞尔曲线（`M x1 y1 C cx1 cy1, cx2 cy2, x2 y2`）结合 DOM 相对坐标测量与 `ResizeObserver` + `requestAnimationFrame` 节流重绘；
  - 流动脉冲粒子 `<animateMotion>` 必须同时提供 `path` 属性与引用 `<path :id="..."><mpath :href="#..." />`，且外层 `<circle>` 需绑定包含 `edge.id + pathD + isActive` 的复合 `:key`，确保各浏览器内核在路径重算或悬停加速时正确重启 SMIL 动画；
  - 桌面端（`lg:` 及以上）展示三栏 SVG 贝塞尔连线画布，窄屏/移动端自动降级为紧凑三级流式链路卡片，并保留顶部切换至「传统分组池 (`TopologyPoolsView`)」的双模开关。

---

## 跨模块上下文导航规范 (Cross-Module Context Navigation)

- **入站至日志溯源导航**：
  - 入站表格操作区与详情抽屉均提供「访问日志」直达跳转，统一定位到 `/logs?type=access&inbound={tag}`；
  - `LogsView` 必须将 URL `route.query`（`type` 与 `inbound`）作为驱动上下文切换的单一事实源，通过 immediate watcher 统一响应，杜绝 `onMounted` 重复请求或状态撕裂；
  - 通过入站上下文切入时，默认重置用户与关键字过滤以保证匿名请求可见，同时保留用户后续手动二次过滤的能力；
  - 结构化访问日志过滤必须按 `item.inbound_tag === targetInbound` 精确匹配；Raw 日志按 `[tag ->` 与 `[tag]` 边界匹配，严禁通过宽松子串误匹配相似标签（如 `local` 误匹配 `local-other`）；
  - 异步请求须使用递增序号或状态保护，杜绝慢网络下的响应乱序覆盖。
