#!/bin/bash

# Color definitions for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "$1"
}

log_success() {
    echo -e "${GREEN}$1${NC}"
}

log_error() {
    echo -e "${RED}$1${NC}"
}

log_step() {
    echo -e "${YELLOW}$1${NC}"
}


# Global variables
INSTALL_DIR="/opt/komari"
DATA_DIR="/opt/komari"
SERVICE_NAME="komari"
BINARY_PATH="$INSTALL_DIR/komari"
BACKUP_DIR="$INSTALL_DIR/backup"
DATA_BACKUP_DIR="$DATA_DIR/data/backup"
AGENT_SERVICE_NAME="komari-agent"
AGENT_BINARY_PATH="$INSTALL_DIR/agent"
AGENT_LOG_DIR="/var/log/komari"
INSTALLER_PATH="/usr/local/lib/komari/install-komari.sh"
COMMAND_PATH="/usr/local/bin/komari"
INSTALLER_URL="https://raw.githubusercontent.com/wander44-svg/komari/komari-optimal/install-komari.sh"
DEFAULT_PORT="25774"
LISTEN_PORT=""
REPO="wander44-svg/komari"
# 发布通道: stable（稳定版）或 snapshot（快照版）。
# optimal 分支默认使用最新 Snapshot，避免安装器回退到旧的 stable Release。
CHANNEL="${KOMARI_CHANNEL:-snapshot}"
# TUI 工具: whiptail / dialog / 空（回退纯文本）
TUI_TOOL=""

# ==========================================================
# TUI / 交互层
# ==========================================================

# 检测可用的 TUI 工具
detect_tui() {
    if command -v whiptail >/dev/null 2>&1; then
        TUI_TOOL="whiptail"
    elif command -v dialog >/dev/null 2>&1; then
        TUI_TOOL="dialog"
    else
        TUI_TOOL=""
    fi
}

# 是否启用 TUI
tui_enabled() {
    [ -n "$TUI_TOOL" ]
}

# 菜单选择
# 用法: ui_menu "标题" "提示" tag1 "item1" tag2 "item2" ...
# 返回: 选中的 tag（输出到 stdout），取消返回非零
ui_menu() {
    local title="$1"; shift
    local prompt="$1"; shift

    if tui_enabled; then
        # Leave enough rows for the complete fixed main menu so no scrollbar is shown.
        $TUI_TOOL --title "$title" --menu "$prompt" 24 78 12 "$@" 3>&1 1>&2 2>&3
        return $?
    fi

    # 纯文本回退
    {
        echo
        echo "=============================================================="
        echo "  $title"
        echo "=============================================================="
        echo "$prompt"
        echo
        local tag item
        local args=("$@")
        local i=0
        while [ $i -lt ${#args[@]} ]; do
            tag="${args[$i]}"
            item="${args[$((i + 1))]}"
            echo "  $tag) $item"
            i=$((i + 2))
        done
        echo
    } >&2
    local choice
    read -r -p "输入选项: " choice >&2
    echo "$choice"
}

# 输入框
# 用法: ui_input "标题" "提示" "默认值"
# 返回: 输入内容（输出到 stdout），取消返回非零
ui_input() {
    local title="$1"
    local prompt="$2"
    local default="$3"

    if tui_enabled; then
        $TUI_TOOL --title "$title" --inputbox "$prompt" 12 70 "$default" 3>&1 1>&2 2>&3
        return $?
    fi

    local input
    read -r -p "$prompt [默认: $default]: " input >&2
    if [ -z "$input" ]; then
        echo "$default"
    else
        echo "$input"
    fi
}

# 是/否确认
# 用法: ui_yesno "标题" "提示"
# 返回: 0 表示 是，1 表示 否
ui_yesno() {
    local title="$1"
    local prompt="$2"

    if tui_enabled; then
        $TUI_TOOL --title "$title" --yesno "$prompt" 12 70
        return $?
    fi

    local confirm
    read -r -p "$prompt (Y/n): " confirm >&2
    if [[ $confirm =~ ^[Nn]$ ]]; then
        return 1
    fi
    return 0
}

# 信息提示框
# 用法: ui_msgbox "标题" "内容"
ui_msgbox() {
    local title="$1"
    local content="$2"

    if tui_enabled; then
        $TUI_TOOL --title "$title" --msgbox "$content" 20 72
        return
    fi

    echo
    echo "=============================================================="
    echo "  $title"
    echo "=============================================================="
    echo -e "$content"
    echo "=============================================================="
    read -r -p "按回车键继续..." _
}

# 显示横幅（仅纯文本模式）
show_banner() {
    if tui_enabled; then
        return
    fi
    clear
    echo "=============================================================="
    echo "            Komari Monitoring System Installer"
    echo "       https://github.com/wander44-svg/komari"
    echo "=============================================================="
    echo
}

# 选择发布通道，结果写入全局变量 CHANNEL
select_channel() {
    local choice
    if ! choice=$(ui_menu "选择发布通道" "请选择要使用的发布通道：" \
        "1" "Stable      稳定版" \
        "2" "Snapshot   测试版"); then
        log_info "发布通道选择已取消"
        return 1
    fi

    case "$choice" in
        1|stable)
            CHANNEL="stable"
            ;;
        2|snapshot)
            CHANNEL="snapshot"
            ;;
        *)
            log_info "发布通道选择已取消"
            return 1
            ;;
    esac
    log_info "已选择通道: $CHANNEL"
}

