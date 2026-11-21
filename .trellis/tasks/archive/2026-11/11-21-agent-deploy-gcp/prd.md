# Agent 节点部署指南与脚本完善 (PRD)

## Goal & User Value
解决新服务器（特别是 GCP 等默认非 root 云主机）仅看仓库文档无法完成部署、以及复制 Master 面板内置 Agent 脚本在非 root 环境下直接报错的问题。让用户既可以通过 Master 面板一键脚本直接把新 GCP 节点接入现有 `jp` Master，也可以通过仓库文档与独立脚本完成可复用的 Master / Agent 双模式部署。

## Confirmed Facts & Evidence
完整代码审计记录见 [research/agent-deploy-audit.md](file:///d:/code/Workspace/xray-panel/.trellis/tasks/11-21-agent-deploy-gcp/research/agent-deploy-audit.md)。
- **架构与二进制拆分**：Master 为 `panel`（`main.go:1-276`），Agent 为独立轻量二进制 `xray-agent`（`cmd/agent/main.go:1-153`）；GitHub Releases 已自动构建并发布 `xray-agent-linux-amd64` 与 `xray-agent-linux-arm64`（`.github/workflows/release.yml:101-135`）。
- **通信与端口模型**：Agent 仅需必填 `-master-url`/`MASTER_URL` 与 `-token`/`AGENT_TOKEN`（`cmd/agent/main.go:39-40`、`internal/agent/client.go:31-39`），通过主动出站 HTTP/HTTPS JSON 轮询 `POST /api/agent/sync` 与 `/api/agent/commands/:id/result`（`internal/delivery/http/router.go:86-93`），无需在 GCP 开放任何 Agent 控制入站端口；使用 `https://` 时要求 Master 具备可信 TLS 证书（`internal/agent/client.go:40-42`）。
- **Master 已有内置脚本生成能力**：`NodeService.RenderInstallScript`（`internal/service/node_service.go:689-783`）配合前端 `NodeTokenModal.vue`（`web/src/views/nodes/components/NodeTokenModal.vue:192-224`）可为新建/轮换节点自动生成内嵌一次性 Token 的 systemd 安装脚本，并已支持通过 `AGENT_BIN_URL` 默认从 GitHub Releases 拉取对应架构二进制。
- **已确认的 4 项现存缺口**：
  1. `README.md:46-62` 的 Master「方式一」未说明 `deploy/install.sh` 不会自动编译/下载 `panel` 二进制，且完全缺失 Agent 模式部署说明、参数表与编译入口；
  2. `deploy/` 目录缺失独立 Agent 安装脚本 `deploy/install-agent.sh`；
  3. `RenderInstallScript`（`internal/service/node_service.go:699-780`）缺少 root 权限校验（GCP 默认非 root 用户直接执行会报错）；
  4. `NodeTokenModal.vue` 未提示非 root 宿主机需使用 `sudo bash`，且 `MASTER_URL` 提示未强调避免误填 `127.0.0.1` 或内网地址。

## Requirements

### R1: 更新 `README.md` 补全 Master 前置与 Agent 部署指南
- 修正「方式一：Master 单机部署」：明确说明执行 `deploy/install.sh` 前需先准备好 `panel` 二进制（按「本地开发与编译」编译生成或放置于当前目录 / `/tmp/panel`）。
- 新增「多节点 Agent 部署（`xray-agent`）」章节：
  - **推荐方式（Master 面板一键脚本）**：在 Master「远端直连节点 → 新增节点」获取预填 Token 的安装脚本，确认公网可达的 `MASTER_URL`（推荐有效 HTTPS），在 GCP 等非 root 服务器通过 `sudo bash` 执行；
  - **独立脚本方式**：使用 `deploy/install-agent.sh`（传入 `MASTER_URL`、`AGENT_TOKEN`，自动从 GitHub Releases 按架构下载 `xray-agent`，可选 `AGENT_BIN_URL` 覆盖下载地址）；
  - **GCP/云主机防火墙与最小前提说明**：明确 Agent 为纯出站轮询，**无需开放 8080/9000 等 Agent 控制入站端口**，仅需出站可达 Master 与业务代理端口；一句话说明宿主机需存在可执行 `xray` 与 `xray.service` 以承接配置下发（不写 Xray 业务配置教程）；
  - **`xray-agent` 启动参数与环境变量表**（对齐 `cmd/agent/main.go:38-51`）；
  - **常用运维排障命令**：`systemctl status xray-agent`、`journalctl -u xray-agent -f`。
- 在「本地开发与编译」中补充 `xray-agent` 编译命令（`go build -ldflags="-s -w" -o xray-agent ./cmd/agent`）。

### R2: 新增独立脚本 `deploy/install-agent.sh`
- 对齐 `deploy/install.sh` 风格，支持在任意 Linux Systemd 宿主机直接安装或升级 `xray-agent`：
  - 强制 root 检查（`EUID -ne 0` 直接给出 `sudo bash` 提示退出）；
  - 架构自动识别（`x86_64|amd64` → `amd64`，`aarch64|arm64` → `arm64`）；
  - 必填环境变量校验：`MASTER_URL` 与 `AGENT_TOKEN`（首次安装必填并写入 `0600` 的 `/etc/xray-agent/agent.env`；升级重跑时若 `/etc/xray-agent/agent.env` 已存在且未显式传入 `MASTER_URL`/`AGENT_TOKEN`，保留现有凭据避免误覆盖；若只传其一则报错）；
  - 统一从 `AGENT_BIN_URL`（默认 `https://github.com/coding-leaf/xray-panel/releases/latest/download/xray-agent-linux-${ARCH}`）经 `curl`/`wget` 下载到隔离临时文件，校验非空后原子替换 `/usr/local/bin/xray-agent`（不做隐式本地文件扫描兜底）；
  - 创建 `/etc/xray-agent` 与 `/var/lib/xray-agent`，配置并启用 `/etc/systemd/system/xray-agent.service`（`ExecStart=/usr/local/bin/xray-agent --spool-dir /var/lib/xray-agent --interval 60s`，通过 `EnvironmentFile=/etc/xray-agent/agent.env` 注入凭据，命令行不出现 `--token`）；
  - 安装完成后输出服务状态与日志排查命令。

### R3: 加固 Master `NodeService.RenderInstallScript` 与单元测试
- 修改 `internal/service/node_service.go:689-783` 中的 `RenderInstallScript`：
  - 在 `set -euo pipefail` 之后增加 root 权限前置校验（`[ "$(id -u)" -ne 0 ]` 时输出需用 `sudo bash` 执行的清晰错误并退出）；
  - 保持现有纯净的 GitHub Release (`AGENT_BIN_URL`) 下载逻辑不变，不引入本地二进制扫描兜底；
  - 严格保留全部既有安全与契约要求：不内嵌明文 Token、`NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"` 占位符字节一致、`install -m 0755 "$TMP_BIN" /usr/local/bin/xray-agent`、`EnvironmentFile=/etc/xray-agent/agent.env`、`0600`、`--interval 60s`、`127.0.0.1:8080`。
- 更新 `internal/service/node_service_test.go:1222-1277`，保留原有全部断言并新增 root 校验断言。

### R4: 前端 `NodeTokenModal.vue` 执行提示微调
- 修改 `web/src/views/nodes/components/NodeTokenModal.vue`：
  - 完善 `MASTER_URL` 提示文案（强调需为远端节点公网可达的有效 HTTPS/HTTP 地址，避免 `127.0.0.1` 或内网地址）；
  - 在脚本展示区补充一行简明执行提示：非 root 云主机（如 GCP）请保存脚本后使用 `sudo bash` 执行。
  - 严格保持 `renderedScript` 中对 `NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"` 的替换逻辑不变。

## Acceptance Criteria
- [ ] **AC-1（文档完整可用）**：`README.md` 明确修正 Master 二进制前置要求，并包含完整的 Agent 部署方式（Master 一键脚本、独立 `deploy/install-agent.sh`、GCP 防火墙与 `sudo` 说明、`xray-agent` 参数表、排障命令及开发编译命令），且不包含 Xray 业务配置教程。
- [ ] **AC-2（独立脚本语法与契约正确）**：`deploy/install-agent.sh` 为 LF 换行、通过 `bash -n deploy/install-agent.sh` 语法检查，包含 root 校验、纯净 `AGENT_BIN_URL` Release 下载、幂等升级凭据保护、`0600` `/etc/xray-agent/agent.env` 与 `xray-agent.service`（`ExecStart` 无明文 `--token`）。
- [ ] **AC-3（Master 模板加固与单测通过）**：`RenderInstallScript` 包含 root 校验并保持 `AGENT_BIN_URL` 下载与 `NODE_TOKEN` 占位符契约完全兼容，`mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .`、`mise x -- go build ./cmd/agent` 全绿。
- [ ] **AC-4（前端构建与类型检查通过）**：`NodeTokenModal.vue` 展示清晰 `sudo bash` 与公网 `MASTER_URL` 提示，`cd web && mise x -- npm run typecheck && mise x -- npm run build` 全绿。
- [ ] **AC-5（安全红线）**：所有文档、脚本、测试中无真实公网 IP、生产域名、Token 或私钥。

## Out of Scope
- Xray-core 业务入站、出站、路由规则或 TLS/Reality 配置教程。
- 本地二进制扫描/兜底逻辑（统一走 GitHub Releases / `AGENT_BIN_URL`）。
- 本机安装 `gcloud` CLI 或直接远程登录操作用户的 GCP 生产机器。
- 重写 `docs/ARCHITECTURE_AND_LOGIC.md` 或修改 Agent 同步协议/API 契约/数据库 Schema。
