# 🚀 Xray Decoupled Panel (解耦运维监控与分流管理面板)

<p align="center">
  <img src="https://img.shields.io/badge/Version-v1.5.1-indigo?style=flat-square" alt="Version">
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

**Xray Decoupled Panel** 是一款基于 Go 与 Vue 3 构建的 Xray 单机运维监控、节点分流与聚合订阅管理面板。项目采用解耦设计，通过 Xray 原生 gRPC API 与系统服务进行交互管理，支持多入站/多出站分流编排、用户流量统计与聚合订阅分发。

---

## 🤖 AI 辅助开发与审计声明 (AI Disclosure)

本项目在架构设计、深层次代码审计、安全漏洞排查及单元测试编写过程中，使用了 AI 工具（Google DeepMind / Antigravity Agent）进行结对开发与分析辅助。

- **客观性提示**：项目文档与代码中的部分实现可能带有 AI 辅助生成的痕迹。虽然所有关键变更均已经过静态检查、语法校验与自动化单元测试，但在实际生产环境中使用前，仍建议使用者根据自身网络环境与安全策略进行充分测试与验证；
- **合规声明**：请使用者自觉遵守当地法律法规，合理合法使用网络代理技术。

---

## ⚡ 核心功能与特性 (Features)

- 🔄 **动态用户管理与热重载**：
  - 日常用户增删改查、配额调整、批量续期走 Xray 原生 gRPC API 动态同步，并配合静默持久化回写磁盘，无需频繁重启 Xray 进程；
  - 仅在入站、出站、全局路由分流规则或 DNS 发生变更时执行平滑内核重载。
- 🛡️ **并发流量统计防冲正**：
  - 采用字段隔离更新与重置时间窗口过滤，避免并发写入或旧周期增量冲正刚归零的流量。
- 🌐 **单入站多通道分流 (VLESS Route)**：
  - 支持基于 VLESS UUID 映射的多出口分流机制，单个入站端口可按用户线路映射到不同落地出站，生成独立订阅节点。
- 🎯 **协议与配置规范对齐**：
  - 支持 VLESS、VMess、Trojan、Shadowsocks 等协议；REALITY 服务端参数规范化清洗；Shadowsocks 客户端配置加密算法支持；Trojan / VMess 订阅链接补齐 SNI、ALPN、WebSocket 路径与 Host 头。
- 🎭 **纯前端 Mock 演示沙盒 (GitHub Pages)**：
  - 内置基于 LocalStorage 的纯前端数据仿真引擎，无需后端服务器即可完整体验节点增删、通道编排与扫码订阅。
- 🧩 **单二进制交付**：
  - 前端基于 Vue 3 + Tailwind CSS 构建，所有静态资源通过 Go `//go:embed` 编译进单一二进制文件，无外部静态资源依赖，便于部署。
- 📊 **实时监控与在线追踪**：
  - 采集主机 CPU、内存、磁盘与双向网卡吞吐速率；
  - 基于 Xray gRPC `StatsService` 的定时轮询机制，追踪用户瞬时速率与在线连接状态。
- 🔗 **聚合订阅与二维码分发**：
  - 采用独立 `subToken` 鉴权，支持多协议节点的统一聚合与单节点/全节点订阅导出。
- 🌍 **GeoData 规则库热更新**：
  - 支持在线拉取 `geoip.dat` 与 `geosite.dat` 规则库并平滑重载。
- 🤖 **Telegram 运维机器人**：
  - 支持 `/status`、`/traffic`、`/sub`、`/restart` 等交互式管理指令与告警推送（流量超额、系统过载、SSL 临期）。
- 🔒 **安全防护与访问控制**：
  - 支持 TOTP 双因素认证（2FA）、动态高熵 JWT 密钥持久化、公开订阅接口防刷限流及 10MB 请求体大小上限防护。

---

## 📝 最近更新日志 (v1.5.1)

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

```
internal/
├── domain/            # 业务领域实体与接口契约（Inbound, Outbound, User, Route）
├── service/           # 核心用例（ConfigService 单向编译管道, UserService, AlertService）
├── adapter/           # 外部系统适配实现
│   ├── xray/          # Xray 强类型 Compiler、gRPC 客户端与 Systemd Supervisor
│   ├── repository/    # SQLite & GORM 仓储实现（WAL 模式加固）
│   ├── telegram/      # Telegram Bot 适配器
│   └── monitor/       # gopsutil 硬件指标采集
└── delivery/          # 传输接入层
    ├── http/          # RESTful API、Gin 路由与限流/鉴权中间件
    └── cron/          # 流量同步与状态轮询定时任务
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
