# Implementation Plan: 节点弹窗单行一键安装命令体验优化

## 1. Ordered Implementation Checklist

### Step 1: 后端 `RenderInstallScript` heredoc 去冲突（Fail-Repro First）
- 目标文件：`internal/service/node_service_test.go`、`internal/service/node_service.go`
- 步骤：
  1. 在 `TestNodeService_RenderInstallScript_EnvFileAndInterval` 新增断言：必须包含 `XRAY_AGENT_ENV_EOF` 与 `XRAY_AGENT_SERVICE_EOF`，且不得包含 `<<EOF`；先跑单测确认红灯；
  2. 修改 `internal/service/node_service.go` 中 `RenderInstallScript` 的两个 `<<EOF ... EOF` 为 `<<XRAY_AGENT_ENV_EOF ... XRAY_AGENT_ENV_EOF` 与 `<<XRAY_AGENT_SERVICE_EOF ... XRAY_AGENT_SERVICE_EOF`；
  3. 重跑 `mise x -- go test ./internal/service -run TestNodeService_RenderInstallScript -v` 确认绿灯。

### Step 2: 增强 `deploy/install-agent.sh`（UTF-8 纯 LF）
- 目标文件：`deploy/install-agent.sh`
- 步骤：
  1. 在环境变量校验处增加对 `AGENT_TOKEN_INPUT == "<YOUR_NODE_TOKEN>"` 的显式拦截报错（防止未替换占位符写入 `agent.env`）；
  2. 将内部两个 `<<EOF ... EOF` 替换为 `<<XRAY_AGENT_ENV_EOF` 与 `<<XRAY_AGENT_SERVICE_EOF`；
  3. 用 `bash -n deploy/install-agent.sh` 与 Python 字节检查确认语法通过且 `\r` 数为 0。

### Step 3: 重构 `web/src/views/nodes/components/NodeTokenModal.vue`
- 目标文件：`web/src/views/nodes/components/NodeTokenModal.vue`
- 步骤：
  1. 引入 `ChevronDown`、`ChevronRight`（来自 `lucide-vue-next`）；
  2. 实现 `shellSingleQuote` 与 `oneLineCommand` computed（基于 `masterUrl` 与 `modalState.token || '<YOUR_NODE_TOKEN>'`）；
  3. 将原脚本区升级为：
     - **主区块（默认可见）**：`一键安装命令（推荐，复制后直接粘贴到 SSH 终端回车）` + 独立复制按钮 + 单行命令代码块 + 一行适配提示（支持 GCP/AWS/Debian/Ubuntu 等非 root 与 root 宿主机，极简容器无 `sudo` 时去掉 `sudo` 即可）；
     - **折叠区块（`showFullScript` 默认 `false`，弹窗关闭时重置）**：`查看完整 systemd 脚本（高级 / 审计）` 切换按钮，展开后展示原 `renderedScript` `<pre>` 及独立「复制完整脚本」按钮；
  4. 底部主按钮改为复制 `oneLineCommand`（文案：`复制一键安装命令`）；
  5. 运行 `cd web && mise x -- npm run typecheck && mise x -- npm run build && mise x -- npm run lint` 及 `npx prettier --check src/views/nodes/components/NodeTokenModal.vue`。

### Step 4: 同步更新 `README.md`
- 目标文件：`README.md`
- 步骤：
  1. 更新「多节点 Agent 部署（`xray-agent`）→ 方式 A / 方式 B」，突出面板默认「单行复制直接粘贴 SSH 回车」体验，并给出对应的 `curl -fsSL https://raw.githubusercontent.com/coding-leaf/xray-panel/master/deploy/install-agent.sh | sudo MASTER_URL='...' AGENT_TOKEN='...' bash` 用法；
  2. 保留本地执行 `deploy/install-agent.sh` 与面板折叠查看完整脚本的说明。

## 2. Validation Commands (Quality Gates)
```bash
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build -o panel.tmp.exe . && rm -f panel.tmp.exe
mise x -- go build -o agent.tmp.exe ./cmd/agent && rm -f agent.tmp.exe
cd web && mise x -- npm run typecheck && mise x -- npm run build && mise x -- npm run lint && npx prettier --check src/views/nodes/components/NodeTokenModal.vue
bash -n deploy/install-agent.sh
```
并校验 `deploy/install-agent.sh` 中 `\r` 字节数为 0，根目录无残留临时二进制。

## 3. Risky Files & Rollback Points
- `NodeTokenModal.vue`：不要破坏 `renderedScript` 的 `NODE_TOKEN` 替换逻辑，且要确保 `oneLineCommand` 对用户输入的 `masterUrl` 做好单引号安全转义。
- `deploy/install-agent.sh`：Windows 下编辑必须保持纯 LF 换行。
