# 三栏动态脉冲连线可视化拓扑画布 PRD

## Goal

将现有的 `TopologyView.vue` 从孤立平铺的文字卡片 Tab 管理模式，全面升级为“接入网关 ➔ 分流通道/路由规则 ➔ 落地出口”三栏动态脉冲连线可视化交互拓扑画布。打通全协议路由流向推导引擎，实时计算平滑三次贝塞尔曲线与流动发光脉冲粒子，实现全链路悬停双向溯源高亮与原位卡片快捷运维，同时提供拓扑画布/传统卡片池双模视图切换与小屏流式降级，兼顾工程整洁解耦与工业级控制台体验。

---

## 背景与痛点 (Background & Pain Points)

1. **割裂孤立的卡片池展示**：
   - 现有 `TopologyView.vue` 将接入网关、分流通道、落地出口拆分为三个孤立的 Tab（`channels`, `gateways`, `exits`）。管理员在同一时间只能看到局部列表，无法在一屏中总览流量从外部入站经过路由与分流节点流向落地节点的宏观拓扑图景。
2. **路由流向推导片面（仅限 VLESS SubRoute）**：
   - 原有实现仅解析了 VLESS 入站的 `subRoutesJson`，其他入站（如普通的 Trojan、Shadowsocks、Socks、VMess 等）被粗暴归纳为“直通”或完全缺乏路由规则关联；
   - 无法直观展示 Xray-core 的 `routing.rules`（如指定 `inboundTag` 的专属规则、全局广告拦截/WARP 分流策略等），导致拓扑与真实的 Xray 路由表脱节。
3. **缺乏全链路视觉穿透与交互溯源**：
   - 当节点与线路增多时，管理员难以快速回答“此落地出口被哪些入站和分流引用？”、“该网关流量分别流向了哪些出站？”等排障问题；
   - 缺乏鼠标悬停时的关联链路动态高亮与无关节点淡化能力。
4. **运维操作跳出感强**：
   - 入站与出站的编辑操作直接跳转到外部路由（`/inbounds?edit=xxx` 或 `/outbounds?edit=xxx`），缺乏原位抽屉联动或就近状态切换能力；
   - 画布缺乏快捷识别“未挂载/闲置出口”并一键将其挂载为订阅线路的直观闭环。
5. **代码膨胀与移动端响应式缺失**：
   - `TopologyView.vue` 目前接近 900 行，将向导弹窗、卡片池、链接解析逻辑堆叠在单一 SFC 巨石中；
   - 缺乏移动端/窄屏自适应策略，若直接渲染复杂三栏连线画布，在小屏幕上会发生严重挤压与连线错位。

---

## 核心需求 (Requirements)

### R1. 三栏动态脉冲连线拓扑画布 (Visual Pipeline Graph)
- **左栏：接入网关 (Inbound Gateways)**：
  - 展示协议图标/徽章（VLESS、Trojan、SS、VMess、Socks 等，遵循语义颜色令牌）；
  - 存活呼吸灯指示器（绿色正常带延迟 `ms` / 红色异常未连通）；
  - 入站 Tag、监听与外部端口映射（`:port ➔ :extPort`）；
  - 上下行流量摘要（Up/Down）；
  - 操作入口：编辑网关、删除网关、跳转查看访问日志（`/logs?type=access&inbound={tag}`）。
- **中栏：分流通道与路由节点 (Channels & Routing Pipeline)**：
  - **VLESS SubRoute 线路节点**：展示线路名称、RouteID（`Route #X`）、挂载网关与出口、状态（已发布/已暂停）、授权用户数；
  - **专属路由规则节点**：展示指定了 `inboundTag` 的特定规则匹配条件（如域名/IP/端口匹配）及目标出口；
  - **全局通用分流节点**：展示全局生效的分流规则摘要（如 Ads 拦截、WARP 分流）；
  - **默认兜底节点 (Default Fallback)**：展示各入站未匹配规则时的默认直连直出节点，确保图中入站 100% 存在流向；
  - 原位快捷操作：一键暂停/启用通道、删除分流线路。
- **右栏：落地出口 (Exit Outbounds)**：
  - 展示出口协议图标/徽章（freedom、vless、trojan、ss、wireguard、blackhole 等）；
  - 出口 Tag、目标服务器端点或设置摘要（如 `hk-node.example.com:443` 或 WARP）；
  - 被引用计数徽章（表示当前被多少条通道/规则指向）；
  - **闲置防护**：若引用计数为 0，展示“未挂载/闲置”警示徽章，并提供“⚡ 1步挂载为线路”快捷入口；
  - 操作入口：编辑设置、删除（系统内置 `direct` / `block` 标记保留）。

### R2. 全协议路由流向推导引擎 (Full-Protocol Route Derivation)
- 前端同时拉取 `/inbounds`、`/outbounds`、`/routing` 与 `/users`；
- **路径 1（VLESS SubRoute）**：解析 `inbound.subRoutesJson`，为每条分流子线路建立 `Inbound -> SubRoute Channel Node -> Target Outbound` 链路；
- **路径 2（路由规则匹配）**：
  - 遍历 `routing.rules`：
    - 若规则包含 `inboundTag`，则建立对应 `Inbound -> Scoped Rule Node -> Outbound` 专属链路；
    - 若为全局通用分流规则（无 `inboundTag` 限制且非内部 `api` 规则），作为共享路由分流通道节点连接普通入站与目标出站；
