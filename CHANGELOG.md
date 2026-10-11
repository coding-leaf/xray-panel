# Changelog (更新日志)

本项目遵循规范记录所有重要版本的功能迭代、性能重构与缺陷修复。

---

## v2.6.3 (2026-10) - 用户生命周期三态一致性与Saga反向补偿、启动容灾与质量门禁加固
- **用户 CRUD 三态一致性与 Saga 反向补偿机制 (Saga Compensation)**：
  - 彻底清除 `UserService` 中对 `xrayManager` 与 `configSvc` 的静默吞错（`_ =`），异常显式向上层暴露并包裹精准阶段上下文（`%w`）；
  - `CreateUser` 引入反向补偿事务：一旦 Xray 节点注入超时或配置文件落盘失败，立即逆序剔除已挂载节点并调用 `userRepo.Delete` 回滚删除 SQLite 记录，杜绝孤儿账号残留，允许同名邮箱安全重试；
  - `UpdateUser` 引入快照回滚机制：保存修改前只读快照，若运行态刷新或写盘失败，精确恢复 Xray 节点与数据库至快照状态；
  - `ResetSubToken` 引入凭证原子回滚：失败时写回原 UUID 与 SubToken，防止凭证半更新导致客户端断网且无法恢复；
  - 重构 `DeleteUser` 执行顺序为“Xray 热剔除 ➔ 物理文件清除 ➔ 数据库最后提交删除”，保障防撕裂数据安全；
  - 批量操作（`BatchRenew`、`BatchResetTraffic`、`BatchSetStatus`）消除静默 `continue`，使用 `errors.Join` 聚合上报失败项明细。
- **服务启动容灾日志与错误治理**：
  - 消除 `main.go` 启动时 `SyncFromFile` 与 `RecompileAndApply` 的静默吞错，捕获异常并输出结构化 `slog.Error` 关键告警日志，防止服务携带脏配置盲跑。
- **单元测试解耦与故障注入覆盖**：
  - 抽象定义 `ConfigFileSynchronizer` 细粒度接口，解除 `UserService` 对物理配置服务的具体指针强耦合；
  - 构建 Mock Xray 管理器与配置同步器，新增覆盖 Xray 超时、写盘失败与状态回滚断言的 15 项故障注入自动化测试。
- **CI 质量门禁加固与文档精简**：
  - `.github/workflows/ci.yml` 引入 `npm run typecheck`（`vue-tsc --noEmit`）门禁，阻断深层 TypeScript 类型错误；
  - CI 集成测试流程中自动下载安装官方 Linux Xray-core 二进制，激活原本被跳过的真实内核 gRPC 集成测试；
  - 精简 `README.md`，剥离历史更新日志至独立的 `CHANGELOG.md`。

---

## v2.6.2 (2026-10) - 日志解析引擎重构：淘汰全局硬正则与实现零堆分配扫描器
- **淘汰全局硬编码正则表达式**：
  - 移除 `internal/adapter/xray/log_reader.go` 中的重量级 `accessLogRe` 与 `errorLogRe` 正则引擎，消除正则状态机回溯与中间切片分配；
- **高性能纯字节零拷贝解析 (Zero-Allocation Tokenizer / Scanner)**：
  - 重构 `ParseLogLineFast`，直接在 `[]byte` 切片上定位时间戳、来源 IP 与目标地址，消除此前 `string(trimmed)` 完整行拷贝，大幅提升日志逆向检索与过滤吞吐量；
  - 实现基于确定性状态机与游标定位的 `ParseAccessLogLine` 与 `ParseErrorLogLine`，访问日志解析吞吐提升 13.2x，错误日志解析吞吐提升 5.6x；
- **全边缘场景向下兼容**：
  - 完整兼容带/不带毫秒时间戳（`YYYY/MM/DD HH:MM:SS.ms`）、`from ` 客户端前缀、IPv6 方括号端口（`[::1]:port`）、`tcp:` / `udp:` 协议剥离；
  - 完整支持 `[inbound -> outbound]`、`[inbound >> outbound]`（Fallback 兜底）及单 Tag 路由；
  - 错误日志支持日志级别规范化映射（`WARNING` -> `WARN`）、进程 `[PID]` 过滤、SmartTip 智能诊断规则以及特殊 DNS 日志回退。
