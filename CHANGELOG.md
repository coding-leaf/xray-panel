# Changelog (更新日志)

本项目遵循规范记录所有重要版本的功能迭代、性能重构与缺陷修复。

---

## v2.6.8 (2026-11) - 全栈架构并发清零、批量操作与 SQLite 性能飞跃、网络防线加固与前端瘦身

- **高并发架构一致性与竞态清零 (Concurrency & State Hardening)**：
  - **Xray gRPC 与进程守护零竞态**：重构 `GRPCClient.getClients` 在读锁内原子拷贝客户端句柄，迁移至现代 `grpc.NewClient` 并注入 `keepalive` 心跳保活，彻底根治高并发下 `Close()` 重连时的空指针 Panic 与竞态；收敛 `SystemdSupervisor` 为单一 `RWMutex` 且将外部 `systemctl`/`xray -version` 调用全部移出锁外；
  - **配置双源读写串行化与禁吞错**：为 `ConfigService` 引入业务级互斥锁与 `*Locked` 无死锁内部管线，杜绝并发增删用户与修改出站/路由/DNS 时的更新丢失及重启“已删用户幽灵复活”，并显式传播全部仓储/同步错误；
  - **无状态 uTLS 默认指纹传递**：新增 `InboundToNodeConfigWithDefaultFP` 等无状态转换入口，消除并发拉取订阅时包级全局默认指纹互相污染；
  - **Telegram 客户端竞态消除与限流**：为 Bot 配置 45s 超时客户端，长轮询浅拷贝独立 `pollBot` 挂载上下文取消传输层，并引入容量 8 的消息处理并发信号量。
- **后端、SQLite 与高频操作性能飞跃 (Backend & Storage Performance)**：
  - **批量用户操作提速 10~50 倍**：重构批量续期、批量重置流量、批量启停并新增 `POST /api/users/batch-delete` 批量删除接口，循环内仅同步 DB + gRPC 内存态，末尾统一执行 **1 次** 配置重编译写盘与 `xray -test` 校验，告别 1C1G 小内存机器上的进程 Fork 风暴；
  - **配置单次读盘与日志路径缓存**：单次保存配置从重复读取解析 `config.json` 5 次压缩至 **1 次** 并直接复用原始字节写快照；`ConfigManager` 内存缓存日志文件路径（写配置时自动刷新），消除日志页高频轮询的重复磁盘解析；
  - **订阅接口 SQL 缩减 85% 与按需编码**：`/sub` 单次请求由串行 11~12 次单键查库合并为 1 次 `GetAll` 批量读取，并按客户端请求的 `clash` / `sing-box` / `v2ray` 格式单次编码；
  - **入站 200ms 并发探测与 Hysteria2 修正**：`ListInbounds` 跳过停用节点并改为并发 TCP 探测（支持 `:: -> ::1`），对 `hysteria2` 及纯 UDP `dokodemo-door` 免除无效 TCP 拨号，彻底解决纯 UDP 节点在列表恒显示“离线”的误报；
  - **SQLite 批量原子 UPSERT 与历史表自裁剪**：每日流量同步采用 `clause.OnConflict` 200 条分批原子累加 UPSERT，DSN 启用 `temp_store(MEMORY)` 与 `cache_size(-8000)`；新增 `config_snapshots`（保留最新 100 条）与 `traffic_logs`（自动清理 180 天前记录）容量自裁剪，删除用户时级联清理关联提件码与历史流量日志。
