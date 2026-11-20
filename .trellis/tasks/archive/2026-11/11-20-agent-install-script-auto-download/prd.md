# Agent 安装脚本自动拉取 GitHub Release 二进制

## Goal
补齐多节点 Agent 安装闭环：在 GitHub Release 工作流同步构建发布 `xray-agent-linux-amd64` 与 `xray-agent-linux-arm64`，并升级 `NodeService.RenderInstallScript` 生成的安装脚本，使其在远端机器执行时自动识别架构、从 GitHub Release 拉取并原子安装 `/usr/local/bin/xray-agent`（支持 `AGENT_BIN_URL` 覆盖与缺失 `xray` 时的清晰提示），彻底消除“脚本未下载二进制导致启动失败”的断层。

## Requirements
1. **R1（`.github/workflows/release.yml` 补齐 `xray-agent` 构建发布）**：
   - 在 Release 流程增加 `xray-agent-linux-amd64` 与 `xray-agent-linux-arm64` 构建（纯 Go `CGO_ENABLED=0`，`-trimpath -ldflags="-s -w -X 'main.Version=${TAG_VERSION}'"`，不依赖前端构建产物），上传 `binary-xray-agent-*` 产物以便 `publish-release` 自动一并生成 `checksums-sha256.txt` 并发布至 GitHub Release。
2. **R2（`internal/service/node_service.go:688-744` `RenderInstallScript` 自动拉取逻辑）**：
   - 保留现有 `MASTER_URL`、`NODE_TOKEN` 校验（保持 `<YOUR_NODE_TOKEN>` 占位符与前端 `NodeTokenModal.vue` 替换完全兼容）、`0600` `/etc/xray-agent/agent.env` + `EnvironmentFile=` 与 `--interval 60s` 安全契约；
   - 新增架构自动检测（`uname -m`：`x86_64|amd64 -> amd64`，`aarch64|arm64 -> arm64`，其余架构明确报错退出）；
   - 新增 `xray` 核心存在性预检（`command -v xray` 或 `/usr/local/bin/xray`），缺失时输出清晰告警提示（不阻断安装，提示需先安装 Xray-core）；
   - 新增二进制自动拉取与原子替换：默认 `AGENT_BIN_URL="https://github.com/coding-leaf/xray-panel/releases/latest/download/xray-agent-linux-${ARCH}"`（允许执行前通过环境变量 `AGENT_BIN_URL` 自定义镜像或内网地址），优先 `curl -fsSL --retry 3`（回退 `wget -qO`）下载至临时文件，校验文件非空后，若已有运行中的 `xray-agent` 先停止再通过 `install -m 0755` 原子安装至 `/usr/local/bin/xray-agent` 并清理临时文件；
   - 随后写入 `agent.env`、`xray-agent.service` 并执行 `systemctl daemon-reload && systemctl enable --now xray-agent`。
3. **R3（单测覆盖 `internal/service/node_service_test.go`）**：
   - 更新/扩展 `TestNodeService_RenderInstallScript_EnvFileAndInterval`，覆盖架构检测、`AGENT_BIN_URL` / GitHub Release 默认下载链接、`curl`/`wget` 拉取、`install -m 0755 /usr/local/bin/xray-agent`、`xray` 缺失提示，以及既有 `0600` `agent.env` / `EnvironmentFile=` / 零明文 `--token` / `--interval 60s` 断言。

## Out of Scope
- 不自动下载/安装 `xray-core` 本身（仅检测并提示）；
- 不修改前端 `NodeTokenModal.vue` UI 或数据库 Schema。

## Acceptance Criteria
1. `.github/workflows/release.yml` 包含 `xray-agent-linux-amd64` 与 `xray-agent-linux-arm64` 构建与 Release 上传，且本地 `go build` 验证命令正确。
2. `RenderInstallScript` 输出的 bash 脚本通过 `bash -n`（如宿主可用）语法检查，且包含架构检测、`xray` 缺失提示、默认 GitHub Release 拉取 + `AGENT_BIN_URL` 覆盖、`install -m 0755 /usr/local/bin/xray-agent` 及全部 M1 安全/间隔不变量。
3. 后端全量门禁通过：`mise x -- go test -count=1 ./...`、`CGO_ENABLED=1 mise x -- go test -race -count=1 ./...`、`mise x -- go vet ./...`、`mise x -- go build -o NUL .`、`mise x -- go build -o NUL ./cmd/agent`。
