# 三栏动态脉冲连线可视化拓扑画布 - 技术设计 (Design)

## 1. 架构边界与组件拆分设计

为了彻底解决原有 `TopologyView.vue` 近 900 行单文件巨石架构（包含数据拉取、卡片池渲染、链接解析、向导弹窗、DOM 操作混合）的可维护性瓶颈，本次设计采用领域就近解耦原则，将拓扑功能收敛至 `web/src/views/topology/` 目录：

```
web/src/views/
├── TopologyView.vue                         # 顶层调度视图（视图模式切换、数据加载、向导及路由跳转）
└── topology/
    ├── types.ts                             # 拓扑图模型、节点、连线、高亮状态与交互契约类型定义
    ├── composables/
    │   ├── useTopologyGraph.ts              # 拓扑推导引擎（全协议路由流向推导、节点与边提取、双向索引）
    │   └── useSvgBezier.ts                  # SVG 贝塞尔曲线计算、DOM 锚点测量、ResizeObserver 与 RAF 调度
    └── components/
        ├── TopologyCanvas.vue               # 三栏拓扑连线画布主容器（含响应式 SVG 动态脉冲层）
        ├── InboundNodeCard.vue              # 左栏：接入网关节点卡片
        ├── ChannelNodeCard.vue              # 中栏：分流通道 / 路由规则 / 兜底直出卡片
        ├── OutboundNodeCard.vue             # 右栏：落地出口卡片（含引用计数与闲置发布引导）
        ├── TopologyPoolsView.vue            # 传统分组池管理视图（Channels/Gateways/Exits 三 Tab 高保真）
        └── QuickPublishWizardModal.vue      # 1步发布落地线路向导弹窗（从主视图解耦剥离）
```

### 职责分工矩阵

| 模块 / 组件 | 归属与职责 | 外部依赖 |
|------------|-----------|---------|
| `TopologyView.vue` | 顶层调度：管理 `viewMode`（`canvas` / `pools`）、拉取 `/inbounds`、`/outbounds`、`/routing`、`/users`、协调向导弹窗唤起与操作反馈 | `api`, `toast`, 子组件 |
| `useTopologyGraph.ts` | 纯逻辑数据流推导：将原始数据清洗并推导为标准图结构（`nodes`, `edges`, `adjacencyMap`），计算引用计数与孤岛防护 | 仅依赖 `types.ts`（无 DOM 依赖，易测试） |
| `useSvgBezier.ts` | 几何测量与动画调度：监听 DOM 容器变化，计算各卡片左右锚点在画布坐标系中的 `(x, y)`，生成 Cubic Bezier 路径 | `ResizeObserver`, `requestAnimationFrame` |
| `TopologyCanvas.vue` | 桌面三栏画布与 SVG 浮层组装，提供悬停高亮状态分发与移动端流式链路折叠渲染 | `lucide-vue-next`, 卡片子组件 |
| `QuickPublishWizardModal.vue` | 独立向导弹窗：支持节点链接自动解析（vless/trojan/ss）与现有出口选择，一键创建出站并挂载 SubRoute | `api`, `toast`, `Button`, `Modal` |
| `TopologyPoolsView.vue` | 传统池视图：原汁原味承接原先的 3 Tab 高密度卡片列表，保障既有管理习惯零断层 | `lucide-vue-next` |

---

## 2. 数据流与路由拓扑推导引擎设计 (Topology Derivation Engine)

### 2.1 核心类型契约 (`types.ts`)

```typescript
export type NodeType = 'inbound' | 'channel' | 'outbound'
export type ChannelCategory = 'subroute' | 'scoped_rule' | 'global_rule' | 'fallback'
export type LinkStatus = 'active' | 'paused' | 'blocked'

export interface TopologyNode {
  id: string                     // 唯一标识，如 inb-1, ch-1-route-2, rule-3, out-direct
  type: NodeType
  category?: ChannelCategory     // 中栏专属分类
  title: string
  subtitle?: string
  tag: string                    // 对应的 inboundTag / outboundTag
  protocol: string               // vless, trojan, freedom, wireguard, socks, etc.
  status: 'online' | 'offline' | 'paused' | 'normal'
  isAlive?: boolean
  latencyMs?: number
  metrics?: {
    upBytes?: number
    downBytes?: number
    clientCount?: number
    referenceCount?: number     // 右栏出站被引用数
  }
  raw: any                       // 原始领域对象引用
}

export interface TopologyEdge {
  id: string                     // edge-{sourceId}-{targetId}
  sourceId: string               // 起始节点 ID (inbound -> channel 或 channel -> outbound)
  targetId: string               // 目标节点 ID
  status: LinkStatus             // active (发光流动), paused (静态灰), blocked (暗红拦截)
  category: ChannelCategory
  label?: string
}

export interface TopologyGraph {
  inboundNodes: TopologyNode[]
  channelNodes: TopologyNode[]
  outboundNodes: TopologyNode[]
  edges: TopologyEdge[]
  // 邻接索引（用于 O(1) 悬停溯源计算）
  upstreamMap: Map<string, Set<string>>    // nodeId -> Set<upstreamNodeId>
  downstreamMap: Map<string, Set<string>>  // nodeId -> Set<downstreamNodeId>
  nodeEdgesMap: Map<string, Set<string>>   // nodeId -> Set<edgeId>
}
```

