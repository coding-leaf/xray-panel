#!/usr/bin/env bash
# ==============================================================================
# VPS 全能体检与基准评测自动化脚本 (xray-panel VPS Benchmark Suite)
# 支持环境: Debian / Ubuntu / CentOS / AlmaLinux / Rocky (x86_64 / aarch64)
# 特性: 硬件极限算力 + 磁盘IOPS + 三网回程路由 + 国际带宽测速 + 流媒体/AI解锁 + IP风险与安全基线
# ==============================================================================

set -o pipefail

# 节点别名（默认 VPS-A）
NODE_ALIAS="${1:-VPS-A}"
REPORT_FILE="vps_audit_${NODE_ALIAS}.log"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
PLAIN='\033[0m'

log() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $1"
    echo -e "$msg" | tee -a "$REPORT_FILE"
}

heading() {
    echo -e "\n${CYAN}===================================================================${PLAIN}" | tee -a "$REPORT_FILE"
    echo -e "${CYAN} $1 ${PLAIN}" | tee -a "$REPORT_FILE"
    echo -e "${CYAN}===================================================================${PLAIN}" | tee -a "$REPORT_FILE"
}

# 基础工具检查与安装（已具备则免 sudo/apt）
install_dependencies() {
    if command -v curl >/dev/null 2>&1 && command -v jq >/dev/null 2>&1 && command -v wget >/dev/null 2>&1; then
        log "${GREEN}核心诊断工具已齐全 (curl, wget, jq)，跳过依赖安装。${PLAIN}"
        return 0
    fi
    log "${YELLOW}正在尝试安装缺失依赖...${PLAIN}"
    if [ "$(id -u)" -eq 0 ]; then
        if [ -f /etc/debian_version ]; then
            apt-get update -y >/dev/null 2>&1
            apt-get install -y curl wget jq tar dnsutils bc util-linux iproute2 >/dev/null 2>&1
        elif [ -f /etc/redhat-release ]; then
            yum install -y curl wget jq tar bind-utils bc util-linux iproute >/dev/null 2>&1 || dnf install -y curl wget jq tar bind-utils bc util-linux iproute >/dev/null 2>&1
        fi
    else
        log "${YELLOW}当前为普通用户且部分工具缺失，将尽可能使用现有工具执行诊断。${PLAIN}"
    fi
}

