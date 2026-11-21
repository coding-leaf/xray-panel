# Xray Decoupled Panel

<p align="center">
  <img src="https://img.shields.io/badge/Version-v2.7.5-indigo?style=flat-square" alt="Version">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Vue-3.4+-4FC08D?style=flat-square&logo=vue.js" alt="Vue Version">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License">
</p>

<p align="center">
  <b>在线免部署体验 Demo：</b>
  <a href="https://coding-leaf.github.io/xray-panel/" target="_blank">
    <b>https://coding-leaf.github.io/xray-panel/</b>
  </a>
</p>

---

**Xray Decoupled Panel** 是基于 Go 与 Vue 3 构建的轻量级 Xray 运维监控、节点分流与订阅管理面板。系统采用控制面与数据面解耦架构，通过 Xray 原生 gRPC API（HandlerService / StatsService）直接与运行中内核通信，支持用户毫秒级热增删、多入站分流编排、实时流量采集与多客户端聚合订阅分发。

---

## 核心特性

- **控制面与数据面解耦**：通过自研轻量 gRPC 契约包与 Xray API 直接交互，用户启停与凭证变更实时生效，长连接不中断；仅在入站、出站、全局路由分流规则变更时执行平滑内核重载。
- **入站分流与用户隔离**：支持单入站端口多出口分流线路（VLESS 16-bit routeId 机制），可按分流线路配置用户授权白名单，实现订阅节点动态下发与内核级路由匹配隔离。
- **高可定制用户与客户端订阅编排**：支持用户全生命周期属性编辑（Email/UUID/SubToken 热同步、用户级 Flow 覆盖、多模式到期时间与备注检索）；支持入站与子线路客户端展示排序权重 (`SortOrder`)、自定义节点命名模板、顶部流量/到期虚拟状态节点与置顶公告节点。
- **多协议适配与聚合订阅**：支持 VLESS (Vision/Reality)、VMess、Trojan、Shadowsocks 等协议，提供 Base64、Clash / Mihomo、Sing-box 订阅格式聚合导出与二维码扫码。
- **全方位系统可观测性**：采集主机 CPU、内存、磁盘与真实网卡吞吐（自动过滤回环及虚拟网卡）；支持用户瞬时速率统计与月度自适应流量重置；内置零堆分配 Xray 访问与错误日志快速检索。
- **可靠事务与单二进制交付**：SQLite 嵌入式存储（WAL 模式）作为业务真相源，具备 Saga 逆向补偿机制保障数据库、Xray 内存与物理配置文件三态一致；前端产物通过 Go embed 打包进单二进制，无外部运行依赖。
- **安全防护与访问控制**：支持 RFC 6238 TOTP 双因素认证 (2FA)、带外凭据提取码分发 (Ticket)、JWT 认证与敏感配置防泄漏脱敏。

> 历史迭代与详细变更请参阅：[CHANGELOG.md (版本更新日志)](CHANGELOG.md)

---

## 架构概览

本项目严格遵循清洁架构（Clean Architecture）原则，分为领域层（`domain`）、用例编排层（`service`）、外部适配层（`adapter`）与交付接入层（`delivery`）。

详细系统拓扑、单端口多出口分流与双轨运行时架构请参阅：
👉 [架构与系统逻辑全景图解 (docs/ARCHITECTURE_AND_LOGIC.md)](docs/ARCHITECTURE_AND_LOGIC.md)

---

## 快速开始

### 方式一：Master 单机一键安装（推荐生产环境）

确保系统已安装 Xray-core 并处于运行状态；执行 `deploy/install.sh` 前需先准备好 `panel` 二进制文件（按下方「本地开发与编译」构建产物或从 Release 下载，并置于当前目录 `./panel` 或 `/tmp/panel`）：

```bash
# 1. 下载源码并进入目录
git clone https://github.com/coding-leaf/xray-panel.git
cd xray-panel

# 2. 准备 panel 二进制（示例：本地构建，或将已下载的 panel 放到当前目录 / /tmp/panel）
# cd web && npm install && npm run build && cd ..
# go build -ldflags="-s -w" -o panel .

# 3. 执行一键安装脚本
sudo bash deploy/install.sh
```

脚本将自动配置 `/usr/local/xray-panel` 工作目录并注册为 `systemd` 系统守护进程（开机自启）。

