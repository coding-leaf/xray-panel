# 技术设计方案 (Technical Design)

## 1. 架构定位：彻底走向单一事实源 (Single Source of Truth)

过去系统的最大弊端是“两层皮”：入站模块在编译器 Layer 3 隐式追加兜底规则，而路由视图又直接读取编译产物，导致保存时规则互相回吞、成倍复制。

本方案彻底确立**“除 `api` 外完全放权、单一事实源”**：
- **消灭 Layer 3**：编译器绝不在用户规则背后隐式追加任何规则。
- **1:1 映射**：用户在「路由管理」看到的 N 条规则，就是 Xray 内核编译出的 N 条规则。
- **新建线路显式入表**：新建入站线路时，默认在规则表尾部追加显式规则，让用户完全掌控优先级。

```text
【改造前 (两层皮 & 回环复制)】
前端 Rules ---> [Layer 2 自定义规则]
                       +
Inbounds    ---> [Layer 3 隐式追加 SubRoute 兜底] ---> config.json
                       ↑                                    │
                       └──────── GET /routing 回显 ─────────┘ (循环污染翻倍)

【改造后 (单一事实源 & 完全放权)】
前端 Rules / 新建线路默认放尾巴 ---> [统一路由规则池]
                                          │
                                          ↓ (完全忠实按序编译)
                                  Layer 1: api -> api (仅内部 gRPC)
                                  Layer 2: 统一规则池 (含 vlessRoute、域名、IP、阻断规则)
                                          │
                                          ↓
                                      config.json (零隐式追加，零回环翻倍)
```

---

## 2. 关键代码变更点

### 2.1 编译器 `compiler.go` 重构
1. 移除 Layer 3 `for _, inb := range inbounds` 的隐式生成逻辑。
2. 在处理 `routing.Rules` 时增加连续完全一致规则的幂等去重（Deduplication），自动清洗历史脏数据中的堆叠项。
3. 确保所有包含 `vlessRoute` 的规则严格忠实编译。

### 2.2 线路与路由联动（默认放尾部）
在 `ConfigService` 或 `InboundService` 中：
当检查入站的分流线路时，若某启用线路（`inb.Tag + sr.RouteID`）在现有路由表中完全没有任何规则匹配，则将其显式作为一条普通兜底规则追加至 `routing.Rules` 末尾（Tail），保证即使新建线路也不会断网，同时在前端路由面板立即可见、可调。

### 2.3 前端高对比度按钮与排位交互
在 `web/src/views/RoutingView.vue` 中：
- 加回 `ArrowUp` 和 `ArrowDown` 按钮；
- 按钮尺寸：`h-8 w-8`，图标 `w-4 h-4`；
- 颜色对比度：
  - 容器背景为 `bg-neutral-900 / bg-card`；
  - 按钮采用 `bg-neutral-800 border border-neutral-600/80 text-neutral-100 hover:bg-neutral-700 hover:text-white`；
  - 禁用态采用 `disabled:opacity-25`；
  - 桌面端与移动端卡片对齐。

---

## 3. 兼容性与防线

- **兼容性**：完全兼容现有 Xray 路由规则格式。
- **平滑过渡**：清理历史堆叠重复项后，现有节点分流与穿透规则立即生效，不再被幽灵兜底截留。
