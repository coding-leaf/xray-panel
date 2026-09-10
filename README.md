# 🚀 Xray Decoupled Panel (解耦运维监控与分流管理面板)

<p align="center">
  <img src="https://img.shields.io/badge/Version-v2.0.0-indigo?style=flat-square" alt="Version">
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Vue-3.4+-4FC08D?style=flat-square&logo=vue.js" alt="Vue Version">
  <img src="https://img.shields.io/badge/Architecture-Clean%20Architecture-blue?style=flat-square" alt="Clean Architecture">
  <img src="https://img.shields.io/badge/Live%20Demo-GitHub%20Pages-success?style=flat-square&logo=github" alt="Live Demo">
  <img src="https://img.shields.io/badge/Developed%20with-AI%20Assisted-8A2BE2?style=flat-square&logo=google-gemini" alt="Developed with AI">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License">
</p>

<p align="center">
  <b>🌟 在线免部署体验 Demo：</b>
  <a href="https://coding-leaf.github.io/xray-panel/" target="_blank">
    <b>https://coding-leaf.github.io/xray-panel/</b>
  </a>
</p>

---

**Xray Decoupled Panel** 是一款基于 Go 与 Vue 3 构建的高性能 Xray 运维监控、节点分流与聚合订阅管理面板。项目采用彻底解耦的运行时架构，通过 Xray 官方原生 gRPC API (`HandlerService` / `StatsService`) 与系统服务直接通信交互，将动态用户状态与物理静态 `config.json` 完全解耦，支持用户毫秒级热增删、多入站/多出站分流编排、用户实时流量监控与多客户端聚合订阅分发。

---

## 🤖 AI 辅助开发与审计声明 (AI Disclosure)

本项目在架构设计、深层次代码审计、安全漏洞排查及单元测试编写过程中，使用了 AI 工具（Google DeepMind / Antigravity Agent）进行结对开发与分析辅助。

- **客观性提示**：项目文档与代码中的部分实现可能带有 AI 辅助生成的痕迹。虽然所有关键变更均已经过静态检查、语法校验与自动化单元测试，但在实际生产环境中使用前，仍建议使用者根据自身网络环境与安全策略进行充分测试与验证；
- **合规声明**：请使用者自觉遵守当地法律法规，合理合法使用网络代理技术。

---

## ⚡ 核心功能与特性 (Features)

- 🚀 **Xray 原生 gRPC 运行时热重载与状态解耦**：
  - 深度接入 Xray 官方 gRPC API（`HandlerService` 与 `StatsService`），用户增删与状态变更通过 gRPC 协议毫秒级下发至运行中内核；
  - 运行中长连接（WebSocket、gRPC、VLESS REALITY）零中断、不掉线，彻底告别传统面板修改 `config.json` 并强制重启 Xray 进程引发的连接中断问题；
  - 仅在入站、出站、全局路由分流规则或 DNS 发生结构性变更时才执行平滑内核重载。
- 💾 **纯 Go SQLite ACID 事务持久化与 WAL 并发排队**：
  - 采用纯 Go 嵌入式存储引擎（`glebarez/sqlite`），彻底告别 CGO 跨平台编译依赖与环境地狱；
  - 启用 WAL（Write-Ahead Logging）模式与并发排队，确保高频流量采集写入与复杂业务查询互不阻塞，根治数据库死锁；
  - 具备状态协调与逆向补偿机制：gRPC 下发与本地事务强一致绑定，杜绝脏状态与幽灵用户。
- ❄️ **冷启动双轨落盘容灾与自动配置编译**：
  - 系统关机、守护退出或配置变更时，自动将 SQLite 动态用户集合与路由拓扑安全编译合并落盘至 `config.json`，确保宿主机断电或冷启动时 Xray 核心能够零延迟恢复全量用户独立运行。