- **网络防线、边缘网关与静态缓存基线 (Network & Edge Security)**：
  - **防伪造头绕过限流与防 OOM**：`GetRealClientIP` 仅在直连对端属于受信代理（回环或配置白名单网段）时才解析转发头，公网直连强制取真实 socket IP，且每个限流分片设 4096 容量硬上限自动淘汰旧桶；
  - **Nginx 四层真实访客 IP 透传与长连接基线**：重构 `deploy/nginx-sample.conf`，为面板分流启用 `proxy_protocol` 还原真实客户端 IP（避免全员坍缩为 `127.0.0.1` 引发连坐误封），同时对 REALITY 分支剥离协议头保持零破坏兼容，补齐 `upstream keepalive 32`、`gzip` 及 Linux BBR/QUIC 缓冲区调优模板 `deploy/99-xray-panel-sysctl.conf`；
  - **REALITY 原生 HTTP/2 探测与边缘流释放**：`DefaultRealityProber` 在协商出 ALPN `h2` 时改用 `x/net/http2` 发送原生二进制请求采集真实响应头，精准识别 Cloudflare CDN 伪装目标；`http.Server` 补齐防 Slowloris 超时基线；Cloudflare Worker/Pages 多源站漫游非命中时立即 `cancel()` 响应流；SPA 静态资源分离 `/assets/*` 永久强缓存（`immutable`）与 `index.html` 协商缓存（`no-cache`），杜绝升级面板后浏览器旧缓存白屏。
- **前端轮询减负与依赖瘦身 (Web Console Polish)**：
  - 卸载未使用的 `pinia` 依赖与冗余 `pnpm-lock.yaml`，精简首屏核心包体积；
  - 修复 `RoutingView` 更新 GeoData 时卸载未清理定时器的泄漏，为仪表盘、日志、用户列表等全部轮询加上 `inFlight` 防重入锁，消除弱网下请求堆积卡顿；
  - `LogsView` 改用响应式视口断点条件渲染（仅挂载桌面表格或移动卡片中的一套）与稳定行 Key，最多减少一半日志 DOM 节点开销。

---

## v2.6.7 (2026-11) - 用户与订阅高可定制化、全链路动态版本号、三栏脉冲连线可视化拓扑与陈年隐患根治

- **三栏动态脉冲连线可视化拓扑中心 (Visual Pipeline Topology Canvas)**：
  - 将 `TopologyView` 重构为对标现代可观测性管线的**左（接入网关）➔ 中（分流通道/路由策略）➔ 右（落地出口）**三栏动态 SVG 贝塞尔连线画布，并保留一键切换回「传统分组池」的双模视图；
  - **全协议路由流向推导引擎 (`useTopologyGraph`)**：同时联动 `/inbounds`、`/outbounds` 与 `/routing`，自动推导 VLESS `SubRoute` 单端口多出口、指定 `inboundTag` 专属路由规则、全局分流规则以及 `Default Fallback` 默认直通兜底链路，让 `socks`、`trojan`、`shadowsocks` 等普通入站也能清晰展现完整流向，实现零孤岛入站与闲置出口智能识别；
  - **动态脉冲粒子与双向悬停溯源 (`useSvgBezier`)**：启用链路呈现虚线导轨 + 从左向右流动的发光脉冲圆点，支持鼠标悬停任意节点时双向 BFS 高亮整条上下游关联链路，并支持原位启停/删除线路、编辑网关/出口及闲置出口「⚡ 1步挂载为线路」；移动端自动降级为紧凑三级流式链路卡片。
- **用户全字段自由定制与热重载安全同步 (User Full Customizability & Hot-Reload Sync)**：
  - 解除编辑用户时的 `Email` 只读限制，支持直接修改用户标识、自定义或一键随机生成 `UUID` 与 `SubToken`；前端随机生成统一采用 128-bit CSPRNG（`crypto.getRandomValues` 生成 32 位十六进制 `SubToken` 与符合 RFC 4122 v4 的安全 `UUID`），与后端 `crypto/rand` 熵强度严格对齐；
  - 当用户的 `Email` 或 `UUID` 变更时，`UserService.UpdateUser` 先以旧快照调用 `RemoveUser(oldEmail)` 精确卸载 Xray 内存态旧凭据，再注入新凭据，并自动联动刷新所有 Inbound `SubRoute.AllowedUsers` 白名单，实现零孤儿凭据残留；
  - 新增 `domain.SupportsDynamicUser` 协议能力判定与 `proxy is not a UserManager` 容错保护：对 `socks`、`http`、`dokodemo-door` 等不支持 Xray gRPC 动态用户管理的入站协议自动旁路放行，彻底解决绑定本地 `socks` 入站时保存或修改用户报错回滚的问题；
  - 新增用户级 `Flow` 策略（`""` 跟随节点默认、`xtls-rprx-vision` 强制开启、`none` 显式禁用流控）及用户备注 (`Remark`) 字段，支持列表徽标展示与关键字搜索；
  - 到期时间支持「设为永不过期」、「按相对天数顺延 (+30/+90/+365天)」与「指定绝对到期日期」三种模式。
