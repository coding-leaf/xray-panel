# Intent: Cloudflare 多 VPS 弹性调度与顺序漫游网关

- **任务编号**: cf-multi-upstream-gateway
- **提出人**: Dev
- **创建时间**: 2026-09-21 18:36
- **初始 Change Tier**: Tier 2
- **当前状态**: In-Review

---

## 1. 问题与现状背景 (Problem)
- **单源站单点故障与被墙瘫痪**：当前 Cloudflare Pages / Worker (`deploy/cloudflare-pages/_worker.js`, `deploy/cloudflare-worker-sub-proxy.js`) 仅支持单 `UPSTREAM_ORIGIN`。当主 VPS 遭遇 IP 阻断、端口封锁或系统宕机时，整个中立门户与节点分发彻底瘫痪。
- **裸备机暴露高危风险**：若直接向用户分发备用机域名/IP，备用机缺乏 Cloudflare WAF/CDN 保护，不仅极易直接暴露源站真实 IP 导致被墙，也破坏了“单一中立域名”的用户体验。
- **旧代码技术缺陷**：
  1. **POST 请求体单次消费**：当前代码直接透传 `request.body`，流一旦被消费后无法复用，若向第一台机器发送失败后直接抛出异常，无法安全切到下一台；
  2. **缺乏快速熔断**：VPS 被墙阻断多表现为 TCP 握手假死与无限丢包，旧代码等待上游默认超时（常达数十秒甚至超限 524），用户端长久转圈卡死；
  3. **状态码语义偏差**：旧代码直接透传首个源站的 404/400 响应。在多机独立无同步场景下，单机 404 仅代表“该凭据未在此机创建”，绝不代表凭据整体无效。

## 2. 变更性质分类 (Change Archetype - 单选)
- [ ] 局部结构精简 (Local Cleanup - 仅限模块内部冗余消除，不改数据流向与全局装配)
- [x] 单模块特性演进 (Single-Module Feature - 单一模块业务增量或修复)
- [ ] 跨领域架构重构 (Cross-Domain Rewiring - 触及应用全局装配、生命周期或跨域流向，必须升 Tier 3)

## 3. 期望达成效果 (Proposed Outcome)
1. **多 VPS 顺序漫游寻呼机制 (Sequential Roaming)**：
   - 提取分发码 (`/api/portal/claim`) 或订阅请求 (`/sub/*`, `/sub?token=...`) 时，网关充当无状态顺序寻呼机，依次向各 VPS 发起请求；
   - 命中判定：仅 `200 OK` 视为命中，立即净化响应头并返回给客户端，终止后续漫游；
   - 顺延判定：遇到 `400/403/404`（码不在此机）或 `超时 / 5xx / 连接重置`（节点宕机或被墙），毫秒级静默切换下一台。
2. **请求体安全重用与 2.5s 极速熔断**：
   - 预缓存非 GET/HEAD 请求的 body 二进制或文本缓冲，支持在多轮回源请求中多次构造有效 Request；
   - 每台上游调用配置严格的 2.5 秒超时控制（AbortController / AbortSignal.timeout），遇假死立即切断跳下一台。
3. **彻底解耦与单一中立入口**：
   - 各 VPS 完全自治，不进行跨机数据同步；
   - 用户统一访问 Cloudflare 网关，源站 IP/域名完全隐匿。
4. **全节点失活中立兜底**：
   - 若所有候选节点均寻呼失败，返回统一格式的中立 404/502 语义，避免向客户端暴露上游真实机器堆栈或 IP 信息。

## 4. 波及工程分面 (Affected Architectural Layers)
- [ ] 核心领域与计算逻辑 (Domain & Core Business Logic)
- [x] 外部接口与协议入口 (Public Ingress & Controllers & Protocols)
- [ ] 数据持久化与状态存储 (Database & Storage & Schemas)
- [ ] 全局装配与应用入口 (Bootstrap & Lifecycle & Service Wiring)

## 5. 边界与硬性约束 (Constraints & Boundaries)
* **硬性技术制约**:
  - 必须完全兼容现有 Cloudflare Worker 与 Cloudflare Pages Functions 环境；
  - 环境变量配置向后兼容：支持 `UPSTREAM_ORIGINS`（逗号/换行分隔的多源站地址）及已有的单源站 `UPSTREAM_ORIGIN`；
  - 每台 VPS 请求超时时间硬性锁定为 2.5 秒（2500ms）；
  - 保持已有安全基线：多重 URL 规范化防路径穿越、爬虫 UA 伪装响应、敏感路径 `no-store` 防边缘缓存等均不可削弱。
* **明确非目标 (Non-Goals / Out-of-Scope)**:
  - 不涉及 VPS 后端 Go 服务、数据库、Schema 或业务逻辑的任何修改；
  - 不做跨 VPS 的数据同步、数据库主从同步或订阅凭据对齐；
  - 不涉及前端 UI 的视觉重构。
* **完成判定条件 (Definition of Done)**:
  1. `deploy/cloudflare-worker-sub-proxy.js` 与 `deploy/cloudflare-pages/_worker.js` 完成改造并具备统一的漫游调度与防重放容灾能力；
  2. 单元测试/模拟集成测试验证：
     - 单源站兼容性正常；
     - 多源站场景下，首机 404/超时自动静默回退至第二机并成功返回 200；
     - POST 请求（`/api/portal/claim`）可跨多个源站重试且 body 完整送达；
     - 超时 2.5 秒强行熔断切下一台；
     - 全部机器失败时返回安全兜底响应。

## 6. 未决疑问与待探讨点 (Open Questions)
- 无未决阻塞项。多源站支持使用标准逗号或换行分隔的 URL 列表，便于在 Cloudflare 环境变量中维护。

---

## 7. 阶段准出签批 (Gate 1 Sign-off)
- [ ] 场景与问题已客观复现并达成共识
- [ ] 边界、非目标与约束清晰明确
- [ ] 初始 Change Tier 评定合理
- **准出结论**: Pending
- **签批人 / 日期**: [待人类签批] / 2026-09-21 18:36
