# Xray Decoupled Panel (解耦运维监控与分流管理面板)

<p align="center">
  <img src="https://img.shields.io/badge/Version-v2.6.0-indigo?style=flat-square" alt="Version">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Vue-3.4+-4FC08D?style=flat-square&logo=vue.js" alt="Vue Version">
  <img src="https://img.shields.io/badge/Architecture-Clean%20Architecture-blue?style=flat-square" alt="Clean Architecture">
  <img src="https://img.shields.io/badge/Live%20Demo-GitHub%20Pages-success?style=flat-square&logo=github" alt="Live Demo">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License">
</p>

<p align="center">
  <b>在线免部署体验 Demo：</b>
  <a href="https://coding-leaf.github.io/xray-panel/" target="_blank">
    <b>https://coding-leaf.github.io/xray-panel/</b>
  </a>
</p>

---

**Xray Decoupled Panel** 是基于 Go 与 Vue 3 构建的轻量级 Xray 运维监控、节点分流与订阅管理面板。系统采用控制面与数据面解耦架构，通过 Xray 官方原生 gRPC API（HandlerService / StatsService）直接与运行中内核通信，支持用户毫秒级热增删、多入站与多出站分流编排、实时流量采集与多客户端聚合订阅分发。

---

## 核心架构与功能特性

- **入站分流线路与细粒度用户权限隔离 (SubRoute User Isolation)**：
  - 单入站端口多落地分流线路（基于 VLESS 16-bit routeId 单端口多出口机制）；
  - 支持为每条分流线路指定授权用户白名单，实现订阅展示过滤与 Xray 引擎内核 Layer 3 双重物理强隔离；
  - 授权用户数与活跃用户状态实时联动响应，管理抽屉提供可视化用户分配器，支持一键全选与全员开放，存量配置无缝平滑兼容。
- **Xray 原生 gRPC 纯契约解耦与状态热重载**：
  - 基于轻量级 Protobuf/gRPC 契约包与 Xray API 通信，剥离运行时多余依赖；
  - 用户增删与状态变更通过 gRPC 协议毫秒级下发，长连接不中断；
  - 仅在入站、出站、全局路由分流规则或 DNS 发生结构性变更时执行平滑内核重载。
- **纯 Go SQLite ACID 事务持久化与批量单事务落盘**：
  - 采用纯 Go 嵌入式存储引擎（glebarez/sqlite），无 CGO 编译依赖；
  - 启用 WAL（Write-Ahead Logging）模式并优化 synchronous=NORMAL，单轮流量采集聚合为单原子事务落盘，降低磁盘写放大与锁等待；
  - 具备状态协调机制：gRPC 下发与本地事务强一致绑定，防止状态不一致。
- **冷启动双轨落盘容灾与自动配置编译**：
  - 系统关机、退出或配置变更时，将 SQLite 用户集合与路由拓扑编译落盘至物理 config.json，保证冷启动独立恢复。
- **多协议多态账户适配**：
  - 原生支持 VLESS (Vision/Reality)、VMess、Trojan、Shadowsocks (2022/AEAD) 等协议的多态序列化与分享链接转换；
  - 淘汰已废弃的 alterId 字段，遵循现代 Xray 规范（alterId=0 / AEAD 加密）。
- **统一服务生命周期编排 (app.Service)**：
  - 基于 errgroup 统一调度 HTTP Server、Telegram Bot Poller 与流量定时轮询任务；
  - 支持系统退出信号拦截、优雅停机（Keep-Alive 连接排空）与有序资源释放。
- **高并发性能优化与内存缓冲复用**：
  - 接口防刷限流中间件采用 16 分段哈希锁（Sharded Mutex），消除高频并发互斥锁竞争；
  - 日志逆向分页流式扫描采用 sync.Pool 缓冲池复用，降低垃圾回收压力；
  - 系统看板与指标统计支持读写双重检查（DCL）轻量缓存，避免前端轮询高频全表扫描；
  - 瞬时速率统计支持批量持锁更新与过期空闲对象自动淘汰。
- **前端按需加载与细粒度分包优化**：
  - 视图组件全面采用标准 ES 动态导入懒加载；
  - 配置 Rollup manualChunks 细粒度分包，核心运行时、图表库、图标库与工具库独立分块，消除大产物打包告警并提升首屏加载性能。