### 2.2 全协议路由流向推导流水线 (`useTopologyGraph.ts`)

```
   ┌────────────────────────────────────────────────────────┐
   │ Raw Inputs: Inbounds[], Outbounds[], Rules[], Users[]  │
   └───────────────────────────┬────────────────────────────┘
                               │
               ┌───────────────┼───────────────┐
               ▼               ▼               ▼
         [Path 1]         [Path 2]        [Path 3]
       VLESS SubRoute    Routing Rules   Fallback Outbound
               │               │               │
               ▼               ▼               ▼
      ch-{inb}-{routeId}  rule-{index}   fallback-{inb}
               │               │               │
               └───────────────┼───────────────┘
                               │
                               ▼
               ┌───────────────────────────────┐
               │    Normalized TopologyGraph   │
               │ (Inbounds, Channels, Exits)   │
               │  Adjacency & Reference Map    │
               └───────────────────────────────┘
```

1. **左栏节点标准化 (Inbound Nodes)**:
   - 遍历 `inbounds`，对每个入站提取协议、端口、外部端口、延迟、流量及在线状态；
   - 统计该入站绑定的客户端用户数（通过匹配 `users[].inboundTags` 或 `inboundTag`）。
2. **右栏节点标准化与引用索引 (Outbound Nodes)**:
   - 遍历 `outbounds`，提取 Tag、协议、设置摘要（Freedom 直连、Blackhole 拦截、WARP 或远端节点端点）；
   - 维护 `referenceCount = 0`，用于后续流向关联计数。
3. **中栏通道与边推导三路径 (Three Derivation Paths)**:
   - **Path 1: VLESS SubRoute 分流线路**:
     - 若入站包含 `subRoutes`，遍历每条分流线路：
       - 创建 Channel 节点：`category: 'subroute'`，携带 `routeId`、`name`、`enabled`、`allowedUsers` 计数；
       - 创建入站至通道的边：`edge(inb.id, ch.id)`；
       - 创建通道至出口的边：`edge(ch.id, subRoute.outboundTag)`；
       - 若 `subRoute.outboundTag` 匹配到出站节点，对应出站的 `referenceCount++`；
       - 边状态：`subRoute.enabled === false` ➔ `paused`；目标出口为 `block/blackhole` ➔ `blocked`；其余为 `active`。
   - **Path 2: 路由规则匹配 (Routing Rules)**:
     - 遍历 `routing.rules`（忽略内置 `outboundTag === 'api'` 规则）：
       - **Scoped Rule（指定 `inboundTag`）**：
         - 针对指定的每个 `inbTag`，在中栏创建规则通道节点（展示匹配条件：如 IP/域名/端口），分别建立 `Inbound(inbTag) -> Rule Node -> Outbound(rule.outboundTag)` 两条边；
       - **Global Rule（无 `inboundTag`，全局分流策略）**：
         - 若系统存在普通入站（无 SubRoute 的入站，如 Trojan/SS/Socks），在中栏创建共享规则分流节点（如“广告拦截”、“WARP 国内分流”），连接相关入站与目标出口；
       - 相应增加目标出口的 `referenceCount`。
   - **Path 3: 默认兜底出口 (Default Fallback)**:
     - 针对所有入站（特别是无 SubRoute 或允许直通的入站），创建兜底直连通道节点（指向系统第一个出口 `outbounds[0]`，通常为 `direct`）；
     - 确保**每一个接入网关至少有一条明确的下游出口链路**，彻底消灭拓扑孤岛。
4. **孤岛与闲置出站标记**:
   - 右栏出站若 `referenceCount === 0`，标记为 `isIdle = true`，在卡片上高亮提示“未挂载”，并显示“一键挂载”操作。

---

## 3. SVG 动态贝塞尔曲线与粒子脉冲渲染方案 (`useSvgBezier.ts`)

### 3.1 视口坐标系与锚点换算

