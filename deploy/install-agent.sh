#!/usr/bin/env bash
# ==============================================================================
# Xray Panel Thin Agent (xray-agent) 一键安装与升级脚本 (Linux Systemd)
# ==============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PLAIN='\033[0m'

BIN_PATH="/usr/local/bin/xray-agent"
CONF_DIR="/etc/xray-agent"
SPOOL_DIR="/var/lib/xray-agent"
ENV_FILE="${CONF_DIR}/agent.env"
SERVICE_FILE="/etc/systemd/system/xray-agent.service"

if [[ $EUID -ne 0 ]]; then
  echo -e "${RED}错误：必须使用 root 用户执行此安装脚本（如非 root 请使用 sudo bash deploy/install-agent.sh）！${PLAIN}" >&2
  exit 1
fi

ARCH_RAW="$(uname -m)"
case "${ARCH_RAW}" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo -e "${RED}错误：不支持的 CPU 架构: ${ARCH_RAW}（仅支持 amd64 / arm64）${PLAIN}" >&2
    exit 1
    ;;
esac

MASTER_URL_INPUT="${MASTER_URL:-}"
AGENT_TOKEN_INPUT="${AGENT_TOKEN:-}"
WRITE_ENV=0

if [[ ! -f "${ENV_FILE}" ]]; then
  if [[ -z "${MASTER_URL_INPUT}" || -z "${AGENT_TOKEN_INPUT}" ]]; then
    echo -e "${RED}错误：首次安装必须同时提供 MASTER_URL 与 AGENT_TOKEN 环境变量！${PLAIN}" >&2
    echo -e "${YELLOW}示例：sudo MASTER_URL=\"https://panel.example.com\" AGENT_TOKEN=\"<YOUR_NODE_TOKEN>\" bash deploy/install-agent.sh${PLAIN}" >&2
    exit 1
  fi
  WRITE_ENV=1
else
  if [[ -n "${MASTER_URL_INPUT}" && -n "${AGENT_TOKEN_INPUT}" ]]; then
    WRITE_ENV=1
  elif [[ -n "${MASTER_URL_INPUT}" || -n "${AGENT_TOKEN_INPUT}" ]]; then
    echo -e "${RED}错误：检测到已有 ${ENV_FILE}。升级重跑时请同时传入 MASTER_URL 和 AGENT_TOKEN 以覆盖凭据，或两者均不传以保留现有凭据。${PLAIN}" >&2
    exit 1
  fi
fi

