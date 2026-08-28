# 🚀 Xray Decoupled Panel (解耦运维监控与订阅面板)

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Vue-3.4+-4FC08D?style=flat-square&logo=vue.js" alt="Vue Version">
  <img src="https://img.shields.io/badge/Architecture-Clean%20Architecture-blue?style=flat-square" alt="Clean Architecture">
  <img src="https://img.shields.io/badge/Developed%20with-AI%20Pair%20Programming-8A2BE2?style=flat-square&logo=google-gemini" alt="Developed with AI">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/Status-Production%20Ready-success?style=flat-square" alt="Production Ready">
</p>

> 💡 **AI 驱动开发**：本项目由 AI 辅助结对编程深度打造，严格遵循 **Clean Architecture（整洁架构）** 分层契约，并通过 **ISO/IEC 25010 国际软件质量标准** 审查与自动化单元测试加固。

---

**Xray Decoupled Panel** 是一款高性能单机 Xray 运维监控与订阅分发面板。它采用**完全解耦模式**，仅通过 Xray 官方 gRPC API 与系统信号协同工作，不侵入 Xray 原生内核进程，具备极高的稳定性和极低资源占用。

---

## ✨ 核心特性

- 🧩 **极致解耦设计**：
  - 核心进程隔离：通过 Xray 原生 gRPC API（`HandlerService`、`StatsService`）进行用户动态增删与流量采集，拒绝魔改 Xray 核心。
  - 独立运行：面板进程崩溃或重启绝不影响现存网络代理连接。
- 📦 **单二进制极简部署**：
  - 前端基于 Vue 3 + Tailwind CSS 构建，所有静态资源通过 Go 1.16+ `//go:embed` 编译进单一可执行文件，零外部静态依赖。
- 📊 **全维度实时运维监控**：
  - 主机负载：实时采集 CPU 使用率、物理内存占比、磁盘使用率及网卡上下行瞬时速率。
  - 流量结算：多协程并发采集并持久化用户与入站节点累计流量，支持按月自动结算清零。
- 🔗 **安全订阅分发系统**：
  - 采用高熵独立 `subToken` 鉴权，支持 VLESS、VMess、Trojan、Shadowsocks 等主流协议节点自动生成与聚合。
  - 支持节点链接分发、Clash/Base64 订阅源及二维码实时渲染。
- ⚙️ **双模配置管理**：
  - **可视化入站管理**：便捷维护端口、协议、TLS/Reality 证书及流控参数。
  - **在线配置编辑器**：支持查看与在线编辑完整 `config.json`，保存前自动调用 `xray -test` 严格语法校验，杜绝配置错误导致核心停机。
- 🌍 **GeoData 规则库热更新**：
  - 一键在线拉取最新 `geoip.dat` 与 `geosite.dat`，具备实时下载进度百分比与平滑重载。
- 🤖 **Telegram 运维机器人**：
  - 支持 `/status`、`/traffic`、`/sub`、`/restart` 等交互式管理指令。
  - 内置防抖与冷却机制的主动告警（流量超额、系统过载、SSL 证书临期提醒）。
- 🛡️ **生产级安全防护**：
  - 内置防爆破滑动窗口限流器（`ulule/limiter`）、TOTP 双因素认证（2FA）、BCrypt 密码哈希、JWT 鉴权及 SQLite WAL 高并发支持。

---

## 🏗️ 架构分层 (Clean Architecture)

```
internal/
├── domain/            # 纯业务领域实体与接口契约（无外部依赖）
│   ├── user.go        # 用户实体与流量状态
│   ├── inbound.go     # 节点入站实体
│   └── repository.go  # 仓储与适配器抽象接口
├── service/           # 核心用例与业务逻辑
│   ├── user_service.go
│   ├── config_service.go
│   ├── sub_service.go
│   └── alert_service.go
├── adapter/           # 外部系统适配实现
│   ├── xray/          # Xray gRPC 客户端与配置文件解析器
│   ├── repository/    # SQLite & GORM 仓储实现（WAL并发加固）
│   ├── telegram/      # Telegram Bot 适配器
│   └── monitor/       # gopsutil 硬件指标采集
└── delivery/          # 传输接入层
    ├── http/          # RESTful API、Gin 路由与限流/鉴权中间件
    └── cron/          # 流量同步与状态轮询定时任务
```

---

## 🔒 生产环境安全与加固建议 (Security Best Practices)

为确保生产环境安全无虞，推荐按以下准则配置与加固：

1. **启用 HTTPS 反向代理（强烈推荐）**：
   - 避免直接将面板 HTTP 裸端口暴露在公网上。
   - 建议使用 Nginx / Caddy 申请合法 SSL 证书并通过反向代理访问（参考项目中的 `deploy/nginx-sample.conf`）。
2. **首次登录强制修改默认凭据**：
   - 初始密码为 `admin123`，首次登录后请立即进入【系统设置】修改为高强度密码。
   - 强烈建议开启 **TOTP 双因素身份验证 (2FA)**，使用 Google Authenticator 等应用扫码绑定。
3. **隔离 gRPC API 通信端口**：
   - Xray 配置中的 gRPC API 监听地址务必限定为 `127.0.0.1:8080`（仅允许本地回环访问），禁止绑定 `0.0.0.0`。
4. **自定义生产 JWT Secret**：
   - 生产环境中建议通过环境变量 `PANEL_JWT_SECRET` 指定至少 32 位的随机高熵密钥，杜绝使用默认密钥。
5. **防火墙策略最小权限原则**：
   - 使用 UFW / iptables 仅对外开放业务代理入站端口（如 `443`）与 Web 反代端口，面板底层端口与 gRPC 端口无需对外开放。

---

## 🚀 快速开始

### 方式一：Linux Systemd 一键安装（推荐生产环境）

确保系统已安装 Xray-core 并正常运行：

```bash
# 下载并进入解压目录
cd /root

# 执行一键部署安装脚本
sudo bash deploy/install.sh
```

一键脚本将自动完成：
1. 创建 `/usr/local/xray-panel` 工作与数据目录。
2. 注册并启动 `/etc/systemd/system/panel.service` 系统服务。
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
| `-xray-service` | `XRAY_SERVICE_NAME` | `xray` | Xray 的 systemd 服务名 |
| `-db` | `PANEL_DB_PATH` | `data/panel.db` | 面板 SQLite 数据库文件路径 |
| `-jwt-secret` | `PANEL_JWT_SECRET` | 随机生成/默认 | 管理员鉴权 JWT 签名密钥 |

---

## 🛠️ 本地开发与源码编译

### 前置要求
- Go 1.22+
- Node.js 18+ & npm / pnpm

```bash
# 1. 克隆代码仓库
git clone https://github.com/your-username/xray-panel.git
cd xray-panel

# 2. 编译前端静态资源
cd web
npm install
npm run build
cd ..

# 3. 运行全量单元测试与质量分析
go vet ./...
go test -v ./...

# 4. 编译单二进制可执行文件
go build -ldflags="-s -w" -o panel main.go embedded.go
```

---

## 📄 开源许可证

本项目基于 [MIT License](LICENSE) 协议开源。