- **带外凭据分发与阅后即焚系统 (Ticket Distribution)**：
  - 采用 6 位 Crockford Base32 编码生成高熵提件码，支持中立凭据安全分发；
  - 提供 CAS 原子校验、物理磁盘抹除、客户端超时自毁与 Cache-Control: no-store 传输防缓存。
- **单二进制交付**：
  - 前端基于 Vue 3 + Tailwind CSS 构建，所有静态资源通过 Go embed 编译进单一二进制文件，无外部静态资源依赖。
- **实时监控与在线追踪**：
  - 采集主机 CPU、内存、磁盘与双向网卡吞吐速率（自动过滤回环及虚拟网卡）；
  - 基于 StatsService 定时轮询，精准追踪用户瞬时速率与在线连接状态。
- **聚合订阅与二维码分发**：
  - 采用独立 subToken 鉴权，支持 Base64、Clash / Mihomo、Sing-box 等主流订阅格式导出与二维码扫码。
- **安全防护与访问控制**：
  - 支持 TOTP 双因素认证（2FA）、高熵 JWT 签名密钥、敏感配置字段脱敏与 10MB 请求体上限防护。

---

## 更新日志

### v2.6.0 (2026-09) - 架构整理、通用类抽取与 Xray-core 特性扩展性升级
- **统一流配置强类型访问器 (InboundStreamAccessor)**：
  - 抽取 `xray.InboundStreamAccessor`，统一消除配置编译（`compiler`）与 gRPC 动态下发中的重复手写解包；
  - 强类型收敛 Vision Flow 与 Shadowsocks Cipher 提取逻辑，防止非 TCP 传输产生 flow 参数污染；
- **gRPC 账户构建策略中心 (AccountBuilder Registry)**：
  - 抽象策略模式与并发安全注册中心，彻底剥离 `grpc_client.go` 单体大 switch；
  - 支持多协议零侵入注册与独立隔离单测，无缝应对未来 Xray 协议扩展；
- **底层通用密码学基础设施下沉 (internal/pkg/crypto)**：
  - 下沉 Curve25519 密钥生成、公钥推导与 VLESS UUID 路由掩码替换至通用基础设施包，理顺 clean architecture 依赖层级；
- **订阅协议通用原语抽取与瘦身**：
  - 抽取 `ValidateBaseNode`、`AttachClashTLS`、`AttachSingBoxTLS`、`BuildNodeQueryParams` 等通用原语，各协议实现代码量精简 50%+；
- **前端冗余死代码清洗**：
  - 移除前端 `subscription.ts` 中脱节的 `generateNodeLink` 代码，统一以后端分享契约为唯一事实源。

### v2.6.0-beta.1 (2026-09) - 控制台现代化重塑、巨石视图解耦与 TypeScript 质量加固
- **前端控制台体系升级与视觉重塑 (Console Redesign)**：
  - 核心设计语言全面转向 Vercel / Linear 风格高密度开发者控制台体系，收敛中性语义令牌，剔除花哨冗余渐变；
  - 视图结构重构为“Table-First + 侧边巡检/配置抽屉”模式，信息密度与可操作性大幅提升；
  - 凭据提取 Gateway 与登录视图支持端侧主题自适应、沉浸式卡片和平滑交互。
- **巨石视图拆解与性能按需加载 (Architecture & Performance)**：
  - 沉淀通用 UI 原语组件（`FormField`、`SectionCard`、`Modal`），消除跨视图重复代码并统一交互规范；
  - 拆解千行巨石视图 `InboundsView.vue`（由 1973 行精简至 193 行）与 `UsersView.vue`（由 1935 行精简至 140 行），转化为清晰的事件/状态调度器；
  - 将重型表单抽屉（`InboundFormDrawer`、`UserFormDrawer`）及弹窗（`UserShareModal`、`UserTrafficModal`）重构为 `defineAsyncComponent` 异步按需加载，隔离第三方重量库（如 `qrcode.vue`），大幅降低首屏 Chunk 体积。