# ==========================================================
# 基础检查
# ==========================================================

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "请使用 root 权限运行此脚本"
        exit 1
    fi
}

# Check for systemd
check_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        return 1
    else
        return 0
    fi
}

# Detect system architecture
detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64)
            echo "amd64"
            ;;
        aarch64)
            echo "arm64"
            ;;
        i386|i686)
            echo "386"
            ;;
        riscv64)
            echo "riscv64"
            ;;
        loongarch64|loong64)
            echo "loong64"
            ;;
        *)
            log_error "不支持的架构: $arch"
            exit 1
            ;;
    esac
}

# Check if Komari is already installed
is_installed() {
    if [ -f "$BINARY_PATH" ]; then
        return 0 # 0 means true in bash exit codes
    else
        return 1 # 1 means false
    fi
}

# Install dependencies
install_dependencies() {
    log_step "检查并安装依赖..."

    if ! command -v curl >/dev/null 2>&1; then
        if command -v apt >/dev/null 2>&1; then
            log_info "使用 apt 安装依赖..."
            apt update
            apt install -y curl
        elif command -v yum >/dev/null 2>&1; then
            log_info "使用 yum 安装依赖..."
            yum install -y curl
        elif command -v apk >/dev/null 2>&1; then
            log_info "使用 apk 安装依赖..."
            apk add curl
        else
            log_error "未找到支持的包管理器 (apt/yum/apk)"
            exit 1
        fi
    fi
}

# Get download URL based on channel
get_download_url() {
    local arch=$1
    local file_name="komari-linux-${arch}"

    if [ "$CHANNEL" = "snapshot" ]; then
        # 获取最新的 snapshot 预发布版本
        log_info "获取最新 snapshot 版本..." >&2
        local latest_snapshot=$(curl -s "https://api.github.com/repos/${REPO}/releases" | grep '"tag_name"' | grep 'Snapshot-' | head -1 | sed -e 's/.*"tag_name": *"//' -e 's/".*//')

        if [ -z "$latest_snapshot" ]; then
            log_error "未找到 snapshot 版本" >&2
            return 1
        fi

        log_info "最新 snapshot 版本: $latest_snapshot" >&2
        echo "https://github.com/${REPO}/releases/download/${latest_snapshot}/${file_name}"
    else
        # 稳定版：先获取最新正式 Release 的 tag，再按 tag 下载资产。
        log_info "获取最新 stable 版本..." >&2
        local latest_stable
        latest_stable=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed -e 's/.*"tag_name": *"//' -e 's/".*//')
        if [ -z "$latest_stable" ]; then
            log_error "未找到 stable 版本" >&2
            return 1
        fi
        log_info "最新 stable 版本: $latest_stable" >&2
        echo "https://github.com/${REPO}/releases/download/${latest_stable}/${file_name}"
    fi
}

# ==========================================================
# 业务操作
# ==========================================================

