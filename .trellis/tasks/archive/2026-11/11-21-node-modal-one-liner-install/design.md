# Technical Design: 节点弹窗单行一键安装命令体验优化

## 1. Architecture & Boundaries
本次改造聚焦运维 UX 与脚本防坑，不改动任何数据库 Schema、Agent 同步协议或后端公共 API 路由，仅涉及 4 个明确边界：

1. **前端弹窗 (`web/src/views/nodes/components/NodeTokenModal.vue`)**：主推单行 `curl | sudo bash` 安装命令，默认折叠原有完整脚本。
2. **独立安装脚本 (`deploy/install-agent.sh`)**：补 `<YOUR_NODE_TOKEN>` 占位符拦截校验 + 独立 heredoc 分隔符，保持纯 LF。
3. **后端脚本模板 (`internal/service/node_service.go` + `node_service_test.go`)**：替换内部 `EOF` 为专用分隔符，消除 `cat << 'EOF'` 嵌套截断坑。
4. **文档 (`README.md`)**：与单行一键粘贴体验保持一致。

## 2. Key Technical Details & Contracts

### 2.1 单行命令构造与 Shell 安全转义 (`NodeTokenModal.vue`)
- 常量 URL：`https://raw.githubusercontent.com/coding-leaf/xray-panel/master/deploy/install-agent.sh`
- Shell 单引号转义辅助函数（纯函数，无外部依赖）：
  ```ts
  const shellSingleQuote = (val: string) => `'${val.replace(/'/g, `'\\''`)}'`
  ```
- `normalizedMasterUrl`：`masterUrl.value.trim().replace(/\/+$/, '') || 'https://panel.example.com'`
- `tokenValue`：`props.modalState?.token?.trim() || '<YOUR_NODE_TOKEN>'`
- 生成的单行命令：
  ```ts
  `curl -fsSL ${INSTALL_AGENT_SCRIPT_URL} | sudo MASTER_URL=${shellSingleQuote(normalizedMasterUrl)} AGENT_TOKEN=${shellSingleQuote(tokenValue)} bash`
  ```
- 安全性：
  - 使用单引号包裹 `MASTER_URL` 与 `AGENT_TOKEN`，阻断 `$`、`` ` ``、`!` 等 Shell 扩展和注入；
  - `sudo VAR=val bash` 在标准 sudoers (`env_reset`) 下由 `sudo` 显式透传给 `bash`，无需修改宿主机 sudo 配置；
  - 不需要新增后端接口，也不把 Token 放进 URL query（避免进入反代/Nginx/CDN 访问日志）。

### 2.2 完整脚本折叠交互与向后兼容
- `showFullScript = ref(false)`，每次关闭弹窗重置为 `false`。
- 保持原有 `loadInstallScript()` 调用与 `renderedScript` 中对 `NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"` 的精确字符串替换契约不变，确保高级用户展开后看到的脚本功能与之前 100% 一致。
- 底部主按钮改为一键复制 `oneLineCommand`（文案清晰标注「复制一键安装命令」）；展开后的完整脚本区自带「复制完整脚本」次级按钮。

### 2.3 Heredoc 分隔符去冲突与占位符拦截
- `internal/service/node_service.go` (`RenderInstallScript`) 与 `deploy/install-agent.sh`：
  - `agent.env` heredoc 使用 `<<'XRAY_AGENT_ENV_EOF'` / `<<XRAY_AGENT_ENV_EOF`（需展开 `${MASTER_URL}` / `${NODE_TOKEN}`，因此保持无引号 `<<XRAY_AGENT_ENV_EOF`）与闭合行 `XRAY_AGENT_ENV_EOF`；
  - `xray-agent.service` heredoc 使用 `<<XRAY_AGENT_SERVICE_EOF` 与闭合行 `XRAY_AGENT_SERVICE_EOF`；
- `deploy/install-agent.sh` 在 `WRITE_ENV == 1` 分支校验 `[[ "${AGENT_TOKEN_INPUT}" == "<YOUR_NODE_TOKEN>" ]]`，命中时输出清晰报错并 `exit 1`，避免用户未替换占位符时污染 `/etc/xray-agent/agent.env`。

## 3. Trade-offs & Rollback
- **为什么用 GitHub raw URL 而非新增 Master 端点**：KISS，避免新增未鉴权端点或引入一次性下载票据状态管理；且本项目的 `xray-agent` 二进制本身就是从 GitHub Releases 下载，目标节点天然需要可达 GitHub。
- **回滚**：仅涉及 UI 展示、脚本模板与文档，无数据迁移，单次 `git revert` 零副作用恢复。