- **基准测试与性能门禁**：
  - 新增 `BenchmarkParseAccessLogLine`、`BenchmarkParseErrorLogLine` 与 `BenchmarkParseLogLineFast` 基准测试，单次访问日志解析堆分配降至仅保留目标结构体单一对象（1 allocs/op）。

---

## v2.6.1 (2026-10) - 本机免认证代理可观测性重构、日志解析协议升级与时序修复
- **本机免认证代理流量核算与双计防御**：
  - 解耦入站流量与用户归属，移除单租户入站流量强制复制到用户的旧机制，实现原生用户与入站各自独立原子核算，杜绝同轮次双重计算；
  - 遗留 `BoundUserEmail` 标记为 deprecated 仅作向后兼容，移除绑定用户存在性硬校验，避免历史绑定阻塞入站维护；
  - 仪表盘 `totalUp/totalDown` 修正为现存业务入站（含禁用入站，排除管理 `api`）累计总和，彻底拆分主机物理网卡流量与代理流量展示。
- **Xray-core 原生日志解析与分流兼容性增强**：
  - 支持 Xray-core 规则命中（`->`）与默认出口兜底（`>>` Fallback）双重路由标记识别；
  - 兼容现网 Xray-core v26+ 输出的 `from tcp:` / `from udp:` 客户端协议前缀，解决结构化访问记录解析为 0 的缺陷；
  - 支持单标签路由（`[tag]`）与未注册动态 Tag 回显候选。
- **前端控制台时序与状态治理**：
  - 修复 `LogsView.vue` 路由参数 `immediate: true` 监听器在挂载期解构未定义旧状态导致的运行时崩溃；
  - 为 `UserTrafficModal` 补齐即时加载时序与 `Loader2` 骨架状态，解决首次打开弹窗显示空白、需手动切换天数 Tab 的问题；
  - 入站管理视图新增累计上下行流量直观展示，支持一键带参跳转至日志溯源。

---

## v2.6.0 (2026-09) - 架构整理、通用类抽取与 Xray-core 特性扩展性升级
- **统一流配置强类型访问器 (InboundStreamAccessor)**：
  - 抽取 `xray.InboundStreamAccessor`，统一消除配置编译（`compiler`）与 gRPC 动态下发中的重复手写解包；
  - 强类型收敛 Vision Flow 与 Shadowsocks Cipher 提取逻辑，防止非 TCP 传输产生 flow 参数污染；
- **gRPC 账户构建策略中心 (AccountBuilder Registry)**：
  - 抽象策略模式与并发安全注册中心，剥离 `grpc_client.go` 单体大 switch；
  - 支持多协议零侵入注册与独立隔离单测，无缝应对未来 Xray 协议扩展；
- **底层通用密码学基础设施下沉 (internal/pkg/crypto)**：
  - 下沉 Curve25519 密钥生成、公钥推导与 VLESS UUID 路由掩码替换至通用基础设施包，理顺 clean architecture 依赖层级；
- **订阅协议通用原语抽取与瘦身**：
  - 抽取 `ValidateBaseNode`、`AttachClashTLS`、`AttachSingBoxTLS`、`BuildNodeQueryParams` 等通用原语，各协议实现代码量精简 50%+；
- **前端冗余死代码清洗**：
  - 移除前端 `subscription.ts` 中脱节的 `generateNodeLink` 代码，统一以后端分享契约为唯一事实源。

---

## v2.6.0-beta.1 (2026-09) - 控制台现代化重塑、巨石视图解耦与 TypeScript 质量加固
- **前端控制台体系升级与视觉重塑 (Console Redesign)**：
  - 核心设计语言全面转向高密度现代控制台风格，收敛中性语义令牌，剔除冗余渐变；
  - 视图结构重构为“Table-First + 侧边巡检/配置抽屉”模式，提升信息密度与交互效率；
  - 凭据提取 Gateway 与登录视图支持端侧主题自适应与平滑交互。