# Binary installation
install_binary() {
    local replacing=0
    local backup_path=""
    if is_installed; then
        replacing=1
        log_step "覆盖现有 Komari 安装，保留数据目录..."
    else
        log_step "开始二进制安装..."
    fi

    # 选择发布通道，整合菜单入口时可由调用方预先选择。
    if [ "${1:-}" != "--channel-selected" ] && ! select_channel; then
        log_info "安装已取消"
        return 0
    fi

    if [ "$replacing" -eq 0 ]; then
        # 首次安装时选择监听端口；覆盖安装保留已有面板设置和服务配置。
        while true; do
            local input_port
            input_port=$(ui_input "监听端口" "请输入 Komari 的监听端口 (1-65535)：" "$DEFAULT_PORT")
            # 取消输入
            if [ $? -ne 0 ]; then
                log_info "安装已取消"
                return
            fi
            if [[ -z "$input_port" ]]; then
                LISTEN_PORT="$DEFAULT_PORT"
                break
            elif [[ "$input_port" =~ ^[0-9]+$ ]] && (( input_port >= 1 && input_port <= 65535 )); then
                LISTEN_PORT="$input_port"
                break
            else
                ui_msgbox "错误" "端口号无效，请输入 1-65535 之间的数字。"
            fi
        done
    else
        LISTEN_PORT="$DEFAULT_PORT"
    fi

    install_dependencies

    if [ "$replacing" -eq 1 ] && check_systemd; then
        log_step "停止现有 Komari 服务..."
        systemctl stop "${SERVICE_NAME}.service" 2>/dev/null || true
    fi

    local arch=$(detect_arch)
    log_info "检测到架构: $arch"

    log_step "创建安装目录: $INSTALL_DIR"
    mkdir -p "$INSTALL_DIR"

    log_step "创建数据目录: $DATA_DIR"
    mkdir -p "$DATA_DIR"

    local download_url
    download_url=$(get_download_url "$arch")
    if [ $? -ne 0 ]; then
        check_systemd && [ "$replacing" -eq 1 ] && systemctl start "${SERVICE_NAME}.service" 2>/dev/null || true
        ui_msgbox "错误" "获取下载链接失败，请检查网络连接或稍后重试。"
        return 1
    fi

    log_step "下载 Komari 二进制文件..."
    log_info "URL: $download_url"

    local staged_binary="${BINARY_PATH}.download.$$"
    if ! curl -fL -o "$staged_binary" "$download_url"; then
        rm -f "$staged_binary"
        check_systemd && [ "$replacing" -eq 1 ] && systemctl start "${SERVICE_NAME}.service" 2>/dev/null || true
        ui_msgbox "错误" "下载失败，请检查网络连接。"
        return 1
    fi

    chmod +x "$staged_binary"
    if [ "$replacing" -eq 1 ]; then
        backup_path="${BINARY_PATH}.backup.$(date +%Y%m%d_%H%M%S)"
        if ! cp "$BINARY_PATH" "$backup_path"; then
            rm -f "$staged_binary"
            check_systemd && systemctl start "${SERVICE_NAME}.service" 2>/dev/null || true
            ui_msgbox "错误" "备份现有 Komari 二进制文件失败，安装已取消。"
            return 1
        fi
    fi
    mv -f "$staged_binary" "$BINARY_PATH"
    log_success "Komari 二进制文件安装完成: $BINARY_PATH"

    if ! check_systemd; then
        ui_msgbox "安装完成" "警告：未检测到 systemd，已跳过服务创建。\n\n您可以手动运行 Komari：\n    $BINARY_PATH server -l 0.0.0.0:$LISTEN_PORT"
        return
    fi

    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"
    if [ "$replacing" -eq 0 ] || [ ! -f "$service_file" ]; then
        create_systemd_service "$LISTEN_PORT"
    else
        log_info "保留现有 systemd 服务、面板端口和证书设置"
    fi

    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME}.service
    systemctl start ${SERVICE_NAME}.service

    if systemctl is-active --quiet ${SERVICE_NAME}.service; then
        log_success "Komari 服务启动成功"
        if [ "$replacing" -eq 1 ]; then
            ui_msgbox "覆盖安装完成" "Komari 已按 $CHANNEL 通道覆盖安装。\n\n原有数据、面板端口、证书路径和 systemd 服务配置均已保留。"
        else
            show_access_info "$LISTEN_PORT"
        fi
    else
        if [ -n "$backup_path" ] && [ -f "$backup_path" ]; then
            log_error "新版本服务启动失败，正在恢复原有二进制文件..."
            mv -f "$backup_path" "$BINARY_PATH"
            systemctl start "${SERVICE_NAME}.service" 2>/dev/null || true
        fi
        ui_msgbox "错误" "Komari 服务启动失败。\n\n查看日志: journalctl -u ${SERVICE_NAME} -f"
        return 1
    fi
}

