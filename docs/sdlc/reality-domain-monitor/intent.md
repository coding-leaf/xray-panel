# Intent: Reality 域名合规性定期监测与面板告警

- **任务编号**: reality-domain-monitor
- **提出人**: User / Dev
- **创建时间**: 2026-09-20 15:07
- **初始 Change Tier**: Tier 2
- **当前状态**: In-Review

---

## 1. 问题与现状背景 (Problem)
在使用 Xray 的 VLESS/Trojan 等协议配置 Reality 伪装时，入站高度依赖外部伪装目标网站（`dest` 及其 `serverNames`）。
如果伪装目标网站出现以下情况，将直接导致客户端握手失败、节点完全瘫痪：
1. 目标服务器不支持 TLSv1.3（Reality 的强制技术规范）；
2. 目标服务器缺少 ALPN（如 h2/http/1.1）；
3. 目标服务器证书过期或 SNI 域名与证书域名（SAN/CN）不匹配；
4. 目标服务器端口（443）网络中断、被防火墙阻断或宕机。

目前 xray-panel 仅在保存时进行基本的格式清洗，运行期缺乏对目标域名的连通性与合规性定期巡检。当外部目标发生变动或失效时，管理员无法及时感知，排查成本高。

## 2. 变更性质分类 (Change Archetype - 单选)
- [ ] 局部结构精简 (Local Cleanup - 仅限模块内部冗余消除，不改数据流向与全局装配)
- [x] 单模块特性演进 (Single-Module Feature - 单一模块业务增量或修复)
- [ ] 跨领域架构重构 (Cross-Domain Rewiring - 触及应用全局装配、生命周期或跨域流向，必须升 Tier 3)

## 3. 期望达成效果 (Proposed Outcome)
1. **轻量合规性探测能力**：提供对 Reality 伪装目标（`dest` 端口与 `serverNames`）的网络连通性、TLS 1.3 协商、ALPN 支持、证书有效期及 CDN 阻断判定的非阻塞探测。
   - **CDN 拦截判定**：在 TLS/HTTP 探测时，加入对 Cloudflare 等公共 CDN（如响应头 `server: cloudflare`、`cf-ray` 等特征）的阻断判定。若套了 CDN，直接告警提示“套用了公共 CDN，易被盗流/特征异常”；
   - **证书临期阈值**：将证书检查细化为“证书已过期，或剩余有效期不足 7 天时触发临期预警”，为管理员留足更换伪装域名的时间窗口。
2. **自动化定期巡检**：后台周期性任务按默认 12 小时间隔自动巡检所有启用了 Reality 的入站，且支持通过 API 触发手动立即检测。
3. **面板显式告警**：
   - 入站管理列表（InboundsView）直观标注异常节点的警示徽标与详细失败原因（悬浮/展开展示，如“套用了公共 CDN”、“证书临期不足 7 天”、“不支持 TLS 1.3”等）；
   - 仪表盘（DashboardView）汇总提示存在异常的 Reality 入站；
   - 联动 AlertService，当检测到域名失效时触发告警（支持冷却控制防刷屏）。
4. **无副作用与平滑降级**：探测过程超时严格可控（单次不超过 3-5 秒），并发安全，绝对不阻塞 Xray 核心运行与常规入站请求。

## 4. 波及工程分面 (Affected Architectural Layers)
- [x] 核心领域与计算逻辑 (Domain & Core Business Logic)
- [x] 外部接口与协议入口 (Public Ingress & Controllers & Protocols)
- [ ] 数据持久化与状态存储 (Database & Storage & Schemas)
- [x] 全局装配与应用入口 (Bootstrap & Lifecycle & Service Wiring)

## 5. 边界与硬性约束 (Constraints & Boundaries)
* **硬性技术制约**:
  - 必须使用标准库 `crypto/tls` 与 `net` 执行探测，不引入笨重外部第三方依赖；
  - 探测失败仅在面板告警与通知，绝对不自动篡改或关闭用户的入站配置；
  - 必须遵守 `-race` 并发安全，监测结果在内存或缓存中有序维护，避免 Goroutine 泄漏。
* **明确非目标 (Non-Goals / Out-of-Scope)**:
  - 不实现目标域名的自动替补或自动轮换；
  - 不篡改 Xray 核心配置文件；
  - 不修改数据库 Schema（探测结果采用内存缓存与实时状态聚合即可）。
* **完成判定条件 (Definition of Done)**:
  - 探测逻辑单元测试覆盖各种异常场景（非 TLS 1.3、证书不匹配、超时网络不可达、正常 Reality 域名等）；
  - HTTP 接口提供检测状态获取与手动触发；
  - 前端 UI 在 InboundsView 与 DashboardView 正确展示告警信息；
  - `go test -race ./...` 和 `go vet ./...` 全绿，前端构建顺利通过。

## 6. 未决疑问与待探讨点 (Open Questions)
- 无未决疑问（用户已确认检测周期默认 12 小时，告警呈现在入站列表与仪表盘，支持手动检测）。

---

## 7. 阶段准出签批 (Gate 1 Sign-off)
- [x] 场景与问题已客观复现并达成共识
- [x] 边界、非目标与约束清晰明确
- [x] 初始 Change Tier 评定合理
- **准出结论**: Accepted
- **签批人 / 日期**: User / 2026-09-20 15:55