---

### 方式二：手动运行与常用参数

```bash
# 直接运行面板
./panel -port 9000 -xray-config /usr/local/etc/xray/config.json -xray-grpc 127.0.0.1:8080
```

#### 启动参数与环境变量：

| 命令行参数 | 环境变量 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `-port` | `PANEL_PORT` | `9000` | 面板 Web 监听端口 |
| `-xray-grpc` | `XRAY_GRPC_ADDR` | `127.0.0.1:8080` | Xray 核心 gRPC API 监听地址 |
| `-xray-config` | `XRAY_CONFIG_PATH` | 自动探测（Linux 优先 `/usr/local/etc/xray/config.json`） | Xray 配置文件路径 |
| `-xray-bin` | `XRAY_BIN_PATH` | 自动探测（Linux 优先 `/usr/local/bin/xray`） | Xray 核心二进制路径 |
| `-db` | `DB_PATH` | `data/panel.db` | SQLite 数据库存储路径 |
| `-jwt-secret` | `PANEL_JWT_SECRET` | 首次运行自动生成高熵密钥 | 管理员登录 JWT 签名密钥 |
| `-log-level` | `LOG_LEVEL` | `info` | 日志输出级别（`debug`/`info`/`warn`/`error`） |
| `-service` | `SERVICE_NAME` | `xray` | Xray 守护进程 systemd 服务名 |
| `-public-url` | `PUBLIC_URL` | `http://127.0.0.1:9000` | 用于生成订阅链接的面板公网访问地址 |
| `-panel-version` | `PANEL_VERSION` | 编译期注入版本 (`v2.7.5`) | 动态覆盖面板展示版本号（同时支持 `-v` / `-version` 打印版本） |

---

## 多节点 Agent 部署（`xray-agent`）

如需纳管多台远端节点服务器，可在节点机器部署轻量级独立守护进程 `xray-agent`。

### 方式 A：通过 Master 面板一键安装命令部署（推荐）

1. 在 Master 面板进入 **「远端直连节点 → 新增节点」**（或对已有节点执行 **「轮换 Token / 查看安装脚本」**）；
2. 在弹窗中确认 **Master URL** 为远端节点公网可达的有效 HTTPS 地址（推荐使用具备可信 TLS 证书的反向代理入口，避免填写 `127.0.0.1` 或内网地址）；
3. 点击 **「复制一键安装命令」**，切到目标节点 SSH 终端直接粘贴回车即可（已自动内嵌 `MASTER_URL` 与一次性 `AGENT_TOKEN`，适配 GCP / AWS / Debian / Ubuntu 等非 root 与 root 宿主机；极简容器若无 `sudo` 可去掉命令中的 `sudo`）：

```bash
curl -fsSL https://raw.githubusercontent.com/coding-leaf/xray-panel/master/deploy/install-agent.sh | sudo MASTER_URL='https://panel.example.com' AGENT_TOKEN='<YOUR_NODE_TOKEN>' bash
```

如需在执行前审计脚本内容或本地保存执行，可在弹窗下方展开 **「查看完整 systemd 脚本（高级 / 审计）」** 复制完整脚本。

脚本会自动识别 `amd64` / `arm64` 架构并从 `AGENT_BIN_URL`（默认 GitHub Releases 最新版）拉取 `xray-agent`，将凭据以 `0600` 权限写入 `/etc/xray-agent/agent.env`，并注册启动 `xray-agent.service`。

### 方式 B：命令行远程拉取或本地运行 `deploy/install-agent.sh`

适用于命令行快速部署、自动化编排或二进制升级：

```bash
# 远程拉取一键安装（无需先克隆仓库，必填 MASTER_URL 与 AGENT_TOKEN）
curl -fsSL https://raw.githubusercontent.com/coding-leaf/xray-panel/master/deploy/install-agent.sh | sudo MASTER_URL='https://panel.example.com' AGENT_TOKEN='<YOUR_NODE_TOKEN>' bash

# 本地仓库直接安装（必填 MASTER_URL 与 AGENT_TOKEN）
sudo MASTER_URL='https://panel.example.com' AGENT_TOKEN='<YOUR_NODE_TOKEN>' bash deploy/install-agent.sh

# 可选：通过 AGENT_BIN_URL 覆盖默认 GitHub Releases 下载地址
sudo AGENT_BIN_URL='https://mirror.example.com/xray-agent-linux-amd64' \
  MASTER_URL='https://panel.example.com' \
  AGENT_TOKEN='<YOUR_NODE_TOKEN>' \
  bash deploy/install-agent.sh

# 升级重跑（当 /etc/xray-agent/agent.env 已存在且不传凭据时，自动保留原有配置并仅更新二进制与服务）
sudo bash deploy/install-agent.sh
```

