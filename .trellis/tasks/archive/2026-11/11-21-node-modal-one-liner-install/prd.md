# 节点弹窗单行一键安装命令体验优化 (PRD)

## Goal & User Value
让第一次使用 `xray-panel` 的管理员在 Master 面板创建或查看远端节点时，真正做到「**点一下复制 → 切到 SSH 终端粘贴 → 回车即装完**」，彻底消除此前 70 行多行脚本裸贴进 Windows SSH 终端易串行截断、`exit 1` 踢断 SSH 会话、以及 `cat << 'EOF'` 嵌套 heredoc 冲突三大真实踩坑点。

## Confirmed Facts & Evidence
- `web/src/views/nodes/components/NodeTokenModal.vue:95-145` 当前将 70+ 行完整 systemd 脚本作为主展示区和主复制按钮目标，第一次使用的用户习惯直接粘贴到交互式 SSH 终端，导致长行截断或 `exit 1` 断连；即使手动用 `cat << 'EOF' > /tmp/install-agent.sh`，也会因脚本内部使用了同名 `EOF` 分隔符而提前截断。
- `deploy/install-agent.sh:1-146` 已合入 `master`（UTF-8 纯 LF），无任何 stdin 交互读取或 `$0`/`BASH_SOURCE` 依赖，天然支持 `curl -fsSL ... | sudo MASTER_URL='...' AGENT_TOKEN='...' bash` 单行管道执行；但当前未拦截占位符 `AGENT_TOKEN="<YOUR_NODE_TOKEN>"`，且内部 heredoc 仍使用 `EOF`。
- `internal/service/node_service.go:689-788` 中的 `RenderInstallScript` 内部 heredoc 使用 `EOF`（L758、L765、L779）；既有单测 `TestNodeService_RenderInstallScript_EnvFileAndInterval`（`internal/service/node_service_test.go:1222-1280`）断言了 `EnvironmentFile=/etc/xray-agent/agent.env`、`0600`、`id -u`、`sudo bash` 等契约，未绑定具体 heredoc 名称 `EOF`。

## Requirements

### R1: 前端 `NodeTokenModal.vue` 升级为「单行命令主入口 + 完整脚本可折叠」
- 在 `web/src/views/nodes/components/NodeTokenModal.vue` 中：
  - **主入口（默认展开且作为底部主 CTA）**：新增「一键安装命令（复制后直接粘贴到 SSH 终端回车）」区域，实时根据 `masterUrl` 与 `modalState.token`（无明文 Token 时回退占位符 `<YOUR_NODE_TOKEN>`）生成安全单引号转义的单行命令：
    ```bash
    curl -fsSL https://raw.githubusercontent.com/coding-leaf/xray-panel/master/deploy/install-agent.sh | sudo MASTER_URL='<safeMasterUrl>' AGENT_TOKEN='<tokenOrPlaceholder>' bash
    ```
  - 命令区域下方补充一行简明说明：适用于 GCP / AWS / Debian / Ubuntu 等非 root 与 root 宿主机（极简容器若无 `sudo` 可去掉命令中的 `sudo`）。
  - 底部主按钮改为「复制一键安装命令」（默认复制单行命令）。
  - **次入口（默认折叠）**：将原有的完整 systemd 脚本（`renderedScript`）收入可折叠的「查看完整脚本（高级 / 审计）」区块（带展开/收起切换图标与独立的「复制完整脚本」按钮）；保持 `renderedScript` 中对 `NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"` 的替换契约不变。

### R2: 消除 `RenderInstallScript` 与 `deploy/install-agent.sh` 的 heredoc `EOF` 冲突并拦截占位符 Token
- 修改 `internal/service/node_service.go` (`RenderInstallScript`)：
  - 将内部两个 heredoc 分隔符 `EOF` 替换为不易撞车的 `XRAY_AGENT_ENV_EOF` 与 `XRAY_AGENT_SERVICE_EOF`，保证用户手动 `cat << 'EOF'` 保存完整脚本时不再被提前截断；
  - 在 `internal/service/node_service_test.go` 补充断言验证无裸 `<<EOF`，并保留全部既有断言全绿。
- 修改 `deploy/install-agent.sh`（保持 UTF-8 纯 LF）：
  - 增加 `AGENT_TOKEN_INPUT == "<YOUR_NODE_TOKEN>"` 拦截校验，避免从「查看安装脚本」弹窗复制占位符未替换直接运行导致把占位符写入 `/etc/xray-agent/agent.env`；
  - 同步将内部 heredoc 分隔符 `EOF` 改为 `XRAY_AGENT_ENV_EOF` 与 `XRAY_AGENT_SERVICE_EOF`。

### R3: 更新 `README.md` 与前端单行命令体验对齐
- 更新 `README.md` 的「多节点 Agent 部署（`xray-agent`）→ 方式 A / 方式 B」：
  - 方式 A（推荐）说明在 Master 面板弹窗直接复制单行 `curl ... | sudo MASTER_URL='...' AGENT_TOKEN='...' bash` 命令粘贴进 SSH 回车即可完成，并说明弹窗内同时提供折叠的完整脚本供审计；
  - 方式 B 给出同款远程拉取单行命令示例及本地仓库 `sudo ... bash deploy/install-agent.sh` 用法。

## Acceptance Criteria
- [ ] **AC-1（首屏单行即贴即跑）**：`NodeTokenModal.vue` 默认展示并主推单行 `curl ... | sudo MASTER_URL='...' AGENT_TOKEN='...' bash` 命令（正确做单引号转义，跟随 `masterUrl` 实时变化，有一次性 Token 时自动内嵌，无明文 Token 时显示 `<YOUR_NODE_TOKEN>`），原有完整脚本默认折叠且可一键展开/复制。
- [ ] **AC-2（零 heredoc 冲突与占位符防护）**：`RenderInstallScript` 与 `deploy/install-agent.sh` 均使用 `XRAY_AGENT_ENV_EOF` / `XRAY_AGENT_SERVICE_EOF`（无裸 `<<EOF`），且 `deploy/install-agent.sh` 显式拒绝 `<YOUR_NODE_TOKEN>` 占位符、为纯 LF 换行并通过 `bash -n`。
- [ ] **AC-3（后端门禁与单测全绿）**：`mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .`、`mise x -- go build ./cmd/agent` 全绿，且 `TestNodeService_RenderInstallScript_EnvFileAndInterval` 新增防 `EOF` 冲突断言通过。
- [ ] **AC-4（前端门禁全绿）**：`cd web && mise x -- npm run typecheck && mise x -- npm run build && mise x -- npm run lint` 与 `prettier --check` 全绿，嵌入式 `web/dist/` 同步更新。
- [ ] **AC-5（文档一致与安全红线）**：`README.md` 与弹窗体验完全对齐，所有改动文件无真实公网 IP、生产域名、Token 或私钥。

## Out of Scope
- 新增 Master 后端免登录脚本下载 HTTP 路由（直接复用 GitHub `raw.githubusercontent.com/.../deploy/install-agent.sh`，零新增攻击面）。
- Xray-core 安装或业务配置教程。
- 版本号发布 / Tag / 远程部署（待用户验收确认后再按需执行）。