# Create systemd service file
create_systemd_service() {
    local port="$1"
    log_step "创建 systemd 服务..."

    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"
    cat > "$service_file" << EOF
[Unit]
Description=Komari Monitor Service
After=network.target

[Service]
Type=simple
ExecStart=${BINARY_PATH} server -l 0.0.0.0:${port}
WorkingDirectory=${DATA_DIR}
Restart=always
User=root

[Install]
WantedBy=multi-user.target
EOF

    log_success "systemd 服务文件创建完成"
}

# Show access information
show_access_info() {
    local port=${1:-$DEFAULT_PORT}
    local ip=$(hostname -I | awk '{print $1}')

    local content="安装完成！\n\n"
    content+="访问信息：\n"
    content+="  URL: http://${ip}:${port}\n"
    content+="\n首次使用请访问上述地址，按安装向导创建管理员账号。\n"
    content+="\n服务管理命令：\n"
    content+="  状态: systemctl status $SERVICE_NAME\n"
    content+="  启动: systemctl start $SERVICE_NAME\n"
    content+="  停止: systemctl stop $SERVICE_NAME\n"
    content+="  重启: systemctl restart $SERVICE_NAME\n"
    content+="  日志: journalctl -u $SERVICE_NAME -f"

    ui_msgbox "安装完成" "$content"
}

# Remove historical upgrade backups
cleanup_backups() {
    if ! ui_yesno "确认清理备份" "将删除 Komari 的二进制升级备份和数据升级压缩包。\n\n您确定要继续吗？"; then
        log_info "备份清理已取消"
        return 0
    fi

    log_step "清理升级历史备份..."
    rm -f -- "${BINARY_PATH}.backup."*

    local backup_dir
    for backup_dir in "$BACKUP_DIR" "$DATA_BACKUP_DIR"; do
        if [ -d "$backup_dir" ]; then
            find "$backup_dir" -maxdepth 1 -type f -name '*.zip' -delete
        fi
    done

    ui_msgbox "清理完成" "升级历史备份已清理。\n\n清理范围：\n  二进制备份: ${BINARY_PATH}.backup.*\n  数据压缩包: $BACKUP_DIR\n  数据压缩包: $DATA_BACKUP_DIR"
}

# Uninstall function
uninstall_komari() {
    log_step "卸载 Komari..."

    local has_service=0
    if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
        has_service=1
    fi
    if ! is_installed && [ "$has_service" -eq 0 ] && [ ! -e "$COMMAND_PATH" ] && [ ! -e "$INSTALLER_PATH" ]; then
        ui_msgbox "提示" "Komari 未安装。"
        return 0
    fi

    if ! ui_yesno "确认卸载" "这将删除 Komari 二进制文件、服务和 komari 唤起命令。\n\n数据目录会保留，您确定要继续吗？"; then
        log_info "卸载已取消"
        return 0
    fi

    if check_systemd; then
        log_step "停止并禁用服务..."
        systemctl stop ${SERVICE_NAME}.service >/dev/null 2>&1
        systemctl disable ${SERVICE_NAME}.service >/dev/null 2>&1
        rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
        systemctl daemon-reload
        log_success "systemd 服务已删除"
    fi

    log_step "删除二进制文件..."
    rm -f "$BINARY_PATH"
    # 尝试在目录为空时删除该目录
    rmdir "$INSTALL_DIR" 2>/dev/null || log_info "数据目录 $INSTALL_DIR 不为空，未删除"
    log_success "Komari 二进制文件已删除"

    log_step "删除 komari 安装器命令..."
    rm -f "$COMMAND_PATH" "$INSTALLER_PATH"
    rmdir "$(dirname "$INSTALLER_PATH")" 2>/dev/null || true

    ui_msgbox "卸载完成" "Komari 卸载完成。\n\n数据文件保留在 $DATA_DIR\nkomari 命令已删除，如需重新安装请再次运行远程安装命令。"

    # The command and installer copy are removed above; leave this menu as well.
    # A later `komari` invocation must only be possible after reinstalling.
    tui_enabled && clear
    exit 0
}