- **巨石视图拆解与性能按需加载 (Architecture & Performance)**：
  - 沉淀通用 UI 原语组件（`FormField`、`SectionCard`、`Modal`），消除跨视图重复代码；
  - 拆解千行巨石视图 `InboundsView.vue` 与 `UsersView.vue`，转化为清晰的事件/状态调度器；
  - 将重型表单抽屉及弹窗重构为 `defineAsyncComponent` 异步按需加载，隔离第三方重量库，降低首屏 Chunk 体积。
- **用户领域层模型与安全参数清洗**：
  - 抽取 `UserSubscriptionService` 统一收敛 VLESS/VMess/Trojan/Shadowsocks 订阅拼接逻辑；
  - 实现纯函数领域清洗器，严格过滤无意义监控与异构协议参数，杜绝前端浅拷贝提交脏数据。
- **TypeScript 严格约束与 LSP 规范治理**：
  - 前端工程开启 `noUnusedLocals`、`noUnusedParameters` 等严格检查；
  - 修复存量未用变量与类型穿透问题，实现 `npm run typecheck` 零告警、零错误门禁。
- **Cloudflare 多机漫游寻呼网关与多 VPS 弹性调度**：
  - 支持 Cloudflare Worker / Pages 作为统一凭据漫游寻呼网关，穿透真实客户端 IP 并放宽多机漫游兑换限流。

---

## v2.5.0 (2026-09) - SubRoute 节点分流线路细粒度用户权限隔离
- **领域模型与强隔离数据契约**：
  - `domain.SubRoute` 扩展 `AllowedUsers` 字段，实现纯方法 `CanAccess(email)`，缺省/空数组全员开放向后兼容；
  - `XrayRoutingRule` 扩展 `User` 属性，编译器在接入规则中精准注入白名单，物理阻断伪造 `routeId` 越权。
- **展示层双重过滤与授权状态动态联动**：
  - 订阅生成与节点转换依据用户邮箱严格过滤分流线路，全未命中严格下发 0 个节点；
  - 入站管理视图重构授权用户统计逻辑，与当前活跃用户集合实时联动响应；
  - 分流线路管理抽屉支持可视化用户选择、一键全选与全员开放重置。

---

## v2.4.2 (2026-09) - Telegram Bot 交互体验与运维管控能力增强
- 支持 InlineKeyboard 原地无缝刷新、用户启停/重置与快捷开号；
- 优化提示词与缓存前缀配置，提升运维响应速度与操作体验。

---

## v2.4.1 (2026-09) - Reality 伪装域名网络合规性巡检与告警体系
- 支持 TCP 连通性、TLS 1.3 协商、ALPN (h2/http1.1) 支持度自动核验；
- 排查公共 CDN 拦截特征与回落风险，实现证书域名匹配校验与临期 (<7天) 预警；
- 后台 12 小时自适应轻量轮询调度与读写锁缓存，支持面板 API 手动即时触发并联动 Telegram 告警。

---

## v2.4.0 (2026-09) - 架构分层解耦、后端高并发性能加固与前端分包优化
- 前端路由视图全面采用标准 ES 动态懒加载，配置 Rollup 细粒度分包；
- 限流中间件升级为 16 分段哈希互斥锁，降低高频请求锁竞争；
- 日志分页检索引入 `sync.Pool` 缓冲区池化复用；系统看板聚合引入 2 秒 TTL 双检缓存；
- Service 层全面定义消费端 Caller-scoped 接口，解耦具体 Adapter 依赖。

---

## v2.3.1 (2026-09) - 网卡回环过滤、节点流量防清零与移动端历史图表重构
- 重构网卡 I/O 采集算法，过滤回环网卡（lo、loopback）及虚拟/容器网卡；
- 修复入站节点在保存修改基本属性时，因已有计数缺失导致流量被误置零的问题；
- 修复移动端下用户流量历史弹窗超出屏幕与图表重叠错位问题。

---

## v2.3.0 (2026-09) - 纯契约协议解耦、核心依赖精简与 SQLite 存储性能优化
- 自研轻量级 Protobuf/gRPC 协议契约包（`internal/adapter/xray/proto`），剥离 xray-core 运行时依赖树；
- 纯 Go 标准库实现 RFC 6238 TOTP 认证器，替换第三方外部依赖；
- 启用 SQLite WAL 与 synchronous=NORMAL，流量采集同步支持单原子事务批量落盘。