为了防止在滚动或缩放时发生位置偏离，连线计算采用**容器相对坐标系**：

```
       Canvas Container (Rect: left0, top0, width, height)
       ┌────────────────────────────────────────────────────────┐
       │                                                        │
       │   [Inbound Card]          [Channel Card]        [Exit] │
       │  ┌──────────────┐ Right  Left ┌─────────┐              │
       │  │              │●───────────●│         │              │
       │  └──────────────┘ Anchor Anchor└─────────┘              │
       │                                                        │
       └────────────────────────────────────────────────────────┘
```

- 获取画布容器矩形：`const containerRect = canvasEl.getBoundingClientRect()`；
- 卡片锚点坐标计算公式：
  - **卡片右侧锚点 (Right Anchor)**：
    $$\begin{cases} x = \text{cardRect.right} - \text{containerRect.left} \\ y = \text{cardRect.top} - \text{containerRect.top} + \dfrac{\text{cardRect.height}}{2} \end{cases}$$
  - **卡片左侧锚点 (Left Anchor)**：
    $$\begin{cases} x = \text{cardRect.left} - \text{containerRect.left} \\ y = \text{cardRect.top} - \text{containerRect.top} + \dfrac{\text{cardRect.height}}{2} \end{cases}$$

### 3.2 平滑三次贝塞尔曲线算法

连接点从 $(x_1, y_1)$ 到 $(x_2, y_2)$，水平跨度 $\Delta x = x_2 - x_1$。
采用三次贝塞尔（Cubic Bezier）生成柔和水平切线流动弧度：

```typescript
function buildCubicBezierPath(x1: number, y1: number, x2: number, y2: number): string {
  const dx = Math.max(x2 - x1, 40)
  const cx1 = x1 + dx * 0.5
  const cy1 = y1
  const cx2 = x2 - dx * 0.5
  const cy2 = y2
  return `M ${x1} ${y1} C ${cx1} ${cy1}, ${cx2} ${cy2}, ${x2} ${y2}`
}
```

### 3.3 脉冲粒子流动动画机制

每条边由双层 SVG 元素构成，兼顾底层质感与动态发光效果：

1. **底层导轨 (Base Track)**：
   - 静态虚线：`stroke="currentColor"`，`stroke-dasharray="4 4"`，`stroke-width="1.5"`，透明度 `0.2` ~ `0.4`。
2. **顶层发光流动层 (Flowing Pulse Layer)**：
   - **方式 A（纯 CSS 虚线流光，性能最优，零额外 DOM）**：
     ```css
     .flowing-edge {
       stroke-dasharray: 6 12;
       stroke-dashoffset: 0;
       animation: flowStroke 1.8s linear infinite;
     }
     @keyframes flowStroke {
       to { stroke-dashoffset: -36; }
     }
     ```
   - **方式 B（SVG `animateMotion` 发光光斑粒子，极具工业美感）**：
     ```html
     <circle r="3" fill="#38bdf8" filter="url(#glow)">
       <animateMotion
         :path="edge.pathD"
         dur="2.4s"
         repeatCount="indefinite"
       />
     </circle>
     ```
   - 实现中组合使用：虚线动态偏移流速指示 + 活跃高亮链路上的发光流动圆点。
3. **状态颜色映射**：
   - `active`（启用）：Cyan / Indigo / Emerald 渐变光，流速正常；
   - `paused`（暂停）：Neutral-600 灰色暗淡虚线，静止不流动；
   - `blocked`（拦截/黑洞）：Rose-500 暗红色虚线，带有警告感。

### 3.4 响应式重绘与性能保障机制

- 挂载 `ResizeObserver` 监听 `canvasEl` 容器尺寸变化；
- 监听 `window` 的 `resize` 与 `scroll` 事件；
- 采用 `requestAnimationFrame` 节流调度（Debounce/RAF Batching），在同一浏览器重绘周期内最多执行一次连线重算，杜绝高频重排抖动；
- 组件卸载时严格成对注销 `observer.disconnect()`。

---

## 4. 交互状态机与全链路双向高亮设计

### 4.1 悬停拓扑溯源状态机

```
                      [Mouse Enter Node / Edge]
                                 │
                                 ▼
                     Compute Reachable Subgraph
                     (Upstream & Downstream BFS)
                                 │
                                 ▼
                   Set activeNodeIds & activeEdgeIds
                                 │
                ┌────────────────┴────────────────┐
                ▼                                 ▼
         Active Elements                  Inactive Elements
   - Opacity 100%                   - Opacity 20% ~ 25%
   - Stroke highlighted & glowing   - Stroke dimmed & static
   - Card border accent glow        - Desaturated
                                 │
                                 ▼
                      [Mouse Leave Canvas / Node]
                                 │
                                 ▼
                     Reset hoveredNodeId = null
                      All elements Opacity 100%
```