if [[ "${WRITE_ENV}" -eq 1 ]] && [[ ! "${MASTER_URL_INPUT}" =~ ^https?://[^[:space:]]+$ ]]; then
  echo -e "${RED}错误：MASTER_URL 必须为合法的 http:// 或 https:// 地址（例如 https://panel.example.com）。${PLAIN}" >&2
  exit 1
fi

if [[ "${WRITE_ENV}" -eq 1 ]] && [[ "${AGENT_TOKEN_INPUT}" == "<YOUR_NODE_TOKEN>" ]]; then
  echo -e "${RED}错误：请将 AGENT_TOKEN 替换为节点实际的明文 Token，不能直接使用占位符 <YOUR_NODE_TOKEN>。${PLAIN}" >&2
  exit 1
fi

echo -e "${BLUE}======================================================${PLAIN}"
echo -e "${GREEN}      开始安装 / 升级 Xray Panel Thin Agent           ${PLAIN}"
echo -e "${BLUE}======================================================${PLAIN}"

if ! command -v xray >/dev/null 2>&1 && [[ ! -x /usr/local/bin/xray ]]; then
  echo -e "${YELLOW}[WARN] 未检测到 xray 可执行文件（PATH 或 /usr/local/bin/xray）。请确保宿主机已安装 xray 与 xray.service 以承接配置下发。${PLAIN}" >&2
fi

AGENT_BIN_URL="${AGENT_BIN_URL:-https://github.com/coding-leaf/xray-panel/releases/latest/download/xray-agent-linux-${ARCH}}"
TMP_BIN="$(mktemp /tmp/xray-agent.XXXXXX)"
trap 'rm -f "${TMP_BIN}"' EXIT

echo -e "${GREEN}[1/4] 下载 xray-agent (${ARCH}) 从 ${AGENT_BIN_URL} ...${PLAIN}"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL --retry 3 --connect-timeout 15 "${AGENT_BIN_URL}" -o "${TMP_BIN}"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${TMP_BIN}" "${AGENT_BIN_URL}"
else
  echo -e "${RED}错误：未找到 curl 或 wget，无法下载 xray-agent。${PLAIN}" >&2
  exit 1
fi

if [[ ! -s "${TMP_BIN}" ]]; then
  echo -e "${RED}错误：下载的 xray-agent 二进制文件为空。${PLAIN}" >&2
  exit 1
fi

if systemctl is-active --quiet xray-agent 2>/dev/null; then
  systemctl stop xray-agent
fi
install -m 0755 "${TMP_BIN}" "${BIN_PATH}"

echo -e "${GREEN}[2/4] 配置运行目录与节点凭据 ${ENV_FILE} ...${PLAIN}"
install -d -m 0750 "${CONF_DIR}" "${SPOOL_DIR}"
if [[ "${WRITE_ENV}" -eq 1 ]]; then
  (
    umask 077
    cat >"${ENV_FILE}" <<XRAY_AGENT_ENV_EOF
MASTER_URL=${MASTER_URL_INPUT%/}
AGENT_TOKEN=${AGENT_TOKEN_INPUT}
XRAY_AGENT_ENV_EOF
  )
  chmod 0600 "${ENV_FILE}"
  echo -e "${GREEN}  ✓ 已写入权限为 0600 的节点凭据文件 ${ENV_FILE}${PLAIN}"
else
  chmod 0600 "${ENV_FILE}"
  echo -e "${YELLOW}  ✓ 保留现有节点凭据文件 ${ENV_FILE}${PLAIN}"
fi

echo -e "${GREEN}[3/4] 配置 Systemd 后台服务 ${SERVICE_FILE} ...${PLAIN}"
cat >"${SERVICE_FILE}" <<XRAY_AGENT_SERVICE_EOF
[Unit]
Description=Xray Panel Thin Agent
After=network-online.target xray.service
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=${ENV_FILE}
ExecStart=${BIN_PATH} --spool-dir ${SPOOL_DIR} --interval 60s
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
XRAY_AGENT_SERVICE_EOF
chmod 0644 "${SERVICE_FILE}"

systemctl daemon-reload
systemctl enable xray-agent.service
systemctl restart xray-agent.service

echo -e "${GREEN}[4/4] 检查服务运行状态 ...${PLAIN}"
sleep 1
if systemctl is-active --quiet xray-agent.service; then
  echo -e "${GREEN}======================================================${PLAIN}"
  echo -e "${GREEN}  ✓ xray-agent 安装并启动成功！                       ${PLAIN}"
  echo -e "${BLUE}  - 二进制路径: ${BIN_PATH}                           ${PLAIN}"
  echo -e "${BLUE}  - 凭据文件:   ${ENV_FILE} (0600)                    ${PLAIN}"
  echo -e "${BLUE}  - 状态目录:   ${SPOOL_DIR}                          ${PLAIN}"
  echo -e "${BLUE}  - 查看状态:   systemctl status xray-agent           ${PLAIN}"
  echo -e "${BLUE}  - 查看日志:   journalctl -u xray-agent -f           ${PLAIN}"
  echo -e "${GREEN}======================================================${PLAIN}"
else
  echo -e "${RED}警告：xray-agent.service 启动未就绪，请使用 'systemctl status xray-agent' 与 'journalctl -u xray-agent -e' 查看详细日志！${PLAIN}" >&2
  exit 1
fi