- **用户领域层模型与安全参数清洗**：
  - 抽取 `UserSubscriptionService` 统一收敛 VLESS/VMess/Trojan/Shadowsocks 订阅拼接逻辑；
  - 实现纯函数领域清洗器 `sanitizeUserPayload` 与 `sanitizeInboundPayload`，严格过滤无意义监控与异构协议参数，杜绝前端浅拷贝提交脏数据。
- **TypeScript 严格约束与 LSP 规范治理**：
  - 前端工程全面开启 `noUnusedLocals`、`noUnusedParameters`、`noFallthroughCasesInSwitch` 等严格检查；
  - 接入 Volar 接管模式与统一 VSCode settings，修复存量未用变量与类型穿透问题，实现 `npm run typecheck` 零告警、零错误门禁。
- **Cloudflare 多机漫游寻呼网关与多 VPS 弹性调度**：
  - 支持 Cloudflare Worker / Pages 作为统一凭据漫游寻呼网关，穿透真实客户端 IP 并放宽多机漫游兑换限流。

### v2.5.0 (2026-09) - SubRoute 节点分流线路细粒度用户权限隔离
- **领域模型与强隔离数据契约**：
  - `domain.SubRoute` 扩展 `AllowedUsers` 字段，实现纯方法 `CanAccess(email)`，缺省/空数组全员开放无缝向后兼容；
  - `XrayRoutingRule` 扩展 `User` 属性，编译器在 Layer 3 接入网关规则中精准注入白名单，物理阻断伪造 `routeId` 越权。
- **展示层双重过滤与授权状态动态联动**：
  - 订阅生成与节点转换依据用户邮箱严格过滤分流线路，全未命中严格下发 0 个节点；
  - 入站管理视图重构授权用户统计逻辑，与当前活跃用户集合实时联动响应；
  - 分流线路管理抽屉支持可视化用户选择、一键全选与全员开放重置。

### v2.4.2 (2026-09) - Telegram Bot 交互体验与运维管控能力增强
- **运维与管控交互升级**：
  - 支持 InlineKeyboard 原地无缝刷新、用户启停/重置与快捷开号；
  - 优化脚手架提示词与缓存前缀配置，提升运维响应速度与操作体验。

### v2.4.1 (2026-09) - Reality 伪装域名网络合规性巡检与告警体系
- **Reality 域名多维合规性主动巡检**：
  - 支持 TCP 连通性、TLS 1.3 强制握手协商、ALPN (h2/http1.1) 支持度自动核验；
  - 深度排查公共 CDN (Cloudflare) 拦截特征与回落风险，实现证书域名匹配校验与临期 (<7天) 预警。
- **自动化后台调度与状态呈现**：
  - 后台 12 小时自适应轻量轮询调度与读写锁缓存，支持面板 API 手动即时触发；
  - 节点视图与仪表盘提供直观合规状态徽标与异常指引，联动 Telegram 告警通知。

### v2.4.0 (2026-09) - 架构分层解耦、后端高并发性能加固与前端分包优化
- **前端包体与加载优化**：
  - 将所有路由视图组件重构为标准 ES 动态懒加载；
  - 配置 Vite/Rollup manualChunks 细粒度代码拆分（vendor-vue, vendor-icons, vendor-qrcode, vendor-utils），彻底消除 500kB 单 chunk 警告，核心 runtime 产物压缩至约 103kB。
- **后端数据流与并发性能加固**：
  - 限流中间件升级为 16 分段哈希互斥锁，大幅降低高频 API 请求下的锁冲突概率；
  - 日志逆向分页检索引入 sync.Pool 64KB 缓冲区池化复用与不可变截断拷贝，消除频繁 GC 压力；
  - 系统看板监控聚合引入 2 秒 TTL 读写双检快照缓存，阻断前端短轮询对 SQLite 的重复全表扫描；
  - 实时速率统计由逐用户抢占锁重构为单次加锁批量更新，并自动淘汰过期空闲条目，杜绝内存泄漏风险。
- **Clean Architecture 架构分层解耦**：
  - Service 层全面定义消费端 Caller-scoped 接口，彻底切断对 Adapter 具体实现的直接反向依赖；
  - 提炼 AuthService 与 SettingService，剥离 HTTP Handler 内的复杂用例编排与跨模块联动，纯化请求接入层；
  - 将节点转换与订阅配置生成逻辑归位至 internal/protocol 包，移除废弃孤立函数与冗余类型别名，保证代码 KISS 极简。