- **客户端订阅节点排序、置顶公告与导出参数定制 (Subscription Display & Export Controls)**：
  - `domain.Inbound` 与 `domain.SubRoute` 新增 `SortOrder` 排序权重（数值越小越靠前），确保客户端拉取订阅时节点顺序严格按权重确定性排列；
  - 支持在订阅列表最顶端自动注入虚拟状态提示节点（`📢 公告`、`📊 剩余流量`、`⏳ 到期时间`），支持在「系统设置」中一键开关或配置自定义置顶公告；
  - 支持自定义节点命名模板（`{remark}`, `{tag}`, `{protocol}`, `{port}`, `{index}`）、订阅显示名称 (`Profile-Title`)、更新间隔、全局默认 TLS 指纹、Clash 端口 (`mixed-port`) 与 `allow-lan` 开关；
  - 修复单节点分享链接 (`GetUserShareInfo`) 绕过系统默认 uTLS 指纹 (`sub_default_fingerprint`) 导致手机客户端（如 v2rayNG）始终写死 `fp=chrome` 的问题，并在入站 REALITY / TLS 配置中新增节点级「客户端 uTLS 指纹」覆盖选择器；
  - 修复全协议分享链接 `#` 锚点后缀（URI Fragment）使用 `url.QueryEscape` 将空格编码为 `+` 号导致 v2rayNG / NekoBox 等客户端显示加号的问题，统一改为符合 RFC 3986 的 `%20` 百分号编码。
- **面板版本号全链路动态绑定 (Dynamic Panel Version)**：
  - 新增 `-panel-version` 命令行参数、`PANEL_VERSION` 环境变量及 `-v` / `-version` 版本打印指令，优先级为 `CLI Flag > ENV > ldflags/默认版本`；
  - `/api/service/status` 响应新增 `panelVersion`、`commit`、`buildTime` 字段，移除前端顶栏 `v2.6` 硬编码，桌面端侧边栏与移动端抽屉实时绑定后端版本号。
- **前后端配置失配修复与全协议参数补齐 (Config Compiler & Multi-Protocol Enhancements)**：
  - **修复重启面板后空 Flow (`none`) 自动死灰复燃为 `xtls-rprx-vision` 的陈年缺陷**：修复 `syncFromRawJSON` 在启动同步 `config.json` 时抹除数据库 `inbound.SettingsJSON` 中的 `"flow": "none"` 标记及覆盖 `existingUser.Flow` 策略的问题；并限定仅 `vless` 协议展示流控徽标，消除 `socks` 节点误显 Vision 的问题；
  - **修复删除或改号后 `vlessRoute` 僵尸规则残留问题**：重构 `ensureSubRouteRules`，自动清理已删除/停用/改号的旧 `vlessRoute` 兜底规则，并就地同步最新的 `OutboundTag` 与 `AllowedUsers`；
  - **补齐入站全协议专属设置**：新增 VMess 专属配置卡片（`alterId`、`disableInsecureEncryption`）、Trojan 独立密码支持、Shadowsocks 转发网络 (`tcp,udp`/`tcp`/`udp`) 与 `ivCheck`、Socks UDP 绑定本地 IP (`ip`)、HTTP `allowTransparent` 与 `timeout`、以及 Dokodemo-door 透明代理核心参数 `followRedirect` 与 `timeout`，并在后端注册 `HTTPAdapter` 与 `DokodemoDoorAdapter`；
  - **补齐传输层/出站/路由/DNS 缺失配置**：入站抽屉补齐嗅探 `仅限路由 (RouteOnly)` 开关与 `destOverride` 多选框、`httpupgrade` / `mkcp` 传输层表单、TLS `alpn` 与 Fallback `alpn/path`；出站抽屉补齐 TLS `Fingerprint`、Reality `spiderX`、WireGuard `MTU` / `noKernelTun`；路由抽屉补齐按用户 (`User`) 分流与规则标识 (`Tag`)；
  - **修复 DNS 与 GeoData 配置丢失**：补齐 `XrayDNSConfig` 的 `clientIp`、`disableCache`、`disableFallback`、`hosts` 编译映射；`GeoDataService` 动态读取系统设置中的 `xray_geodata_dir`；
  - **Xray 真实二进制契约验收**：引入官方 Xray-core 真实二进制执行矩阵，验证全协议、传输层、DNS、路由与 gRPC 动态热操作 100% 零报错通过。