# Show service status
show_status() {
    if ! is_installed; then
        ui_msgbox "错误" "Komari 未安装。"
        return
    fi
    if ! check_systemd; then
        ui_msgbox "错误" "未检测到 systemd。无法获取服务状态。"
        return
    fi
    if tui_enabled; then
        local status_output
        status_output=$(systemctl status ${SERVICE_NAME}.service --no-pager -l 2>&1)
        ui_msgbox "服务状态" "$status_output"
    else
        log_step "Komari 服务状态:"
        systemctl status ${SERVICE_NAME}.service --no-pager -l
        read -r -p "按回车键继续..." _
    fi
}

# Show service logs
show_logs() {
    if ! is_installed; then
        ui_msgbox "错误" "Komari 未安装。"
        return
    fi
    if ! check_systemd; then
        ui_msgbox "错误" "未检测到 systemd。无法获取服务日志。"
        return
    fi
    # 日志为实时流，直接在终端显示
    if tui_enabled; then
        clear
    fi
    log_step "查看 Komari 服务日志 (按 Ctrl+C 退出)..."
    journalctl -u ${SERVICE_NAME} -f --no-pager
}

# Restart service
restart_service() {
    if ! is_installed; then
        ui_msgbox "错误" "Komari 未安装。"
        return
    fi
    if ! check_systemd; then
        ui_msgbox "错误" "未检测到 systemd。无法重启服务。"
        return
    fi
    log_step "重启 Komari 服务..."
    systemctl restart ${SERVICE_NAME}.service
    if systemctl is-active --quiet ${SERVICE_NAME}.service; then
        ui_msgbox "成功" "服务重启成功。"
    else
        ui_msgbox "错误" "服务重启失败，请检查日志。"
    fi
}

# Stop service
stop_service() {
    if ! is_installed; then
        ui_msgbox "错误" "Komari 未安装。"
        return
    fi
    if ! check_systemd; then
        ui_msgbox "错误" "未检测到 systemd。无法停止服务。"
        return
    fi
    log_step "停止 Komari 服务..."
    systemctl stop ${SERVICE_NAME}.service
    ui_msgbox "成功" "服务已停止。"
}

# Install or overwrite using the release channel selected in the submenu.
install_selected_channel() {
    if ! select_channel; then
        log_info "操作已取消"
        return 0
    fi

    if is_installed; then
        log_step "按所选通道覆盖安装 Komari (当前通道: $CHANNEL)..."
    else
        log_step "按所选通道安装 Komari (当前通道: $CHANNEL)..."
    fi
    install_binary --channel-selected
}

# Uninstall Komari Agent and remove its service and logs.
uninstall_agent() {
    if ! ui_yesno "确认卸载 Komari Agent" \
        "这将停止并删除 Komari Agent 服务、程序文件和日志。\n\n您确定要继续吗？"; then
        log_info "Komari Agent 卸载已取消"
        return 0
    fi

    log_step "停止并删除 Komari Agent..."
    local agent_service_file="/etc/systemd/system/${AGENT_SERVICE_NAME}.service"
    if check_systemd; then
        systemctl stop "$AGENT_SERVICE_NAME" 2>/dev/null || true
        systemctl disable "$AGENT_SERVICE_NAME" 2>/dev/null || true
    else
        log_info "未检测到 systemd，跳过服务处理"
    fi
    rm -f "$agent_service_file"
    check_systemd && systemctl daemon-reload
    rm -rf "$AGENT_BINARY_PATH" "$AGENT_LOG_DIR"

    if [ -e "$agent_service_file" ] || [ -e "$AGENT_BINARY_PATH" ] || [ -e "$AGENT_LOG_DIR" ]; then
        ui_msgbox "卸载失败" "部分 Komari Agent 文件未能删除，请检查文件权限后重试。"
        return 1
    fi

    ui_msgbox "卸载完成" "Komari Agent 已卸载。\n\n已删除：\n  服务: ${AGENT_SERVICE_NAME}.service\n  程序: $AGENT_BINARY_PATH\n  日志: $AGENT_LOG_DIR"
}

