# Implementation Plan: Agent 节点部署指南与脚本完善

## 1. Ordered Implementation Checklist

### Step 1: 后端 `RenderInstallScript` root 校验与 Fail-Repro 单测
- 目标文件：
  - `internal/service/node_service_test.go`（`TestNodeService_RenderInstallScript_EnvFileAndInterval`）
  - `internal/service/node_service.go`（`RenderInstallScript`，约 L689-783）
- 实施细节：
  1. 先在 `internal/service/node_service_test.go` 补充断言，验证 `RenderInstallScript` 输出包含 root 校验（`id -u` 与 `sudo bash` 提示），运行单测确认红灯；
  2. 更新 `internal/service/node_service.go` 中的 `RenderInstallScript` 模板：
     - 仅在 `set -euo pipefail` 之后添加 `if [ "$(id -u)" -ne 0 ]; then echo "Error: please run this script as root (e.g. sudo bash ...)." >&2; exit 1; fi`；
     - 保持现有 `AGENT_BIN_URL` 下载、`NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"`、`install -m 0755 "$TMP_BIN" /usr/local/bin/xray-agent`、`EnvironmentFile=/etc/xray-agent/agent.env`、`0600`、`--interval 60s`、`127.0.0.1:8080` 等全部既有逻辑不变；
  3. 重新运行 `mise x -- go test ./internal/service -run TestNodeService_RenderInstallScript -v` 确认绿灯。

### Step 2: 新增独立部署脚本 `deploy/install-agent.sh`
- 目标文件：`deploy/install-agent.sh`（新建，UTF-8 LF 换行）
- 实施细节：
  1. 对齐 `deploy/install.sh` 交互提示风格，使用 `#!/usr/bin/env bash` 与 `set -euo pipefail`；
  2. 实现 root 权限检查（`[[ $EUID -ne 0 ]]`）；
  3. 识别 CPU 架构（`amd64` / `arm64`）；
  4. 校验并处理 `MASTER_URL` / `AGENT_TOKEN`：首次安装必填；升级重跑时若 `/etc/xray-agent/agent.env` 已存在且两者均未传入则保留现有凭据，若只传其一则报错；
  5. 从 `AGENT_BIN_URL`（默认 GitHub Release 最新版 `xray-agent-linux-${ARCH}`）经 `curl`/`wget` 下载至 `mktemp` 临时文件（带 `trap` 清理），校验非空并原子安装至 `/usr/local/bin/xray-agent`；
  6. 安全写入 `0600` `/etc/xray-agent/agent.env` 与 `0644` `/etc/systemd/system/xray-agent.service`，重载并启动服务，检查 `systemctl is-active` 状态并输出排查命令；
  7. 使用 `bash -n deploy/install-agent.sh` 验证脚本语法，并检查文件为纯 LF 换行符。

### Step 3: 前端 `NodeTokenModal.vue` 执行提示优化
- 目标文件：`web/src/views/nodes/components/NodeTokenModal.vue`
- 实施细节：
  1. 微调 `MASTER_URL` `FormField` 的 `hint` 文案，提醒优先使用远端节点公网可达且证书有效的 HTTPS 地址（避免 `127.0.0.1` 或内网 IP）；
  2. 在 systemd 部署脚本 `<pre>` 上方增加一行简洁提示，说明 GCP 等非 root 宿主机请保存脚本后使用 `sudo bash` 运行；
  3. 不改动 `renderedScript` 的 `NODE_TOKEN` 替换逻辑与任何 API 调用。

### Step 4: 更新 `README.md`
- 目标文件：`README.md`
- 实施细节：
  1. 在「快速开始 → 方式一：Master 单机一键安装」中明确补充：执行 `sudo bash deploy/install.sh` 前需先准备好 `./panel` 二进制（按「本地开发与编译」构建或放置于当前目录 / `/tmp/panel`）；
  2. 新增「多节点 Agent 部署（`xray-agent`）」章节，包含：
     - 方式 A（推荐）：通过 Master 面板「远端直连节点 → 新增节点」一键脚本部署（强调确认公网 HTTPS `MASTER_URL` 与使用 `sudo bash`）；
     - 方式 B：独立命令行脚本 `deploy/install-agent.sh`（传入 `MASTER_URL` 与 `AGENT_TOKEN`，自动从 GitHub Releases 拉取二进制，可选 `AGENT_BIN_URL` 覆盖）；
     - 云主机（如 GCP）防火墙与最小宿主机前提说明（Agent 纯出站轮询，无需开放 8080/9000 入站；宿主机需已安装可执行 `xray` 与 `xray.service`，不展开 Xray 业务配置）；
     - `xray-agent` CLI 参数与环境变量速查表（严格对齐 `cmd/agent/main.go:38-51`）；
     - 常用服务管理与日志排查命令（`systemctl status xray-agent`、`journalctl -u xray-agent -f`）。
  3. 在「本地开发与编译」补充一行 `go build -ldflags="-s -w" -o xray-agent ./cmd/agent`。

## 2. Validation Commands (Quality Gates)
执行以下全部门禁并确保全绿：
```bash
# 1. 后端测试、静态检查与双二进制构建
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build .
mise x -- go build ./cmd/agent

# 2. 前端类型检查与生产构建
cd web && mise x -- npm run typecheck && mise x -- npm run build

# 3. Shell 脚本语法检查与 LF 换行检查
bash -n deploy/install-agent.sh
```

## 3. Risky Files & Rollback Points
- **`internal/service/node_service.go` (`RenderInstallScript`)**：严禁改动 `NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"` 占位符，保持与 `NodeTokenModal.vue` 及既有单测 100% 兼容。
- **`deploy/install-agent.sh`**：在 Windows 下创建文件必须确保纯 **LF (`\n`)** 换行符，避免在 Linux 上报 `$'\r': command not found`。
- **回滚点**：改动仅限 5 个明确文件，无迁移或外部副作用，单次 `git checkout` / `git revert` 即可完整回退。

## 4. Pre-`task.py start` Follow-up Checks
- 确认 `prd.md` 已完成无损收敛且无开放阻塞问题；
- 确认 `implement.jsonl` 与 `check.jsonl` 均已写入真实 `.trellis/spec/` 与 `research/` 路径；
- 等待用户明确批准更新后的规划总结、Phase 2 阵型与子代理模型后再执行 `task.py start`。