### 云主机（如 GCP）防火墙与最小宿主机前提

- **零 Agent 控制入站端口**：`xray-agent` 仅主动出站轮询 Master（`POST /api/agent/sync`），**无需**在 GCP 或云防火墙开放 `8080`、`9000` 等 Agent 控制入站端口；仅需确保节点可出站访问 Master 入口，并按需放行你的业务代理入站端口（本机 gRPC `127.0.0.1:8080` 仅限回环通信）。
- **最小宿主机前提**：宿主机需已安装可执行 `xray` 与 `xray.service` 以承接 Master 下发配置与重载。

### `xray-agent` 启动参数与环境变量

参数优先级：CLI 参数 > 环境变量 > 默认值。

| 命令行参数 | 环境变量 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `-master-url` | `MASTER_URL` | `""`（必填） | Master 面板基础访问地址（如 `https://panel.example.com`） |
| `-token` | `AGENT_TOKEN` | `""`（必填） | 节点 Bearer Token（生产环境建议通过 `agent.env` 注入，避免命令行明文暴露） |
| `-xray-bin` | `XRAY_BIN` | `xray` | Xray 可执行文件路径 |
| `-xray-config` | `XRAY_CONFIG` | `/usr/local/etc/xray/config.json` | Xray 配置文件路径 |
| `-xray-grpc` | `XRAY_GRPC` | `127.0.0.1:8080` | 本机 Xray gRPC API 地址（需与远端编译配置 `127.0.0.1:8080` 一致） |
| `-xray-service` | `XRAY_SERVICE` | `xray` | Xray 守护进程 systemd 服务名 |
| `-xray-assets-dir` | `XRAY_ASSETS_DIR` | `/usr/local/share/xray` | `geoip.dat` 与 `geosite.dat` 存放目录 |
| `-spool-dir` | `SPOOL_DIR` | `/var/lib/xray-agent` | 待确认流量批次与同步状态缓存目录 |
| `-interval` | `SYNC_INTERVAL` | `60s` | 心跳同步周期（合法正时长，运行时受 Master 配置下发动态调整） |
| `-log-level` | `LOG_LEVEL` | `info` | 日志输出级别（`debug` / `info` / `warn` / `error`） |
| `-log-json` | *(无)* | `false` | 是否输出结构化 JSON 日志 |

### 常用运维与排障命令

```bash
# 查看 Agent 运行状态
sudo systemctl status xray-agent

# 实时查看 Agent 日志
sudo journalctl -u xray-agent -f

# 重启 Agent 服务
sudo systemctl restart xray-agent
```

---

## 本地开发与编译

### 环境要求
- Go 1.26+
- Node.js 20+ & npm

```bash
# 1. 纯前端 Mock 模式本地调试 (无需 Xray 内核)
cd web && npm install && npm run dev:demo

# 2. 前端类型检查与构建
npm run typecheck
npm run build
cd ..

# 3. 运行后端测试与编译二进制
go test -v ./...
go build -ldflags="-s -w" -o panel .
go build -ldflags="-s -w" -o xray-agent ./cmd/agent
```

---

## 安全建议

1. **配置反向代理**：建议生产环境使用 Nginx / Caddy 申请权威 TLS 证书反向代理面板端口；
2. **启用双因素认证**：初始管理员账户为 `admin` / `admin123`，登录后请立即进入系统设置修改密码并启用 TOTP (2FA)；
3. **隔离 gRPC API 端口**：Xray 的 gRPC API 监听端口务必限定为本机回环地址（`127.0.0.1:8080`），严禁映射至公网；
4. **防火墙收敛**：仅对外开放必要的业务代理端口与 Web 端口。

---

## 开源许可证

本项目基于 [MIT License](LICENSE) 协议开源。