- **路径 3（默认兜底 Fallback）**：
  - 针对任何无 SubRoute 或未被专属规则垄断的入站，推导一条指向 `outbounds[0]`（通常为 `direct`）的默认兜底直连链路；
  - 确保左侧接入网关零孤岛节点，任意入站均可追溯其物理落地出站。

### R3. 动态 SVG 贝塞尔连线与脉冲粒子动画 (Animated SVG Bezier & Flowing Dots)
- 桌面端（`lg:` 及以上）在画布背景覆盖响应式全尺寸 SVG 图层；
- 动态测量左栏卡片右侧锚点、中栏卡片左右侧锚点、右栏卡片左侧锚点的视口相对坐标；
- 使用平滑三次贝塞尔曲线（Cubic Bezier）绘制连接路径：`M x1 y1 C (x1 + dx*0.5) y1, (x2 - dx*0.5) y2, x2 y2`；
- **链路状态视觉语言**：
  - **启用态 (Active)**：具有流动发光粒子效果的虚线，高亮粒子顺着贝塞尔轨道从左向右平滑流动；
  - **暂停态 (Paused)**：暗灰色静态虚线，无发光粒子流动；
  - **阻断态 (Blocked, 如指向 block/blackhole)**：暗红色拦截虚线，警示拦截流量；
- 使用 `ResizeObserver` 监听画布容器，结合 `requestAnimationFrame` 防抖实时计算连线坐标，支持页面缩放、滚动与折叠时无漂移渲染。

### R4. 悬停全链路高亮溯源与原位交互 (Interactive Path Highlighting & In-Place Actions)
- 当鼠标悬停在任一节点（网关、分流通道、出口）时：
  - 自动向上游与下游双向拓扑寻径，找出连通的完整链路（`Inbound ↔ Channel ↔ Outbound`）；
  - 高亮全链路上的所有卡片与 SVG 贝塞尔曲线，边框发光；
  - 其余非关联卡片与连线平滑降暗淡化（`opacity-25`，CSS transition 平滑过渡）；
- 支持直接在卡片上触发关键运维行为：
  - 分流通道原位启停与删除（直接更新入站 SubRoute）；
  - 快速唤起“⚡ 1步发布落地线路”向导弹窗；
  - 快速跳转网关/出口编辑或详情检视。

### R5. 视图模式切换与移动端自适应降级 (View Switcher & Mobile Responsive)
- 顶部常驻视图切换控件：
  - `「拓扑画布 (Canvas Graph)」`：默认主视图，提供三栏脉冲图谱；
  - `「传统分组池 (Card Pools)」`：保留原有的三 Tab 卡片列表管理形态，满足高密度批量检视需求，零功能丢失；
- **移动端与窄屏自适应**：
  - 屏幕宽度 `< lg` 时，自动隐藏全屏 SVG 连线图层；
  - 将三栏画布优雅降级为紧凑的**三级流向链路卡片（左➔中➔右 垂直/折行流式指示流）**，使用内联流向箭头与徽章标识上下游关系，杜绝横向滚动溢出。

---

## 验收标准 (Acceptance Criteria)

- **AC1: 三栏拓扑画布完整展示 (Visual Pipeline Graph Completeness)**：
  - 左中右三栏清晰呈现：接入网关、分流通道与路由规则、落地出口；
  - 每个卡片呈现协议徽标、状态、端口、延迟或流量等核心指标；
  - 右侧出口卡片准确展示被引用计数，对零引用出口展示“闲置/未挂载”徽标与快捷发布入口。
- **AC2: 全协议与兜底流向推导 (Full-Protocol Route Derivation & Zero Orphans)**：
  - 正确解析 VLESS SubRoute、普通入站 scoped 规则、全局通用分流规则与缺省直连 fallback；
  - 杜绝左侧网关出现孤岛节点，所有入站均能追溯至中栏通道及右栏出口。
- **AC3: 动态 SVG 贝塞尔连线与脉冲动画 (SVG Bezier & Pulsing Flow Animation)**：
  - 桌面端精确测量左右中卡片挂载点，生成三次贝塞尔曲线；
  - 启用链路具备动态流动发光粒子/虚线脉冲效果，暂停链路呈灰色静态虚线，阻断链路呈红色拦截虚线；
  - 容器缩放、窗口改变尺寸时通过 ResizeObserver + RAF 平滑重绘，无漂移断裂。
- **AC4: 悬停全链路高亮与穿透交互 (Hover Path Highlighting & Direct Actions)**：
  - 悬停卡片即刻高亮完整上下游路径（网关 ↔ 通道 ↔ 出口），非关联元素平滑淡化至 25% 不透明度；
  - 支持在卡片上直接执行启停通道、删除通道、跳转编辑网关/出口、唤起发布向导。
- **AC5: 视图双模切换与向导保真 (View Mode Switching & Quick Publish Parity)**：
  - 提供“拓扑画布”与“传统分组池”平滑切换，原有 1步发布向导与三池管理能力 100% 保持可用。
- **AC6: 移动端自适应与工程质量门禁 (Mobile Adaptability & Quality Gates)**：
  - 小屏端自动降级为紧凑流式链路卡片，无横向溢出与错位；
  - 前端 `npm run typecheck` 与 `npm run build` 零警告零错误通过，满足 Neutral Design Tokens 设计规范。