# Remove the complete Komari installation and data directory.
delete_komari_data() {
    if ! ui_yesno "确认删除 Komari 数据" \
        "这将停止 Komari 服务并删除 /opt/komari 下的全部程序和数据。\n\n此操作不可恢复，您确定要继续吗？"; then
        log_info "Komari 数据删除已取消"
        return 0
    fi

    log_step "停止并删除 Komari 服务..."
    local service_file="/etc/systemd/system/${SERVICE_NAME}.service"
    if check_systemd; then
        systemctl stop "$SERVICE_NAME" 2>/dev/null || true
        systemctl disable "$SERVICE_NAME" 2>/dev/null || true
    else
        log_info "未检测到 systemd，跳过服务处理"
    fi
    rm -f "$service_file"
    check_systemd && systemctl daemon-reload
    rm -rf "$INSTALL_DIR"

    if [ -e "$service_file" ] || [ -e "$INSTALL_DIR" ]; then
        ui_msgbox "删除失败" "部分 Komari 文件或数据未能删除，请检查文件权限后重试。"
        return 1
    fi

    ui_msgbox "删除完成" "Komari 数据已删除。\n\n已删除：\n  服务: ${SERVICE_NAME}.service\n  目录: $INSTALL_DIR\n\n输入 komari 可再次打开安装器并重新安装。"
}

# Install a persistent `komari` command that opens this installer.
install_cli_command() {
    local script_source="${BASH_SOURCE[0]}"
    mkdir -p "$(dirname "$INSTALLER_PATH")"

    # Process substitution (bash <(curl ...)) exposes a transient /dev/fd path;
    # fetch a complete copy in that case instead of copying an already-consumed FD.
    if [ "$script_source" = "$INSTALLER_PATH" ]; then
        :
    elif [ -r "$script_source" ] && [[ "$script_source" != /dev/fd/* ]] && [[ "$script_source" != /proc/*/fd/* ]]; then
        local staged_path="${INSTALLER_PATH}.tmp.$$"
        if ! cp "$script_source" "$staged_path"; then
            rm -f "$staged_path"
            return 1
        fi
        mv -f "$staged_path" "$INSTALLER_PATH"
    elif command -v curl >/dev/null 2>&1; then
        local staged_path="${INSTALLER_PATH}.tmp.$$"
        if ! curl -fsSL "$INSTALLER_URL" -o "$staged_path"; then
            rm -f "$staged_path"
            return 1
        fi
        mv -f "$staged_path" "$INSTALLER_PATH"
    else
        log_error "无法保存安装器：未找到当前脚本文件或 curl"
        return 1
    fi
    chmod 755 "$INSTALLER_PATH"

    cat > "$COMMAND_PATH" << EOF
#!/bin/sh
# Komari installer command wrapper.
exec /usr/bin/env bash "$INSTALLER_PATH" "\$@"
EOF
    chmod 755 "$COMMAND_PATH"
    log_success "已安装 komari 命令：输入 komari 可打开安装器"
}


# Main menu
main_menu() {
    while true; do
        show_banner

        local choice
        choice=$(ui_menu "Komari 监控系统安装器" "请选择操作：" \
            "1" "安装 Komari" \
            "2" "卸载 Komari" \
            "3" "卸载 Komari Agent" \
            "4" "删除 Komari 数据" \
            "5" "查看状态" \
            "6" "查看日志" \
            "7" "重启服务" \
            "8" "停止服务" \
            "9" "清理升级历史备份" \
            "10" "退出")

        # 用户在 TUI 中取消（ESC/Cancel）则退出
        if [ $? -ne 0 ] && tui_enabled; then
            clear
            exit 0
        fi

        case $choice in
            1) install_selected_channel ;;
            2) uninstall_komari ;;
            3) uninstall_agent ;;
            4) delete_komari_data ;;
            5) show_status ;;
            6) show_logs ;;
            7) restart_service ;;
            8) stop_service ;;
            9) cleanup_backups ;;
            10)
                tui_enabled && clear
                exit 0 
                ;;
            *) ui_msgbox "错误" "无效选项" ;;
        esac

        # 纯文本模式下单次执行后退出循环（保持原有行为，避免输出被覆盖）
        if ! tui_enabled; then
            break
        fi
    done
}

# Main execution
check_root
detect_tui
install_cli_command || log_info "提示：未能安装 komari 命令，可稍后重新运行安装器重试。"
main_menu