---

## v2.6.6 (2026-10) - 修复用户保存异常与虚拟节点转化规范、新增出站日志过滤与核心日志等级、移动端全量响应式适配

- **用户保存与失效 Inbound 移除容错治理 (Idempotent Node Removal)**：
  - 提炼 `isXrayIgnorableRemoveError` 判定，精准识别 `handler not found`、`failed to get handler`、`not found`、`not exist` 等错误；
  - 在用户更新旧标签清理、用户停用时从入站移除、物理删除用户以及 Saga 逆向补偿等链路中，将失效节点或凭据已清理的良性错误作为非致命状态放行并记录 Info 日志，彻底消除了历史失效节点残留导致用户保存报 500 回滚死锁的缺陷；
  - 修复 `UserSubscriptionService.getNodeTags` 空匹配时危险回退全选所有可用节点的隐患，杜绝自动给用户绑定全量入站。
- **单端口多出口 (SubRoute / 虚拟节点) 规范化转换**：
  - 严格限制 SubRoute（虚拟节点）展开仅对 `vless` 协议生效，消灭对 VMess/Trojan 等不支持 UUID 路由隐写的协议生成无效虚假节点的问题；
  - 规范未命名分流线路 fallback 为 `<InboundRemark/Tag> - 线路 #<RouteID>` 确定性命名，消除客户端导入同名冲突；
  - 订阅服务与节点转换器支持按 `route`（RouteID）过滤，并在前端 `SingleSub` 中准确注入 `?tag=<inboundTag>&route=<routeId>` 独立定向订阅链接；
  - 用户表格与抽屉的线路统计统一收敛至服务层，并联动 `CanAccess(user.Email)` 进行真实权限过滤。
- **出站日志过滤与 Xray 核心运行日志等级全栈能力 (Logs & Outbound Filtering)**：
  - 扩展 `domain.LogFilter` 与 `/logs` 接口，支持接收 `outbound` 出口标签与 `level` 日志级别过滤；
  - 在 `log_reader.go` 中实现零堆内存分配（`0 B/op, 0 allocs/op`）的 `matchOutboundTagBytes` 与 `matchLevelBytes` 逆向分块扫描算法；
  - 前端 `LogsView.vue` 访问模式新增“出站 (Outbound)”下拉选择器与点击徽章联动过滤；错误模式切换级别透传至后端加速扫描；
  - 赋予 `XrayCompiler` 并发安全的 `SetLogLevel` / `GetLogLevel`，并在生成 Xray 根配置时动态渲染 `log.loglevel`（支持 warning/info/debug/error/none）；
  - `SettingService` 持久化 `xray_log_level` 并驱动重新编译平滑热更新，启动时自动预载入；前端 `SettingsView.vue` 新增对应配置项。
- **移动端“桌面表格 + 移动卡片流”全量响应式适配 (Mobile UX & Safe Area)**：
  - `UserTable.vue` 新增移动端专用卡片流，支持整卡点击检视、多选复选框防冒泡、实时传输测速、配额进度条、到期状态与快捷操作栏自适应折行；
  - `InboundTable.vue` 新增入站移动端卡片流，直观呈现 Tag、端口、协议流控、REALITY 健康巡检状态与操作组；
  - `OutboundTable.vue` 修复桌面端横向截断问题（改为 `overflow-x-auto`），并补齐移动端出站卡片流；
  - `DashboardView.vue` 与 `UserTrafficModal.vue` 修复小屏溢出，增加自适应水平滚动；
  - `Drawer.vue` 与 `Modal.vue` 的 Footer 底部操作栏引入 CSS `env(safe-area-inset-bottom)`，彻底杜绝 iOS/Android 全面屏手势条遮挡按键。