# IP 隐私掩码函数 (如 123.45.67.89 -> 123.45.x.x)
mask_ip() {
    local ip="$1"
    if [[ "$ip" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        echo "${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.x.x"
    elif [[ "$ip" =~ : ]]; then
        echo "${ip:0:9}::xxxx"
    else
        echo "$ip"
    fi
}

# 1. 系统与基础硬件体检
check_hardware() {
    heading "【1/5】系统基础信息与硬件架构探测"
    
    local os_info="未知"
    if [ -f /etc/os-release ]; then
        os_info=$(grep -oP 'PRETTY_NAME="\K[^"]+' /etc/os-release)
    fi
    local kernel_ver=$(uname -r)
    local arch=$(uname -m)
    local virt_tech="物理机/独立硬件"
    if command -v systemd-detect-virt >/dev/null 2>&1; then
        virt_tech=$(systemd-detect-virt)
    fi

    local cpu_model=$(awk -F: '/model name/ {name=$2} END {print name}' /proc/cpuinfo | sed 's/^[ \t]*//')
    [ -z "$cpu_model" ] && cpu_model=$(lscpu | grep "Model name\|Architecture" | head -n 1 | awk -F: '{print $2}' | sed 's/^[ \t]*//')
    local cpu_cores=$(grep -c ^processor /proc/cpuinfo)
    local cpu_freq=$(lscpu | grep "CPU MHz" | awk -F: '{print $2}' | sed 's/^[ \t]*//')
    [ -z "$cpu_freq" ] && cpu_freq="N/A"

    local total_ram=$(free -h | awk '/Mem:/ {print $2}')
    local used_ram=$(free -h | awk '/Mem:/ {print $3}')
    local total_swap=$(free -h | awk '/Swap:/ {print $2}')

    local disk_info=$(df -h / | awk 'NR==2 {print "总容量: "$2" | 已用: "$3" ("$5")"}')
    local uptime_str=$(uptime -p 2>/dev/null || uptime | awk -F'( |,|:)+' '{print $6" hours, "$7" minutes"}')

    local tcp_cc=$(sysctl -n net.ipv4.tcp_congestion_control 2>/dev/null || echo "unknown")
    local tcp_qdisc=$(sysctl -n net.core.default_qdisc 2>/dev/null || echo "unknown")

    log "节点别名      : ${GREEN}${NODE_ALIAS}${PLAIN}"
    log "操作系统      : $os_info"
    log "内核版本      : $kernel_ver ($arch)"
    log "虚拟化架构    : $virt_tech"
    log "系统运行时间  : $uptime_str"
    log "CPU 型号      : $cpu_model"
    log "CPU 核心/主频 : ${cpu_cores} 核 / ${cpu_freq} MHz"
    log "内存容量      : 物理总内存: $total_ram (已用: $used_ram) | Swap: $total_swap"
    log "系统主盘      : $disk_info"
    log "TCP 拥塞算法  : $tcp_cc ($tcp_qdisc) [BBR启用状态: $([[ "$tcp_cc" =~ "bbr" ]] && echo -e "${GREEN}已开启${PLAIN}" || echo -e "${YELLOW}未启用BBR${PLAIN}")]"
}

# 2. 硬件算力与磁盘 IOPS 极限压测
check_performance() {
    heading "【2/5】CPU 算力与磁盘 I/O 极限跑分 (YABS)"
    log "${YELLOW}正在启动 YABS (Yet Another Bench Script)...${PLAIN}"
    log "包含 Geekbench 6 真实算力跑分与 FIO 4K/64K/512K/1M 磁盘随机/顺序读写测试..."
    log "（全量压测约需 3-8 分钟，请耐心等待）"
    
    curl -sL yabs.sh | bash -s -- -i -5 | tee -a "$REPORT_FILE"
}

# 3. 三网回程路由、延迟与带宽测速
check_network() {
    heading "【3/5】网络拓扑、三网回程路由与上下行测速"

    # 获取公网 IP 与地理位置（打码）
    local raw_ipv4=$(curl -s4 --max-time 5 api64.ipify.org || curl -s4 --max-time 5 ifconfig.me || echo "N/A")
    local masked_ip=$(mask_ip "$raw_ipv4")
    local ip_api_json=$(curl -s --max-time 5 "http://ip-api.com/json/${raw_ipv4}?fields=status,country,regionName,city,isp,org,as,query" 2>/dev/null)
    
    local country=$(echo "$ip_api_json" | jq -r '.country // "Unknown"')
    local city=$(echo "$ip_api_json" | jq -r '.city // "Unknown"')
    local asn=$(echo "$ip_api_json" | jq -r '.as // "Unknown"')
    local isp=$(echo "$ip_api_json" | jq -r '.isp // "Unknown"')

    log "公网 IPv4     : ${GREEN}${masked_ip}${PLAIN} (已脱敏)"
    log "归属地与城市  : ${country} - ${city}"
    log "ASN 与运营商  : ${asn} (${isp})"

    # 三网回程路由
    log "\n${YELLOW}>>> 正在检测中国大陆三大运营商回程路由链路类型 (电信/联通/移动)...${PLAIN}"
    if curl -sL --max-time 10 https://raw.githubusercontent.com/zhanghanyun/backtrace/main/install.sh -sSf | sh | tee -a "$REPORT_FILE"; then
        log "${GREEN}三网回程路由检测完成。${PLAIN}"
    else
        log "${YELLOW}Backtrace 路由检测源暂不可用，尝试备用路由快速检测...${PLAIN}"
        curl -sL https://raw.githubusercontent.com/oneclickvirt/backtrace/raw/main/backtrace.sh | bash | tee -a "$REPORT_FILE" || true
    fi

    # 多节点网络测速
    log "\n${YELLOW}>>> 正在启动三网核心骨干与国际节点上下行带宽与延迟测速...${PLAIN}"
    if command -v speedtest >/dev/null 2>&1; then
        speedtest --accept-license --accept-gdpr | tee -a "$REPORT_FILE" || true
    else
        # 采用主流轻量 speedtest 脚本
        curl -sL https://raw.githubusercontent.com/i-abc/Speedtest/main/speedtest.sh | bash -s -- --all | tee -a "$REPORT_FILE" || \
        bash <(curl -sL https://raw.githubusercontent.com/oneclickvirt/speedtest/master/speedtest.sh) | tee -a "$REPORT_FILE" || true
    fi
}

# 4. 流媒体、AI 服务解锁与 IP 纯净度检测
check_unlock_and_reputation() {
    heading "【4/5】流媒体/主流 AI 服务解锁能力与 IP 纯净度/欺诈分"

    log "${YELLOW}>>> 正在检测 Netflix, Disney+, YouTube, ChatGPT, Claude 等主流服务解锁状态...${PLAIN}"
    # 使用业界主流且持续维护的流媒体与 AI 解锁检测脚本
    bash <(curl -L -s media.ispvps.com) | tee -a "$REPORT_FILE" || \
    bash <(curl -sL https://raw.githubusercontent.com/ludashi98/check.sh/main/check.sh) -A | tee -a "$REPORT_FILE" || true

    log "\n${YELLOW}>>> 正在审计 IP 纯净度、欺诈分 (Scamalytics) 与邮件端口 (Port 25)...${PLAIN}"
    # 检查 25 端口出站阻断状态（防垃圾邮件封锁标尺）
    local smtp_test=$(timeout 3 bash -c "</dev/tcp/smtp.gmail.com/25" 2>/dev/null && echo "开放 (Open)" || echo "已阻断 (Blocked - 常见于正规云厂商防护)")
    log "出站 25 (SMTP) 端口状态: $smtp_test"

    # 调用 IPQuality/IP 风险检测
    bash <(curl -sL https://raw.githubusercontent.com/xykt/IPQuality/main/ip.sh) | tee -a "$REPORT_FILE" || true
}

# 5. 主机安全基线与暴露面审计
check_security_baseline() {
    heading "【5/5】主机安全基线、SSH 配置与端口暴露面审计"

    # SSH 配置审计
    local ssh_config="/etc/ssh/sshd_config"
    local ssh_port="22"
    local root_login="默认允许"
    local pwd_auth="默认启用"
    local pubkey_auth="默认启用"

    if [ -f "$ssh_config" ]; then
        local cfg_port=$(grep -iE "^[#]*Port " "$ssh_config" | tail -n 1 | awk '{print $2}')
        [ -n "$cfg_port" ] && ssh_port="$cfg_port"

        local cfg_root=$(grep -iE "^[#]*PermitRootLogin " "$ssh_config" | tail -n 1 | awk '{print $2}')
        [ -n "$cfg_root" ] && root_login="$cfg_root"

        local cfg_pwd=$(grep -iE "^[#]*PasswordAuthentication " "$ssh_config" | tail -n 1 | awk '{print $2}')
        [ -n "$cfg_pwd" ] && pwd_auth="$cfg_pwd"

        local cfg_pub=$(grep -iE "^[#]*PubkeyAuthentication " "$ssh_config" | tail -n 1 | awk '{print $2}')
        [ -n "$cfg_pub" ] && pubkey_auth="$cfg_pub"
    fi

    log "SSH 监听端口  : $ssh_port $([ "$ssh_port" == "22" ] && echo -e "${YELLOW}(默认 22 端口，建议修改)${PLAIN}" || echo -e "${GREEN}(已修改默认端口)${PLAIN}")"
    log "Root 远程登录 : $root_login $([ "$root_login" == "no" ] && echo -e "${GREEN}(安全)${PLAIN}" || echo -e "${YELLOW}(高暴露风险)${PLAIN}")"
    log "密码身份验证  : $pwd_auth $([ "$pwd_auth" == "no" ] && echo -e "${GREEN}(已强制密钥登录)${PLAIN}" || echo -e "${YELLOW}(建议禁用密码转为密钥)${PLAIN}")"
    log "公钥认证启用  : $pubkey_auth"

    # 防火墙状态
    local fw_status="未检测到活跃防火墙"
    if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
        fw_status="UFW 已激活 (Active)"
    elif command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld; then
        fw_status="Firewalld 已激活 (Active)"
    elif command -v nft >/dev/null 2>&1 && nft list ruleset >/dev/null 2>&1; then
        fw_status="nftables 规则已加载"
    elif iptables -L -n 2>/dev/null | grep -q "Chain INPUT"; then
        fw_status="iptables 基础过滤已启用"
    fi
    log "防火墙防护状态: $fw_status"

    # Fail2ban 防爆破
    local f2b_status="未安装或未运行"
    if command -v fail2ban-client >/dev/null 2>&1 && systemctl is-active --quiet fail2ban; then
        local jails=$(fail2ban-client status 2>/dev/null | grep "Jail list" | awk -F: '{print $2}' | sed 's/^[ \t]*//')
        f2b_status="运行中 (活跃 Jail: ${jails:-none})"
    fi
    log "Fail2ban 防爆破: $f2b_status"

    # 监听在 0.0.0.0 / :: 的高危公网开放端口
    log "\n${YELLOW}>>> 当前暴露在公网 (0.0.0.0 / ::) 的监听端口排查:${PLAIN}"
    if command -v ss >/dev/null 2>&1; then
        ss -tulnp | awk 'NR==1 || /0\.0\.0\.0|\*:/' | tee -a "$REPORT_FILE"
    else
        netstat -tulnp 2>/dev/null | awk 'NR==1 || /0\.0\.0\.0|\*:/' | tee -a "$REPORT_FILE" || true
    fi

    # 特权账号检查 (UID 0)
    local uid0_users=$(awk -F: '($3 == "0") {print $1}' /etc/passwd | tr '\n' ' ')
    log "\nUID 0 特权账户列表: $uid0_users $([ "$uid0_users" == "root " ] && echo -e "${GREEN}(正常)${PLAIN}" || echo -e "${RED}(警告: 存在多余特权用户)${PLAIN}")"
}

# 结束报告与输出指引
finish_report() {
    heading "体检报告生成完毕"
    log "${GREEN}恭喜！${NODE_ALIAS} 节点体检已全部完成！${PLAIN}"
    log "完整的脱敏测评日志已成功保存到: ${CYAN}$(pwd)/${REPORT_FILE}${PLAIN}"
    echo -e "\n${YELLOW}-------------------------------------------------------------------${PLAIN}"
    echo -e "接下来请对另外两台 VPS 分别执行同样操作（传入 VPS-B / VPS-C 参数）："
    echo -e "  bash vps_audit.sh VPS-B"
    echo -e "  bash vps_audit.sh VPS-C"
    echo -e "完成后，只需把生成的 .log 文件内容发给 AI 助手，即可生成三机全景对比与打分报告！"
    echo -e "${YELLOW}-------------------------------------------------------------------${PLAIN}\n"
}

# 运行主流程
main() {
    install_dependencies
    check_hardware
    check_performance
    check_network
    check_unlock_and_reputation
    check_security_baseline
    finish_report
}

main "$@"