- 🎯 **多协议多态账户适配与废弃字段彻底淘汰**：
  - 统一支持 VLESS (Vision/Reality)、VMess、Trojan、Shadowsocks (2022/AEAD) 等主流协议多态 Account 结构与订阅分享转换；
  - 彻底淘汰并移除 VMess 协议已废弃且存在安全隐患的 `alterId`，严格遵循现代 Xray 官方规范（强制 `alterId=0` / AEAD 加密）。
- 🔄 **统一服务生命周期编排 (`app.Service`)**：
  - 抽象标准 `app.Service` 契约，基于 `errgroup` 统一调度 HTTP Server、Telegram Bot Poller、Traffic Sync 定时轮询；
  - 支持系统退出信号拦截、优雅关机（HTTP 活跃 Keep-Alive 连接排空）与有序资源释放（先关 gRPC 连接，再释放数据库文件锁）。
- 🛡️ **并发流量统计防冲正**：
  - 采用字段隔离更新与重置时间窗口过滤，避免并发写入或旧周期增量冲正刚归零的流量。
- 🛡️ **带外高熵安全分发系统 (Out-of-Band Ticket Distribution & 阅后即焚)**：
  - 采用 6 位 Crockford Base32 编码（10.7 亿高熵空间），生成中立提件码，支持在微信、QQ 等境内即时通讯安全发送；
  - 客户端在专属【分布式网管接入点】输入提件码即可一键获取急救节点与长效订阅，完全脱敏、阻断爬虫主动探测；
  - **四维自毁保障**：秒级逻辑失效（CAS 原子校验）、磁盘物理抹除（次数耗尽即刻 DELETE + 60秒定时垃圾回收）、客户端绝对时钟自毁（防切后台休眠）、传输层 `Cache-Control: no-store` 强制防缓存。
- 🌐 **前后端彻底解耦 Headless API 与独立轻量单文件门户**：
  - 标准化 `POST /api/portal/claim` 接口并全局放行 CORS 跨域；
  - 提供开箱即用、零构建依赖的轻量单文件 HTML 模板（`deploy/standalone-portal.html`，~15KB 原生 Vanilla JS/CSS），支持直接部署于 Cloudflare Pages、GitHub Pages 或轻量虚拟主机中独立托管，源站 VPS 零暴露。
- ☁️ **Cloudflare 边缘中立门户网关与反探测代理**：
  - 提供工业级加固版 Cloudflare Worker / Pages 脚本（`deploy/cloudflare-worker-sub-proxy.js` 与 `deploy/cloudflare-pages/`）；
  - 根路径伪装为中立健康站点，Fixed-point 多重解码彻底杜绝路径穿越；智能识别腾讯、微信及自动化扫描爬虫并返回 200 OK 伪装页，保护域名免遭拦截报红。