---

## v2.6.5 (2026-10) - 路由单一事实源重构、跨机节点防截流、把手拖拽与移动端卡片流响应式

- **消灭路由阴阳两套规则与单一事实源重构 (Single Source of Truth)**：
  - 彻底拔除 `compiler.go` 底层遍历入站分流线路隐式追加 Layer 3 规则的历史逻辑；
  - 路由规则除系统核心 `api -> api` 通信保障外，100% 由用户在面板编排的统一规则池唯一决定（所见即所得）；
  - 彻底切断由于 `GetRoutingConfig` 回显编译产物导致前端保存时规则反复回吞、成倍翻倍复制的历史循环死锁；
  - 编译器与服务层均引入连续相同规则的幂等去重保护，自动清洗历史回环累积的堆叠冗余项；
  - 新建入站分流线路默认显式追加至规则表尾部（Tail），保证用户拥有完整的可见性与拖拽优先级编排权。
- **跨机节点隧道分流与防截流治理 (Cross-Node Tunnel Protection)**：
  - 移除 Layer 2 中对 `vlessRoute` 的粗暴静默过滤，忠实编译节点级协议分流；
  - 客户端显式指定跨机节点出口（如 US 出口 `vlessRoute: 2/3`）的流量在入口网关优先匹配直通落地机，彻底避免在 JP 被 `geosite:cn -> warp-out` 提前截胡导致 Google 会话出口分裂、ASN 不一致及风控问题。
- **路由面板交互与移动端卡片流响应式 (UI & Responsive Experience)**：
  - 引入 `GripVertical`（`⋮⋮`）拖拽把手，支持 HTML5 原生拖拽自由重排规则；
  - 规则优先级 `#序号` 徽标支持点击直接弹窗输入目标位次秒级直达调序；
  - 操作区加回尺寸适宜（`h-8 w-8`、图标 `w-4 h-4`）、高反差显色边框（`bg-neutral-800 border border-neutral-600 text-neutral-100`）的上下移动箭头按钮，杜绝小屏挤压；
  - 窄屏设备（`< sm / 640px`）自适应切换为紧凑优雅的卡片流（Card Stack）布局，杜绝横向滚动溢出。

---

## v2.6.4 (2026-10) - 修复入站出站 VLESS TCP 流控关闭持久化、回显及 Xray 内核协议对齐
- **流控状态持久化与回显准确性**：
  - 入站抽屉流控选项规范化为 `none`，彻底移除表单数据清洗中将空流控强制短路覆盖为 `xtls-rprx-vision` 的历史兜底逻辑；
  - 修复重新打开抽屉与协议/传输层联动（`onNetworkChange`、`onSecurityChange`）时丢失关闭状态并意外开启 Vision 的缺陷；
  - 出站表单（`populateOutboundForm`、`buildSettingsJSON`）同步对齐支持稳定关闭与持久化；
- **Xray-core 原生协议规范严格对齐与报错防御**：
  - 修复关闭流控时将内部标识 `"none"` 泄露至 Xray 导致 `VLESS "settings.flow" doesn't support "none"` 的核心校验拦截问题；
  - 严格遵循官方规范：在生成 Xray 核心入站/出站配置时，自动剥离非 Vision 的非法流控字段；
  - 订阅转换与客户端节点导出（`node_converter.go`）严格对齐，关闭流控时不生成 `flow` 参数，不被用户默认 flow 意外覆盖；
- **部署脚本增强**：
  - Windows 原生部署脚本 `deploy.ps1` 与通用脚本 `deploy.sh` 升级支持 3 台节点集群部署（`vps-jp`, `vps-us`, `vps-us2`）。

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