- **双向追溯算法**：
  - 维护预构建的图邻接表 `upstreamMap` 与 `downstreamMap`；
  - 当鼠标悬停在卡片 $N$ 上时：
    1. 向上寻找 $N$ 的所有前置网关（若 $N$ 为出站，回溯中间通道及所有源入站；若 $N$ 为通道，回溯其源入站）；
    2. 向下寻找 $N$ 的所有落地出口（若 $N$ 为入站，顺流找到挂载的所有通道及落地出站）；
    3. 将包含的节点集合放入 `activeNodeIds`，直接相连的边放入 `activeEdgeIds`；
  - 配合 Tailwind 类 `:class="{ 'opacity-25 blur-[0.2px]': isDimmed(node.id) }"` 实现丝滑降暗与聚焦。

### 4.2 原位交互与模态框调度

- **分流通道启停**：
  - 在卡片内直接点击“暂停/启用”，调用 `api.put('/inbounds/:id')` 更新 `subRoutesJson`，就地刷新，无需离开画布；
- **分流通道删除**：
  - 卡片内直接确认删除，自动将该 SubRoute 从对应入站的 `subRoutes` 数组中移除并保存；
- **快速 1 步发布向导**：
  - 点击卡片或右上方“⚡ 1步发布落地线路”，唤起 `QuickPublishWizardModal`；
  - 支持直接粘贴 `vless://` / `trojan://` / `ss://` 链接自动解析出站协议与参数，选定网关后自动分配下一个空闲 `RouteID` 并直接注入生效；
- **巡检与编辑直达**：
  - 点击网关卡片或出口卡片上的“编辑/检视”，平滑跳转到对应管理视图的编辑抽屉上下文。

---

## 5. 视图双模切换与移动端响应式降级设计

### 5.1 顶部视图切换器设计

在视图 Header 放置精致的 Segmented Control 切换器：
- `「拓扑画布 (Canvas Graph)」`（默认激活，图标 `Workflow` 或 `Network`）；
- `「传统分组池 (Card Pools)」`（图标 `LayoutGrid`，展示原有 Channels / Gateways / Exits 三 Tab 卡片管理列表）。
- 切换状态通过本地 `ref<'canvas' | 'pools'>` 维持，支持在两种管理视角之间无缝穿梭。

### 5.2 移动端与窄屏自适应降级 (`< lg`)

在移动端（屏幕 `< 1024px`）下，三栏横向贝塞尔连线容易因空间狭窄造成文字截断和线条杂乱。设计专属的优雅降级方案：

1. **自动隐藏 SVG 连线图层**：`<div class="hidden lg:block absolute inset-0 pointer-events-none">...</div>`；
2. **切换为流式链路卡片流 (Pipeline Stream Cards)**：
   - 采用纵向或折行卡片结构，将每一组连通关系呈现为紧凑的流水线单元：
     ```
     ┌────────────────────────────────────────────────────────┐
     │ 🇯🇵 日本原生直连 (Route #1)                    [已发布]  │
     ├────────────────────────────────────────────────────────┤
     │  入站网关: [vless-reality :443]                        │
     │     ↓ (Scoped VLESS 分流 0x0001)                       │
     │  落地出口: [direct (FREEDOM)]                          │
     └────────────────────────────────────────────────────────┘
     ```
   - 并在各卡片内提供明确的上下游 Badge 标签与跳转操作，保证移动端触摸运维体验极致流畅。

---

## 6. 与现有系统与 Spec 的契约一致性

1. **Design Tokens 纯粹性**：
   - 严格遵循 `.trellis/spec/frontend/components-and-routing.md`，使用 CSS 语义变量（`bg-background`, `bg-card`, `border-border`, `text-foreground`, `text-muted-foreground`）；
   - 杜绝毛玻璃 `backdrop-blur` 与野性 Hex 色值，圆角统一为 `rounded-md` (6px) 与 `rounded-lg` (8px)；
2. **API 与协议数据一致性**：
   - 严格遵循 `api-and-dataflow.md`，使用单例 `api` 发起请求，不引入未授权的全局依赖；
   - 协议流控徽标与参数清洗与 `web/src/views/inbounds/` 严格统一。
3. **前端构建与类型安全**：
   - 完整输出 TypeScript 类型接口；
   - 确保 `npm run typecheck` 零错误与 `npm run build` 成功。