### v2.3.1 (2026-09) - 网卡回环过滤、节点流量防清零与移动端历史图表重构
- **主机网卡监控吞吐校准与回环过滤**：
  - 重构网卡 I/O 采集算法，过滤回环网卡（lo、loopback）及虚拟/容器网卡（Docker、veth、Bridge、CNI 等）；
  - 采样快照引入读写并发锁保护，保障主机吞吐与网络累计指标的线程安全与原子一致性。
- **节点配置修改防流量清零**：
  - 修复入站节点在保存修改基本属性时，因上层未带计数导致已有入站 up_bytes 和 down_bytes 被误覆盖置零的问题；
  - 持久化层应用 Omit 安全隔离策略，并补齐 CRUD 与增量流量统计单测。
- **移动端历史流量图表交互与响应式优化**：
  - 修复移动端下用户流量历史弹窗超出屏幕与图表重叠错位的问题；
  - 表格表头增加 sticky 吸顶，柱状图适配触摸屏点击高亮与数值常驻冻结。
- **订阅凭据展示安全收敛**：
  - 移除用户列表快捷一键直连订阅复制，引导使用聚合导出、二维码或阅后即焚分发。

### v2.3.0 (2026-09) - 纯契约协议解耦、核心依赖精简与 SQLite 存储性能优化
- **Xray gRPC 纯契约化解耦**：
  - 自研轻量级 Protobuf/gRPC 协议契约包（internal/adapter/xray/proto），兼容 Xray 原生 Wire 格式；
  - 剥离 xray-core 运行时及其依赖树，依赖体积精简 75%，大幅收敛攻击面；
- **核心组件去冗余**：
  - 纯 Go 标准库实现 RFC 6238 TOTP 认证器，拔除第三方库与多余图片生成依赖；
  - 纯 Go 泛型并发安全 TTL 缓存（internal/pkg/cache），替代旧版反射缓存；
  - 升级 yaml.v3 并收敛限流器为官方令牌桶算法；
- **SQLite 批量事务落盘**：
  - 启用 synchronous=NORMAL，消除 WAL 模式下每次提交的强制落盘开销；
  - 流量同步支持单事务批量落盘（BatchSyncTraffic），将单轮周期的多次数据库操作收敛为单一原子事务。

---

## 架构分层 (Clean Architecture)

详细的业务领域模型、单端口多出口分流与双轨运行时架构请参阅：
[架构与系统逻辑全景图解 (docs/ARCHITECTURE_AND_LOGIC.md)](docs/ARCHITECTURE_AND_LOGIC.md)

```
internal/
├── app/               # 应用生命周期契约与服务抽象 (Service, ServiceFunc)
├── config/            # 启动配置装载 (flag + 环境变量)
├── domain/            # 业务领域实体与纯业务契约 (Inbound, Outbound, User, SubRoute, Ticket, AuditLog, Reality 等)
├── protocol/          # 多协议注册表、节点转换与分享链接生成 (vless/vmess/trojan/shadowsocks/hysteria2/socks)
├── service/           # 核心用例编排 (Auth/Setting/Config/User/Sub/Monitor/Alert/GeoData/Reality/Ticket/AuditLog 等)
├── sub/               # 聚合订阅导出器 (Base64, Clash/Mihomo, Sing-box)
├── adapter/           # 外部基础设施与系统适配实现
│   ├── xray/          # gRPC Client, XrayCompiler, Config Parser, Supervisor 与日志读取
│   │   └── proto/     # 自研轻量 Protobuf/gRPC 契约客户端定义
│   ├── repository/    # SQLite & GORM 仓储实现（WAL 模式加固）
│   ├── telegram/      # Telegram Bot 适配器与告警通知
│   ├── reality/       # REALITY 伪装域名探测与合规评估
│   └── monitor/       # gopsutil 硬件性能指标采集
├── delivery/          # 传输接入与服务暴露层
│   ├── http/          # RESTful API、Gin 路由、优雅停机 Server
│   │   └── middleware/# JWT 鉴权、限流与防护中间件
│   └── cron/          # 流量同步 (TrafficSyncJob) 与 Reality 巡检调度 (RealitySyncJob)
└── pkg/               # 基础设施通用工具库
    ├── cache/         # 泛型并发安全 TTL 缓存
    ├── jsonc/         # 带注释 JSON 解析
    ├── jwt/           # JWT 签发与校验
    ├── logger/        # 结构化日志初始化
    └── totp/          # 纯 Go RFC 6238 TOTP 认证器
```

