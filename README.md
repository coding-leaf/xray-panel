# 🚀 Xray Decoupled Panel (解耦运维监控与分流管理面板)

<p align="center">
  <img src="https://img.shields.io/badge/Version-v1.4.2-indigo?style=flat-square" alt="Version">
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Vue-3.4+-4FC08D?style=flat-square&logo=vue.js" alt="Vue Version">
  <img src="https://img.shields.io/badge/Architecture-Clean%20Architecture-blue?style=flat-square" alt="Clean Architecture">
  <img src="https://img.shields.io/badge/Live%20Demo-GitHub%20Pages-success?style=flat-square&logo=github" alt="Live Demo">
  <img src="https://img.shields.io/badge/Developed%20with-AI%20Pair%20Programming-8A2BE2?style=flat-square&logo=google-gemini" alt="Developed with AI">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License">
</p>

<p align="center">
  <b>🌟 在线免部署体验 Demo：</b>
  <a href="https://coding-leaf.github.io/xray-panel/" target="_blank">
    <b>https://coding-leaf.github.io/xray-panel/</b>
  </a>
</p>

---

> 💡 **AI 结对工程范式**：本项目采用现代 **Clean Architecture（整洁架构）** 分层契约驱动开发，具备严密的业务实体封装、单向配置编译管道与完备的自动化单元测试加固。

**Xray Decoupled Panel** 是一款高性能单机 Xray 运维监控、节点分流与订阅分发面板。它采用**完全解耦设计**，通过 Xray 原生 gRPC API 与系统信号协同工作，不侵入 Xray 原生内核进程，具备极高的稳定性和极低资源占用。

---

## ⚡ 核心亮点 (Core Highlights)

- 🌐 **单入站多通道解耦分流 (Decoupled Relaying)**：
  - 首创 **VLESS Route ID 动态 UUID 映射** 机制，单端口（如 443 Reality）可挂载数十条不同国家/WARP 的独立落地出口。
  - **1 步极速发布**：支持直接从落地出口一键绑定发布至主入口网关，秒级生成独立订阅节点。
- 🎭 **纯前端 Mock 演示沙盒 (GitHub Pages)**：
  - 内置基于 LocalStorage 的纯前端数据仿真引擎，无需后端服务器即可完整体验节点增删、通道编排与扫码订阅。
- 🧩 **极致解耦与单二进制交付**：
  - 前端基于 Vue 3 + Tailwind CSS 构建，所有静态资源通过 Go `//go:embed` 编译进单一二进制文件，无外部静态依赖。
  - 核心进程隔离：面板崩溃或热重载绝不影响现存网络连接与代理进程。
- 📊 **5秒级精准监控与在线追踪**：
  - 实时采集主机 CPU、内存、磁盘与双向网卡吞吐速率。
  - 基于 Xray gRPC `StatsService` 的 5 秒级轮询机制，智能追踪用户瞬时速率与 `🟢 在线传输` / `🟢 正常(空闲)` / `🔴 已禁用` 状态。
- 🔗 **聚合订阅与二维码分发**：
  - 采用高熵独立 `subToken` 鉴权，支持 VLESS、VMess、Trojan、Shadowsocks 等协议节点的统一聚合与单节点/全节点订阅导出。
- 🌍 **GeoData 规则库热更新**：
  - 一键在线拉取最新 `geoip.dat` 与 `geosite.dat`，具备实时下载进度百分比与平滑重载。
- 🤖 **Telegram 运维机器人**：
  - 支持 `/status`、`/traffic`、`/sub`、`/restart` 等交互式管理指令与主动告警推送（流量超额、系统过载、SSL 临期）。
- 🛡️ **生产级安全防护**：
  - 内置滑动窗口限流器（`ulule/limiter`）、TOTP 双因素认证（2FA）、BCrypt 哈希加密、JWT 鉴权及 SQLite WAL 模式加固。

---

## 🏗️ 架构分层 (Clean Architecture)

```
internal/
├── domain/            # 纯业务领域实体与接口契约（Inbound, Outbound, User, Route）
├── service/           # 核心用例（ConfigService 单向编译管道, UserService, AlertService）
├── adapter/           # 外部系统适配实现
│   ├── xray/          # Xray 强类型 Compiler、gRPC 客户端与 Systemd Supervisor
│   ├── repository/    # SQLite & GORM 仓储实现（WAL 并发加固）
│   ├── telegram/      # Telegram Bot 适配器
│   └── monitor/       # gopsutil 硬件指标采集
└── delivery/          # 传输接入层
    ├── http/          # RESTful API、Gin 路由与限流/鉴权中间件
    └── cron/          # 流量同步与状态轮询定时任务 (5s 极速同步)
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
3. 开启开机自启并输出面板访问地址。

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
| `-xray-service` | `XRAY_SERVICE_NAME`| `xray` | Xray 的 systemd 服务名 |
| `-db` | `PANEL_DB_PATH` | `data/panel.db` | 面板 SQLite 数据库文件路径 |
| `-jwt-secret` | `PANEL_JWT_SECRET` | 随机生成/默认 | 管理员鉴权 JWT 签名密钥 |

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

## 🔒 生产环境安全与加固建议

1. **启用 HTTPS 反向代理**：建议使用 Nginx / Caddy 申请 SSL 证书反向代理面板端口；
2. **首次登录强制修改凭据**：初始账号为 `admin` / `admin123`，登录后请立即进入【系统设置】修改密码并启用 **TOTP 双因素认证 (2FA)**；
3. **隔离 gRPC API 通信端口**：Xray 的 gRPC API 监听地址必须限定为 `127.0.0.1:8080`，严禁对外网开放；
4. **最小防火墙权限**：仅对外开放业务代理端口（如 443）与 Web 反代端口。

---

## 📄 开源许可证

本项目基于 [MIT License](LICENSE) 协议开源。