- 🚀 **生产级平滑安装与热升级守护 ([deploy/install.sh](file:///home/yezisama/workspace/WorkSpace/xray-panel/deploy/install.sh))**：
  - 升级为 Linux `install -m 755` 原子覆盖写入，彻底消除生产环境二进制热更新时的 `Text file busy` (ETXTBSY) 错误；
  - 具备智能多路径自动寻径，并对敏感存储目录自动设置 `chmod 700` 权限安全收敛。
- 🌐 **单入站多通道分流 (VLESS Route)**：
  - 支持基于 VLESS UUID 映射的多出口分流机制，单个入站端口可按用户线路映射到不同落地出站，自动展开多通道独立订阅节点。
- 🎭 **纯前端 Mock 演示沙盒 (GitHub Pages)**：
  - 内置升级版 LocalStorage v3 数据仿真引擎，无需后端服务器即可完整体验多协议节点增删、通道编排与扫码订阅。
- 🧩 **单二进制交付**：
  - 前端基于 Vue 3 + Tailwind CSS 构建，所有静态资源通过 Go `//go:embed` 编译进单一二进制文件，无外部静态资源依赖，极简运维。
- 📊 **实时监控与在线追踪**：
  - 采集主机 CPU、内存、磁盘与双向网卡吞吐速率；
  - 基于 Xray gRPC `StatsService` 定时轮询，精准追踪用户瞬时速率与在线连接状态。
- 🔗 **聚合订阅与二维码分发**：
  - 采用独立 `subToken` 鉴权，支持通用 Base64、Clash / Mihomo、Sing-box 等主流订阅格式导出与二维码扫码。
- 🌍 **GeoData 规则库热更新**：
  - 支持在线拉取 `geoip.dat` 与 `geosite.dat` 规则库并平滑重载。
- 🤖 **Telegram 运维机器人**：
  - 支持 `/status`、`/traffic`、`/sub`、`/restart` 等交互式管理指令与告警推送（流量超额、系统过载、SSL 临期）。
- 🔒 **安全防护与访问控制**：
  - 支持 TOTP 双因素认证（2FA）、动态高熵 JWT 密钥持久化、公开订阅接口防刷限流及 10MB 请求体大小上限防护。

---

## 📝 最近更新日志 (v2.0.0 重大里程碑)

- **🎉 带外高熵安全凭据分发与阅后即焚系统 (Ticket Distribution System)**：
  - 新增 `internal/domain/ticket.go`、`internal/service/ticket_service.go` 与 `internal/delivery/http/handler_ticket.go`；
  - 生成 6 位 Crockford Base32 提取凭据（10.7 亿高熵组合），支持在微信/QQ等境内即时通讯安全发送；
  - 客户端在纯中立的【分布式网管接入点】输入提件码即可获取双轨配置（轨道 1：急救直连节点，轨道 2：长效自动更新订阅）；
  - **闭环四维阅后即焚**：
    1. **逻辑时效**：CAS 条件原子更新 `remaining_uses > 0 AND expires_at > now`，到期即拒；
    2. **物理磁盘抹除**：兑换次数归零后立即从 SQLite 物理删除（`DELETE`），后台守护协程每 60 秒定期彻底清空过期记录；
    3. **客户端内存自毁**：前端采用绝对时间戳与 `visibilitychange` 事件监听，手机锁屏休眠唤醒超时立即清空内存 `payload` 并卸载 DOM；
    4. **传输层 Zero-Store**：接口响应显式注入 `Cache-Control: no-store`，杜绝任何中间代理和浏览器磁盘缓存。
- **🌐 架构彻底解耦：Headless API 与零依赖独立单文件门户**：
  - 核心分发接口 `POST /api/portal/claim` 规范为标准 JSON API，全局启用 CORS 跨域支持；
  - 新增单文件纯 HTML 静态模板 `deploy/standalone-portal.html`（原生 Vanilla JS + 纯 CSS，单文件仅 ~15KB），支持独立部署于 Cloudflare Pages、GitHub Pages 或轻量虚拟主机中独立托管，源站 VPS 零暴露。
- **☁️ Cloudflare 边缘中立门户网关与反探测代理**：
  - 提供生产加固版 Cloudflare Worker（`deploy/cloudflare-worker-sub-proxy.js`）与 Cloudflare Pages（`deploy/cloudflare-pages/`）代理脚本；
  - 根路径伪装为中立静态页面，实现 Fixed-point 收敛多重 URL 解码防御路径穿越；
  - 精准识别腾讯、微信官方扫描爬虫与网络探测器并返回 200 OK 伪装页，保护分发域名免遭拦截报红。
- **🚀 生产环境平滑升级与安装脚本加固 (`deploy/install.sh`)**：
  - 升级为 Linux `install -m 755` 原子覆盖覆写机制，彻底解决 Linux 内核对运行中进程报错 `Text file busy` (ETXTBSY) 导致安装中断的顽疾；
  - 脚本支持智能多路径自动寻径（无论在根目录、deploy 子目录或 /tmp/ 下执行均可自动定位二进制）；
  - 敏感数据目录 `data/` 自动应用 `chmod 700` 权限安全收敛。
- **⚡ 单入站多通道分流线路展开优化 (SubRoutes Fan-out)**：
  - 修复并优化了多出口分流的订阅分发，支持在获取单节点或提取码时将入站绑定的所有可用多通道线路全量展开。
- **🧹 全工作区敏感信息脱敏审查与构建优化**：
  - 公开未鉴权页面全面移除所有代理协议敏感关键字，统一中立化伪装为“分布式网管接入点”；
  - 代码库所有示例与单元测试 100% 剥离个人真实 IP 与域名，彻底杜绝隐私泄露风险。

### 历史版本 (v1.6.0)

- **Xray-core 原生 gRPC 运行时热重载架构落地**：
  - 新增 `internal/xray` 高性能运行时协调器，全面接入官方 `proxyman/command.HandlerServiceClient`，实现用户增删（AddUser / RemoveUser）毫秒级动态下发；
  - 接入官方 `stats/command.StatsServiceClient`，实施无锁、低开销的实时用户双向流量采集；
  - 彻底解耦动态用户状态与物理静态 `config.json`，用户变动时长连接不断线、零重启。
- **嵌入式 BoltDB ACID 事务存储与故障补偿回滚**：
  - 新增 `internal/storage` 模块，提供基于 bbolt 的嵌入式强一致持久化；
  - 引入双向事务安全与自动逆向补偿机制：gRPC 调用失败中止提交，DB 写入异常自动调用 gRPC 逆向回滚，保障内存与磁盘强一致。
- **冷启动容灾回写管道 (`SyncToDiskConfig`)**：
  - 提供安全落盘合并引擎，在服务退出或手动触发时将 BoltDB 动态用户集合原子写回物理 `config.json`，确保冷启动与停机恢复零数据漂移。
- **多协议多态账户体系与废弃字段彻底淘汰**：
  - 新增 `internal/protocol` 领域格式化引擎，原生支持 VLESS (Vision/Reality)、VMess、Trojan、Shadowsocks 等协议的多态序列化与分享链接转换；
  - 全面清理过时 `alterId` 字段，严格对齐现代 Xray 规范，VMess 强制启用 AEAD 加密；
  - 新增 `internal/sub` 聚合订阅导出器，支持通用 Base64、Clash/Mihomo 与 Sing-box 订阅转换。
- **全链路生命周期纳管与优雅退出**：
  - 抽象 `app.Service` 标准接口，通过 `errgroup.WithContext` 统一拉起并编排 HTTP Server、Traffic Sync Job 与 Telegram Bot；
  - 优雅关机支持 Keep-Alive 连接排空，退出阶段按序关闭 gRPC 连接并释放数据库独占文件锁。
- **纯前端演示沙盒全面升级 (v3)**：
  - 升级 LocalStorage 演示沙盒至 v3 引擎，支持多协议节点生成、实时指标仪表盘与 gRPC 运行时交互日志仿真。

### 历史版本 (v1.5.1)

- **安全性加固**：
  - 修复 TOTP 2FA 关闭时的参数校验逻辑，强制要求输入 6 位动态验证码；
  - 移除默认静态硬编码 JWT 密钥，改为首次启动自动生成 256 位高熵密钥持久化，并在设置查询接口中脱敏；
  - 公开订阅接口 `/sub` 增加独立限流中间件，防止高频遍历扫描消耗 SQLite 连接；
  - 全局增加 10MB 请求体大小上限，防止超大恶意报文引发内存 OOM 异常。
- **协议与核心编译修复**：
  - 修复 Shadowsocks 客户端编译时缺失 Method 加密算法导致 Xray 内核崩溃的问题；
  - 完善配置文件读写容灾：配置读取异常时中止保存以保护物理文件，写入操作改为同目录临时文件原子重命名并配合读写锁；
  - 补齐 Trojan 与 VMess 订阅链接中的 SNI、ALPN、WebSocket 路径与 Host 等关键握手参数；
  - 自适应支持 REALITY 配置中的单数（serverName/shortId）与复数（serverNames/shortIds）字段。
- **并发与生命周期优化**：
  - 定时任务协程解耦：流量计费主循环与 Telegram 外部网络告警分离至独立协程，避免外部网络抖动阻塞计费；
  - 改进月度流量重置逻辑，自适应月末天数（如平年 2 月 28 天）并增加停机恢复补偿能力；
  - 每日历史流量表增加 `(user_email, date)` 复合唯一索引，杜绝并发写入导致的重复记录；
  - 修复用户删除后内存中运行时速率追踪器对象的常驻泄漏问题。

---

## 🏗️ 架构分层 (Clean Architecture)

> 💡 详细的业务领域模型、单端口多出口分流、4 层 Scoped 路由编排机制与双轨运行时架构全景图解请参阅：
> 👉 **[架构与系统逻辑全景图解 (docs/ARCHITECTURE_AND_LOGIC.md)](docs/ARCHITECTURE_AND_LOGIC.md)**

```
internal/
├── app/               # 应用生命周期契约与服务抽象 (Service, ServiceFunc)
├── domain/            # 业务领域实体与接口契约 (Inbound, Outbound, User, Route)
├── protocol/          # 多协议多态格式化与订阅生成 (VLESS, VMess, Trojan, Shadowsocks)
├── service/           # 核心用例与编译管道 (ConfigService, UserService, AlertService, SubService)
├── storage/           # 嵌入式 ACID 键值存储 (bbolt 持久化与编解码器)
├── sub/               # 聚合订阅导出器 (Base64, Clash/Mihomo, Sing-box)
├── xray/              # Xray 原生 gRPC Client、运行时协调器与冷启动落盘引擎
├── adapter/           # 外部系统适配实现
│   ├── xray/          # 强类型 Compiler、Config Parser 与 Supervisor
│   ├── repository/    # SQLite & GORM 仓储实现（WAL 模式加固）
│   ├── telegram/      # Telegram Bot 适配器与告警通知
│   └── monitor/       # gopsutil 硬件性能指标采集
└── delivery/          # 传输接入与服务暴露层
    ├── http/          # RESTful API、Gin 路由、优雅停机 Server 与防护中间件
    └── cron/          # 流量同步与状态轮询调度
```

---

## 🚀 快速开始

### 方式一：Linux Systemd 一键安装（推荐生产环境）

确保系统已安装 Xray-core 并正常运行：

```bash
# 1. 下载源码并进入目录
git clone https://github.com/coding-leaf/xray-panel.git
cd xray-panel

# 2. 执行一键部署安装脚本
sudo bash deploy/install.sh
```

一键脚本将自动完成：
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
| `-xray-config` | `XRAY_CONFIG_PATH` | `/usr/local/etc/xray/config.json` | Xray 核心主配置文件路径 |
| `-xray-grpc` | `XRAY_GRPC_ADDR` | `127.0.0.1:8080` | Xray 核心 API gRPC 监听地址 |
| `-xray-bin` | `XRAY_BIN_PATH` | `xray` | Xray 核心二进制程序路径 |
| `-service` | `SERVICE_NAME` | `xray` | Xray 的 systemd 服务名 |
| `-db` | `DB_PATH` | `data/panel.db` | 面板 SQLite 数据库文件路径 |
| `-jwt-secret` | `PANEL_JWT_SECRET` | 自动生成入库 | 管理员鉴权 JWT 签名密钥 |

---

## 🛠️ 本地开发与编译

### 前置要求
- Go 1.22+
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

## 🔒 安全与加固建议

1. **配置反向代理**：建议使用 Nginx / Caddy 申请 SSL 证书反向代理面板 Web 端口；
2. **首次登录修改密码**：初始账号为 `admin` / `admin123`，登录后请立即进入【系统设置】修改密码并启用 **TOTP 双因素认证 (2FA)**；
3. **隔离 gRPC API 通信端口**：Xray 的 gRPC API 监听地址建议限定为 `127.0.0.1:8080`，避免对公网开放；
4. **防火墙规则**：仅对外开放业务代理端口与 Web 管理端口。

---

## 📄 开源许可证

本项目基于 [MIT License](LICENSE) 协议开源。