---

## 快速开始

### 方式一：Linux Systemd 安装（推荐生产环境）

确保系统已安装 Xray-core 并正常运行：

```bash
# 1. 下载源码并进入目录
git clone https://github.com/coding-leaf/xray-panel.git
cd xray-panel

# 2. 执行部署安装脚本
sudo bash deploy/install.sh
```

一键脚本将完成：
1. 注册并配置 `/usr/local/xray-panel` 工作目录；
2. 注册并启动 `/etc/systemd/system/panel.service` 系统守护进程；
3. 配置开机自启并输出面板访问地址。

---

### 方式二：手动运行与命令行参数

```bash
# 启动面板
./panel -port 9000 -xray-config /usr/local/etc/xray/config.json -xray-grpc 127.0.0.1:8080
```

#### 常用启动参数与环境变量：

| 参数名 | 环境变量 | 默认值 | 描述 |
| :--- | :--- | :--- | :--- |
| `-port` | `PANEL_PORT` | `9000` | 面板 Web 监听端口 |
| `-xray-grpc` | `XRAY_GRPC_ADDR` | `127.0.0.1:8080` | Xray 核心 API gRPC 监听地址 |
| `-xray-config` | `XRAY_CONFIG_PATH` | 平台自动探测（Linux: `/usr/local/etc/xray/config.json`，否则 `config.json`） | Xray 核心主配置文件路径 |
| `-xray-bin` | `XRAY_BIN_PATH` | 平台自动探测（Linux: `/usr/local/bin/xray`，Windows: `xray.exe`） | Xray 核心二进制程序路径 |
| `-db` | `DB_PATH` | `data/panel.db` | 面板 SQLite 数据库文件路径 |
| `-jwt-secret` | `PANEL_JWT_SECRET`（兼容 `JWT_SECRET`） | 留空则自动生成高熵密钥并入库 | 管理员鉴权 JWT 签名密钥 |
| `-log-level` | `LOG_LEVEL` | `info` | 日志级别（`debug` / `info` / `warn` / `error`） |
| `-log-json` | 无 | `false` | 是否以 JSON 结构化格式输出日志 |
| `-service` | `SERVICE_NAME` | `xray` | Xray 的 systemd 服务名 |
| `-public-url` | `PUBLIC_URL` | `http://127.0.0.1:9000` | 用于生成订阅链接的面板公开访问地址 |
| `-v` | 无 | `false` | 打印版本信息后退出 |
| `-version` | 无 | `false` | 打印版本信息后退出（`-v` 同义长参数） |

---

## 本地开发与编译

### 前置要求
- Go 1.26+
- Node.js 20+ & npm

```bash
# 1. 克隆代码仓库
git clone https://github.com/coding-leaf/xray-panel.git
cd xray-panel

# 2. 纯前端 Mock 模式本地调试 (无需 Xray 核心)
cd web
npm install
npm run dev:demo

# 3. 生产前端构建
npm run build
cd ..

# 4. 运行全量单元测试与编译生产二进制
go test -v ./...
go build -ldflags="-s -w" -o panel .
```

---

## 安全与加固建议

1. **配置反向代理**：建议使用 Nginx / Caddy 申请 SSL 证书反向代理面板 Web 端口；
2. **首次登录修改密码**：初始账号为 `admin` / `admin123`，登录后请立即进入系统设置修改密码并启用 TOTP 双因素认证 (2FA)；
3. **隔离 gRPC API 通信端口**：Xray 的 gRPC API 监听地址建议限定为 `127.0.0.1:8080`，避免对公网开放；
4. **防火墙规则**：仅对外开放业务代理端口与 Web 管理端口。

---

## 开源许可证

本项目基于 [MIT License](LICENSE) 协议开源。
