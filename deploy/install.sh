#!/usr/bin/env bash
# ==============================================================================
# Xray Decoupled Panel 一键安装与系统服务托管脚本 (Linux Systemd)
# ==============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PLAIN='\033[0m'

INSTALL_DIR="/usr/local/xray-panel"
DATA_DIR="${INSTALL_DIR}/data"
SERVICE_FILE="/etc/systemd/system/panel.service"

[[ $EUID -ne 0 ]] && echo -e "${RED}错误：必须使用 root 用户执行此安装脚本！${PLAIN}" && exit 1

echo -e "${BLUE}======================================================${PLAIN}"
echo -e "${GREEN}      开始安装 / 升级 Xray Decoupled Panel 面板       ${PLAIN}"
echo -e "${BLUE}======================================================${PLAIN}"

# 1. 检查并创建工作目录
mkdir -p "${INSTALL_DIR}"
mkdir -p "${DATA_DIR}"
chmod 700 "${DATA_DIR}"

# 2. 寻找并复制二进制执行程序
PANEL_SRC=""
for p in "./panel" "./bin/panel" "../panel" "../bin/panel" "/tmp/panel"; do
    if [[ -f "$p" ]]; then
        PANEL_SRC="$p"
        break
    fi
done

if [[ -n "${PANEL_SRC}" ]]; then
    echo -e "${GREEN}[1/4] 发现并安装执行程序 (${PANEL_SRC}) 到 ${INSTALL_DIR}/panel ...${PLAIN}"
    # 停止旧进程，并使用 install 命令进行原子替换，天然免疫 Linux 'Text file busy' 报错
    systemctl stop panel.service 2>/dev/null || true
    install -m 755 "${PANEL_SRC}" "${INSTALL_DIR}/panel"
else
    echo -e "${RED}错误：未找到 panel 二进制文件！请确认已执行过编译，或将 panel 放置在当前目录 / /tmp/ 下。${PLAIN}"
    exit 1
fi

# 3. 配置 Systemd 服务（如首次安装自动生成 32 位高熵随机 JWT 密钥）
echo -e "${GREEN}[2/4] 配置 Systemd 后台服务 ${SERVICE_FILE} ...${PLAIN}"
if [[ ! -f "${SERVICE_FILE}" ]]; then
    RANDOM_SECRET=$(head -c 64 /dev/urandom 2>/dev/null | tr -dc A-Za-z0-9 | head -c 32 || date +%s%N | md5sum | head -c 32)
    cat << EOF > "${SERVICE_FILE}"
[Unit]
Description=Xray Decoupled Panel Daemon
Documentation=https://github.com/coding-leaf/xray-panel
After=network.target xray.service
Wants=xray.service

[Service]
Type=simple
User=root
WorkingDirectory=/usr/local/xray-panel
ExecStart=/usr/local/xray-panel/panel
Restart=on-failure
RestartSec=5s
LimitNOFILE=65535
Environment="PANEL_JWT_SECRET=${RANDOM_SECRET}"

[Install]
WantedBy=multi-user.target
EOF
    echo -e "${GREEN}  ✓ 首次安装已自动生成高熵随机 JWT 密钥并配置入 Service！${PLAIN}"
fi

# 4. 重载与启动服务
echo -e "${GREEN}[3/4] 重载 Systemd 守护进程并启动 panel ...${PLAIN}"
systemctl daemon-reload
systemctl enable panel.service
systemctl restart panel.service

# 5. 检查运行状态
echo -e "${GREEN}[4/4] 检查服务运行状态 ...${PLAIN}"
sleep 1
if systemctl is-active --quiet panel.service; then
    echo -e "${GREEN}======================================================${PLAIN}"
    echo -e "${GREEN}  ✓ Xray Decoupled Panel 面板安装并启动成功！         ${PLAIN}"
    echo -e "${BLUE}  - 面板访问地址: http://<你的服务器IP>:9000           ${PLAIN}"
    echo -e "${BLUE}  - 初始默认账户: admin / admin123                    ${PLAIN}"
    echo -e "${BLUE}  - 数据存储目录: ${DATA_DIR}                         ${PLAIN}"
    echo -e "${BLUE}  - 服务管理命令: systemctl restart panel              ${PLAIN}"
    echo -e "${BLUE}  - 查看运行日志: journalctl -u panel -f               ${PLAIN}"
    echo -e "${GREEN}======================================================${PLAIN}"
else
    echo -e "${RED}警告：panel.service 启动未就绪，请使用 'journalctl -u panel -e' 查看详细日志！${PLAIN}"
fi
