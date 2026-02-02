#!/usr/bin/env bash
#
# Paqet Install Script
# Unified installer for both server and client on Linux with systemd
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/hanselime/paqet/master/install-paqet.sh | sudo bash
#   curl -sSL ... | sudo bash -s -- --server -y
#   curl -sSL ... | sudo bash -s -- --client -y -s <IP> -p <PORT> -k <KEY>
#
set -euo pipefail

# ============================================================================
# Constants
# ============================================================================
SCRIPT_VERSION="1.0.0"
GITHUB_REPO="hanselime/paqet"
GITHUB_API="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/paqet"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
SERVICE_FILE="/etc/systemd/system/paqet.service"
BINARY_PATH="${INSTALL_DIR}/paqet"

# Default values
DEFAULT_PORT=9999
DEFAULT_ENCRYPTION="aes"
DEFAULT_KCP_MODE="fast"
DEFAULT_LOG_LEVEL="info"
DEFAULT_LOCAL_PORT=1080

# Command line arguments (for non-interactive mode)
ARG_MODE=""           # server or client
ARG_YES=false         # Non-interactive
ARG_PORT=""
ARG_SERVER_ADDR=""
ARG_SECRET_KEY=""
ARG_LOCAL_PORT=""
ARG_FORWARD_TARGET=""
ARG_PROXY_MODE=""     # socks5 or forward

# ============================================================================
# Color Output
# ============================================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'
BOLD='\033[1m'
DIM='\033[2m'

# ============================================================================
# Utility Functions
# ============================================================================
log_info()    { echo -e "${BLUE}[INFO]${NC} $*" >&2; }
log_success() { echo -e "${GREEN}[OK]${NC} $*" >&2; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $*" >&2; }
log_error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }
log_step()    { echo -e "${CYAN}==>${NC} ${BOLD}$*${NC}" >&2; }

print_line() {
    printf '%*s\n' "${1:-70}" '' | tr ' ' "${2:-─}"
}

print_header() {
    echo
    print_line 70 "═"
    echo -e "${BOLD}${CYAN}$*${NC}"
    print_line 70 "═"
}

print_section() {
    echo
    print_line 70 "─"
    echo -e "${BOLD}$*${NC}"
    print_line 70 "─"
}

prompt() {
    local var_name="$1"
    local prompt_text="$2"
    local default="${3:-}"
    local value

    if [[ -n "$default" ]]; then
        read -rp "$(echo -e "${CYAN}?${NC} ${prompt_text} [${default}]: ")" value
        value="${value:-$default}"
    else
        read -rp "$(echo -e "${CYAN}?${NC} ${prompt_text}: ")" value
    fi

    printf -v "$var_name" '%s' "$value"
}

prompt_password() {
    local var_name="$1"
    local prompt_text="$2"
    local value

    read -rsp "$(echo -e "${CYAN}?${NC} ${prompt_text}: ")" value
    echo
    printf -v "$var_name" '%s' "$value"
}

confirm() {
    local prompt_text="$1"
    local default="${2:-y}"
    local yn

    if [[ "$default" == "y" ]]; then
        read -rp "$(echo -e "${CYAN}?${NC} ${prompt_text} [Y/n]: ")" yn
        yn="${yn:-y}"
    else
        read -rp "$(echo -e "${CYAN}?${NC} ${prompt_text} [y/N]: ")" yn
        yn="${yn:-n}"
    fi

    [[ "$yn" =~ ^[Yy]$ ]]
}

# ============================================================================
# Pre-flight Checks
# ============================================================================
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        echo "Please run: sudo $0"
        exit 1
    fi
}

check_linux() {
    if [[ "$(uname -s)" != "Linux" ]]; then
        log_error "This script only supports Linux"
        exit 1
    fi
}

check_systemd() {
    if ! command -v systemctl &>/dev/null; then
        log_error "systemd is required but not found"
        exit 1
    fi

    if ! systemctl --version &>/dev/null; then
        log_error "systemd is not functioning properly"
        exit 1
    fi
}

check_dependencies() {
    local missing=()

    if ! command -v curl &>/dev/null; then
        missing+=("curl")
    fi

    if ! command -v awk &>/dev/null; then
        missing+=("awk")
    fi

    if ! command -v ip &>/dev/null; then
        missing+=("iproute2 (ip command)")
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing[*]}"
        echo "Please install them first:"
        echo "  apt-get install -y curl gawk iproute2    # Debian/Ubuntu"
        echo "  dnf install -y curl gawk iproute         # Fedora/RHEL"
        exit 1
    fi

    # iptables is optional but warn if missing
    if ! command -v iptables &>/dev/null; then
        log_warn "iptables not found - firewall rules cannot be configured automatically"
    fi
}

run_preflight_checks() {
    log_step "Running pre-flight checks..."

    check_root
    log_success "Running as root"

    check_linux
    log_success "Linux detected"

    check_systemd
    log_success "systemd available"

    check_dependencies
    log_success "Dependencies satisfied"

    echo
}

# ============================================================================
# Detection Functions
# ============================================================================
detect_arch() {
    local arch
    arch=$(uname -m)

    case "$arch" in
        x86_64)
            echo "linux-amd64"
            ;;
        aarch64|arm64)
            echo "linux-arm64"
            ;;
        armv7l|armv6l)
            echo "linux-arm"
            ;;
        *)
            log_error "Unsupported architecture: $arch"
            exit 1
            ;;
    esac
}

detect_network() {
    log_step "Detecting network configuration..."

    # Initialize variables to avoid unset variable errors
    DETECTED_IFACE=""
    DETECTED_IP=""
    DETECTED_GATEWAY=""
    DETECTED_GATEWAY_MAC=""

    # Detect interface
    DETECTED_IFACE=$(ip route 2>/dev/null | awk '/default/ {print $5; exit}' || true)
    if [[ -z "$DETECTED_IFACE" ]]; then
        log_warn "Could not auto-detect network interface"
    else
        log_info "Interface: $DETECTED_IFACE"
    fi

    # Detect local IP
    if [[ -n "$DETECTED_IFACE" ]]; then
        DETECTED_IP=$(ip -4 addr show "$DETECTED_IFACE" 2>/dev/null | awk '/inet / {print $2}' | cut -d/ -f1 | head -1 || true)
        if [[ -n "$DETECTED_IP" ]]; then
            log_info "Local IP: $DETECTED_IP"
        fi
    fi

    # Detect gateway
    DETECTED_GATEWAY=$(ip route 2>/dev/null | awk '/default/ {print $3; exit}' || true)
    if [[ -n "$DETECTED_GATEWAY" ]]; then
        log_info "Gateway: $DETECTED_GATEWAY"

        # Ping gateway to ensure it's in ARP table
        ping -c1 -W1 "$DETECTED_GATEWAY" &>/dev/null || true

        # Get gateway MAC
        DETECTED_GATEWAY_MAC=$(awk -v gw="$DETECTED_GATEWAY" '$1 == gw {print $4}' /proc/net/arp 2>/dev/null || true)
        if [[ -n "$DETECTED_GATEWAY_MAC" ]]; then
            log_info "Gateway MAC: $DETECTED_GATEWAY_MAC"
        else
            log_warn "Could not detect gateway MAC address"
        fi
    fi

    echo
}

is_installed() {
    [[ -f "$BINARY_PATH" ]] && [[ -f "$CONFIG_FILE" ]]
}

get_installed_role() {
    if [[ -f "$CONFIG_FILE" ]]; then
        grep -oP '^role:\s*"\K[^"]+' "$CONFIG_FILE" 2>/dev/null || echo ""
    else
        echo ""
    fi
}

get_installed_version() {
    if [[ -f "$BINARY_PATH" ]]; then
        "$BINARY_PATH" version 2>/dev/null | head -1 || echo "unknown"
    else
        echo "not installed"
    fi
}

# ============================================================================
# GitHub Functions
# ============================================================================
get_latest_version() {
    log_step "Fetching latest release information..."

    LATEST_VERSION=""

    local response
    response=$(curl -sSL --connect-timeout 10 "$GITHUB_API" 2>/dev/null) || {
        log_error "Failed to fetch release information from GitHub"
        log_info "Check your internet connection or try again later"
        return 1
    }

    # Try perl regex first, fall back to extended regex
    LATEST_VERSION=$(echo "$response" | grep -oP '"tag_name":\s*"\K[^"]+' 2>/dev/null | head -1 || true)
    if [[ -z "$LATEST_VERSION" ]]; then
        # Fallback for systems without grep -P
        LATEST_VERSION=$(echo "$response" | grep -oE '"tag_name":\s*"[^"]+"' | head -1 | sed 's/.*"tag_name":\s*"//;s/"//' || true)
    fi

    if [[ -z "$LATEST_VERSION" ]]; then
        log_error "Could not parse latest version from GitHub API"
        return 1
    fi

    log_info "Latest version: $LATEST_VERSION"
}

download_binary() {
    local arch="$1"
    local version="$2"
    local archive_name="paqet-${arch}-${version}.tar.gz"
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${archive_name}"
    local temp_archive="/tmp/paqet-archive-$$.tar.gz"
    local temp_dir="/tmp/paqet-extract-$$"
    local temp_file="/tmp/paqet-download-$$"

    log_step "Downloading paqet ${version} for ${arch}..."
    log_info "URL: $download_url"

    local retry=0
    local max_retries=3

    while [[ $retry -lt $max_retries ]]; do
        if curl -sSL --connect-timeout 30 --max-time 120 -o "$temp_archive" "$download_url" 2>/dev/null; then
            # Verify it's a valid gzip file
            if file "$temp_archive" 2>/dev/null | grep -q "gzip"; then
                log_success "Download complete"

                # Extract the binary
                log_step "Extracting binary..."
                mkdir -p "$temp_dir"
                if tar -xzf "$temp_archive" -C "$temp_dir" 2>/dev/null; then
                    # Find the paqet binary in extracted files (may be named paqet_linux_amd64 etc)
                    local extracted_binary
                    extracted_binary=$(find "$temp_dir" -maxdepth 1 -name "paqet*" -type f ! -name "*.md" ! -name "*.yaml*" 2>/dev/null | head -1)

                    if [[ -n "$extracted_binary" ]] && file "$extracted_binary" 2>/dev/null | grep -q "ELF"; then
                        mv "$extracted_binary" "$temp_file"
                        rm -rf "$temp_dir" "$temp_archive"
                        log_success "Extraction complete"
                        echo "$temp_file"
                        return 0
                    else
                        log_warn "Could not find valid binary in archive"
                        log_warn "Archive contents: $(ls -la "$temp_dir" 2>/dev/null)"
                    fi
                else
                    log_warn "Failed to extract archive"
                fi
                rm -rf "$temp_dir"
            else
                log_warn "Downloaded file is not a valid gzip archive"
            fi
        fi

        retry=$((retry + 1))
        if [[ $retry -lt $max_retries ]]; then
            log_warn "Download failed, retrying ($retry/$max_retries)..."
            sleep 2
        fi
    done

    log_error "Failed to download binary after $max_retries attempts"
    rm -f "$temp_archive" "$temp_file"
    rm -rf "$temp_dir"
    return 1
}

install_binary() {
    local temp_file="$1"

    log_step "Installing binary..."

    # Create install directory if needed
    mkdir -p "$INSTALL_DIR"

    # Install binary
    mv "$temp_file" "$BINARY_PATH"
    chmod 755 "$BINARY_PATH"

    log_success "Installed to $BINARY_PATH"

    # Verify installation
    if "$BINARY_PATH" version &>/dev/null; then
        log_success "Binary verification passed"
    else
        log_warn "Binary installed but version check failed"
    fi
}

# ============================================================================
# Configuration Functions
# ============================================================================
generate_secret() {
    log_step "Generating secret key..."

    SECRET_KEY=$("$BINARY_PATH" secret 2>/dev/null)
    if [[ -z "$SECRET_KEY" ]] || [[ ${#SECRET_KEY} -ne 64 ]]; then
        log_error "Failed to generate secret key"
        return 1
    fi

    log_success "Secret key generated"
}

create_config_dir() {
    mkdir -p "$CONFIG_DIR"
    chmod 700 "$CONFIG_DIR"
}

create_server_config() {
    local interface="$1"
    local ip="$2"
    local port="$3"
    local gateway_mac="$4"
    local encryption="$5"
    local kcp_mode="$6"
    local log_level="$7"
    local secret_key="$8"

    log_step "Creating server configuration..."

    create_config_dir

    cat > "$CONFIG_FILE" <<EOF
role: "server"

log:
  level: "${log_level}"

listen:
  addr: ":${port}"

network:
  interface: "${interface}"
  ipv4:
    addr: "${ip}:${port}"
    router_mac: "${gateway_mac}"
  tcp:
    local_flag: ["PA"]
  pcap:
    sockbuf: 8388608

transport:
  protocol: "kcp"
  conn: 1
  kcp:
    mode: "${kcp_mode}"
    mtu: 1350
    rcvwnd: 1024
    sndwnd: 1024
    block: "${encryption}"
    key: "${secret_key}"
    smuxbuf: 4194304
    streambuf: 2097152
EOF

    chmod 600 "$CONFIG_FILE"
    log_success "Configuration saved to $CONFIG_FILE"
}

create_client_config_socks() {
    local interface="$1"
    local client_ip="$2"
    local gateway_mac="$3"
    local server_addr="$4"
    local server_port="$5"
    local local_port="$6"
    local encryption="$7"
    local kcp_mode="$8"
    local log_level="$9"
    local secret_key="${10}"

    log_step "Creating client configuration (SOCKS5 mode)..."

    create_config_dir

    cat > "$CONFIG_FILE" <<EOF
role: "client"

log:
  level: "${log_level}"

socks5:
  - listen: "127.0.0.1:${local_port}"

network:
  interface: "${interface}"
  ipv4:
    addr: "${client_ip}:0"
    router_mac: "${gateway_mac}"
  tcp:
    local_flag: ["PA"]
    remote_flag: ["PA"]
  pcap:
    sockbuf: 4194304

server:
  addr: "${server_addr}:${server_port}"

transport:
  protocol: "kcp"
  conn: 1
  kcp:
    mode: "${kcp_mode}"
    mtu: 1350
    rcvwnd: 512
    sndwnd: 512
    block: "${encryption}"
    key: "${secret_key}"
    smuxbuf: 4194304
    streambuf: 2097152
EOF

    chmod 600 "$CONFIG_FILE"
    log_success "Configuration saved to $CONFIG_FILE"
}

create_client_config_forward() {
    local interface="$1"
    local client_ip="$2"
    local gateway_mac="$3"
    local server_addr="$4"
    local server_port="$5"
    local local_port="$6"
    local forward_target="$7"
    local encryption="$8"
    local kcp_mode="$9"
    local log_level="${10}"
    local secret_key="${11}"

    log_step "Creating client configuration (TCP Forward mode)..."

    create_config_dir

    cat > "$CONFIG_FILE" <<EOF
role: "client"

log:
  level: "${log_level}"

forward:
  - listen: "127.0.0.1:${local_port}"
    target: "${forward_target}"

network:
  interface: "${interface}"
  ipv4:
    addr: "${client_ip}:0"
    router_mac: "${gateway_mac}"
  tcp:
    local_flag: ["PA"]
    remote_flag: ["PA"]
  pcap:
    sockbuf: 4194304

server:
  addr: "${server_addr}:${server_port}"

transport:
  protocol: "kcp"
  conn: 1
  kcp:
    mode: "${kcp_mode}"
    mtu: 1350
    rcvwnd: 512
    sndwnd: 512
    block: "${encryption}"
    key: "${secret_key}"
    smuxbuf: 4194304
    streambuf: 2097152
EOF

    chmod 600 "$CONFIG_FILE"
    log_success "Configuration saved to $CONFIG_FILE"
}

# ============================================================================
# Firewall Functions (Server Only)
# ============================================================================
setup_iptables() {
    local port="$1"

    if ! command -v iptables &>/dev/null; then
        log_warn "iptables not found - skipping firewall configuration"
        log_warn "You must manually configure your firewall!"
        return 0
    fi

    log_step "Configuring iptables rules..."

    # Check if rules already exist
    if iptables -t raw -C PREROUTING -p tcp --dport "$port" -j NOTRACK 2>/dev/null; then
        log_info "iptables rules already configured"
        return 0
    fi

    # Add NOTRACK rules
    iptables -t raw -A PREROUTING -p tcp --dport "$port" -j NOTRACK || {
        log_warn "Failed to add PREROUTING NOTRACK rule"
    }

    iptables -t raw -A OUTPUT -p tcp --sport "$port" -j NOTRACK || {
        log_warn "Failed to add OUTPUT NOTRACK rule"
    }

    # Drop RST packets
    iptables -t mangle -A OUTPUT -p tcp --sport "$port" --tcp-flags RST RST -j DROP || {
        log_warn "Failed to add RST DROP rule"
    }

    log_success "iptables rules configured"

    # Try to persist rules
    persist_iptables_rules
}

persist_iptables_rules() {
    log_step "Attempting to persist iptables rules..."

    # Try netfilter-persistent (Debian/Ubuntu)
    if command -v netfilter-persistent &>/dev/null; then
        netfilter-persistent save 2>/dev/null && {
            log_success "Rules saved with netfilter-persistent"
            return 0
        }
    fi

    # Try iptables-save (generic)
    if command -v iptables-save &>/dev/null; then
        mkdir -p /etc/iptables
        iptables-save > /etc/iptables/rules.v4 2>/dev/null && {
            log_success "Rules saved to /etc/iptables/rules.v4"
            log_warn "You may need to configure your system to restore rules on boot"
            return 0
        }
    fi

    log_warn "Could not persist iptables rules automatically"
    log_warn "Rules will be lost on reboot unless you save them manually"
}

remove_iptables_rules() {
    local port="$1"

    if ! command -v iptables &>/dev/null; then
        return 0
    fi

    log_step "Removing iptables rules for port $port..."

    iptables -t raw -D PREROUTING -p tcp --dport "$port" -j NOTRACK 2>/dev/null || true
    iptables -t raw -D OUTPUT -p tcp --sport "$port" -j NOTRACK 2>/dev/null || true
    iptables -t mangle -D OUTPUT -p tcp --sport "$port" --tcp-flags RST RST -j DROP 2>/dev/null || true

    log_success "iptables rules removed"
}

# ============================================================================
# systemd Functions
# ============================================================================
create_systemd_service() {
    local role="$1"

    log_step "Creating systemd service..."

    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Paqet Packet-Level Proxy (${role})
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BINARY_PATH} run -c ${CONFIG_FILE}
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=paqet

[Install]
WantedBy=multi-user.target
EOF

    chmod 644 "$SERVICE_FILE"

    # Reload systemd
    systemctl daemon-reload

    log_success "systemd service created"
}

enable_service() {
    log_step "Enabling paqet service..."
    systemctl enable paqet 2>/dev/null
    log_success "Service enabled (will start on boot)"
}

start_service() {
    log_step "Starting paqet service..."

    if systemctl start paqet; then
        sleep 2
        if systemctl is-active --quiet paqet; then
            log_success "Service started successfully"
            return 0
        fi
    fi

    log_error "Service failed to start"
    log_info "Check logs with: journalctl -u paqet -n 50"
    return 1
}

stop_service() {
    if systemctl is-active --quiet paqet 2>/dev/null; then
        log_step "Stopping paqet service..."
        systemctl stop paqet
        log_success "Service stopped"
    fi
}

get_service_status() {
    if systemctl is-active --quiet paqet 2>/dev/null; then
        echo -e "${GREEN}● active (running)${NC}"
    elif systemctl is-enabled --quiet paqet 2>/dev/null; then
        echo -e "${YELLOW}○ inactive (enabled)${NC}"
    else
        echo -e "${RED}○ inactive${NC}"
    fi
}

# ============================================================================
# Uninstall Function
# ============================================================================
uninstall_paqet() {
    print_header "UNINSTALLING PAQET"

    local role
    role=$(get_installed_role)

    # Get port from config for iptables cleanup
    local port=""
    if [[ -f "$CONFIG_FILE" ]]; then
        port=$(grep -oP 'addr:\s*":\K\d+' "$CONFIG_FILE" 2>/dev/null | head -1 || true)
    fi
    port="${port:-$DEFAULT_PORT}"

    if [[ "$ARG_YES" != true ]]; then
        if ! confirm "Are you sure you want to uninstall paqet?" "n"; then
            log_info "Uninstall cancelled"
            return 0
        fi
    fi

    echo

    # Stop and disable service
    stop_service
    if systemctl is-enabled --quiet paqet 2>/dev/null; then
        log_step "Disabling service..."
        systemctl disable paqet 2>/dev/null
        log_success "Service disabled"
    fi

    # Remove iptables rules (server only)
    if [[ "$role" == "server" ]] && [[ -n "$port" ]]; then
        remove_iptables_rules "$port"
    fi

    # Remove files
    log_step "Removing files..."

    rm -f "$BINARY_PATH"
    rm -rf "$CONFIG_DIR"
    rm -f "$SERVICE_FILE"

    # Reload systemd
    systemctl daemon-reload

    log_success "Files removed"

    print_section "UNINSTALL COMPLETE"
    echo
    echo -e "  Paqet has been completely removed from this system."
    echo
}

# ============================================================================
# UI Functions
# ============================================================================
show_banner() {
    echo
    echo -e "${CYAN}${BOLD}"
    echo "  ██████╗  █████╗  ██████╗ ███████╗████████╗"
    echo "  ██╔══██╗██╔══██╗██╔═══██╗██╔════╝╚══██╔══╝"
    echo "  ██████╔╝███████║██║   ██║█████╗     ██║   "
    echo "  ██╔═══╝ ██╔══██║██║▄▄ ██║██╔══╝     ██║   "
    echo "  ██║     ██║  ██║╚██████╔╝███████╗   ██║   "
    echo "  ╚═╝     ╚═╝  ╚═╝ ╚══▀▀═╝ ╚══════╝   ╚═╝   "
    echo -e "${NC}"
    echo -e "  ${DIM}Packet-Level Proxy Installer v${SCRIPT_VERSION}${NC}"
    echo
}

show_main_menu() {
    echo
    echo -e "  ${BOLD}Select installation type:${NC}"
    echo
    echo -e "    ${CYAN}1)${NC} Install Server"
    echo -e "    ${CYAN}2)${NC} Install Client"
    echo -e "    ${CYAN}3)${NC} Exit"
    echo

    local choice
    read -rp "$(echo -e "  ${CYAN}?${NC} Enter choice [1-3]: ")" choice

    case "$choice" in
        1) install_server ;;
        2) install_client ;;
        3) exit 0 ;;
        *) log_error "Invalid choice"; show_main_menu ;;
    esac
}

show_installed_menu() {
    local role
    role=$(get_installed_role)
    local version
    version=$(get_installed_version)
    local status
    status=$(get_service_status)

    echo
    echo -e "  ${BOLD}Paqet is already installed${NC}"
    echo
    echo -e "    Role:    ${MAGENTA}${role}${NC}"
    echo -e "    Version: ${version}"
    echo -e "    Status:  ${status}"
    echo
    echo -e "  ${BOLD}What would you like to do?${NC}"
    echo
    echo -e "    ${CYAN}1)${NC} Update to latest version"
    echo -e "    ${CYAN}2)${NC} Uninstall"
    echo -e "    ${CYAN}3)${NC} Show status & logs"
    echo -e "    ${CYAN}4)${NC} Restart service"
    echo -e "    ${CYAN}5)${NC} Exit"
    echo

    local choice
    read -rp "$(echo -e "  ${CYAN}?${NC} Enter choice [1-5]: ")" choice

    case "$choice" in
        1) update_paqet ;;
        2) uninstall_paqet ;;
        3) show_status ;;
        4) restart_service ;;
        5) exit 0 ;;
        *) log_error "Invalid choice"; show_installed_menu ;;
    esac
}

show_status() {
    print_header "PAQET STATUS"

    echo
    systemctl status paqet --no-pager 2>/dev/null || true
    echo

    print_section "RECENT LOGS"
    echo
    journalctl -u paqet -n 20 --no-pager 2>/dev/null || echo "  No logs available"
    echo
}

restart_service() {
    log_step "Restarting paqet service..."
    systemctl restart paqet
    sleep 2
    if systemctl is-active --quiet paqet; then
        log_success "Service restarted successfully"
    else
        log_error "Service failed to restart"
        log_info "Check logs with: journalctl -u paqet -n 50"
    fi
}

update_paqet() {
    print_header "UPDATING PAQET"

    local current_version
    current_version=$(get_installed_version)

    get_latest_version || exit 1

    if [[ "$current_version" == *"$LATEST_VERSION"* ]]; then
        log_info "Already running the latest version ($LATEST_VERSION)"
        return 0
    fi

    local arch
    arch=$(detect_arch)

    local temp_file
    temp_file=$(download_binary "$arch" "$LATEST_VERSION") || exit 1

    stop_service
    install_binary "$temp_file"
    start_service

    print_section "UPDATE COMPLETE"
    echo
    echo -e "  Updated from ${current_version} to ${LATEST_VERSION}"
    echo
}

# ============================================================================
# Server Installation
# ============================================================================
prompt_server_config() {
    print_section "SERVER CONFIGURATION"

    # Interface
    if [[ -n "$DETECTED_IFACE" ]]; then
        prompt SERVER_INTERFACE "Network interface" "$DETECTED_IFACE"
    else
        echo
        echo "  Available interfaces:"
        ip -o link show | awk -F': ' '{print "    " $2}' | grep -v "^    lo$"
        echo
        prompt SERVER_INTERFACE "Network interface" ""
    fi

    # Validate interface
    if ! ip link show "$SERVER_INTERFACE" &>/dev/null; then
        log_error "Interface '$SERVER_INTERFACE' does not exist"
        exit 1
    fi

    # Get IP for selected interface
    SERVER_IP=$(ip -4 addr show "$SERVER_INTERFACE" 2>/dev/null | awk '/inet / {print $2}' | cut -d/ -f1 | head -1)
    if [[ -z "$SERVER_IP" ]]; then
        prompt SERVER_IP "Server IP address" ""
    else
        prompt SERVER_IP "Server IP address" "$SERVER_IP"
    fi

    # Gateway MAC
    if [[ -n "$DETECTED_GATEWAY_MAC" ]]; then
        prompt SERVER_GATEWAY_MAC "Gateway MAC address" "$DETECTED_GATEWAY_MAC"
    else
        echo
        log_warn "Could not auto-detect gateway MAC address"
        echo "  Find it with: arp -n \$(ip route | awk '/default/ {print \$3}')"
        echo
        prompt SERVER_GATEWAY_MAC "Gateway MAC address" ""
    fi

    # Port
    prompt SERVER_PORT "Listen port" "$DEFAULT_PORT"

    # Encryption
    echo
    echo "  Encryption options: aes, aes-128, aes-192, salsa20, blowfish, twofish, tea, xtea, none"
    prompt SERVER_ENCRYPTION "Encryption type" "$DEFAULT_ENCRYPTION"

    # KCP Mode
    echo
    echo "  KCP modes: normal, fast, fast2, fast3"
    prompt SERVER_KCP_MODE "KCP mode" "$DEFAULT_KCP_MODE"

    # Log level
    prompt SERVER_LOG_LEVEL "Log level (debug/info/warn/error)" "$DEFAULT_LOG_LEVEL"
}

install_server() {
    print_header "SERVER INSTALLATION"

    detect_network
    get_latest_version || exit 1

    local arch
    arch=$(detect_arch)
    log_info "Architecture: $arch"

    prompt_server_config

    echo
    if ! confirm "Proceed with installation?" "y"; then
        log_info "Installation cancelled"
        exit 0
    fi

    echo

    # Download and install binary
    local temp_file
    temp_file=$(download_binary "$arch" "$LATEST_VERSION") || exit 1
    install_binary "$temp_file"

    # Generate secret key
    generate_secret

    # Create configuration
    create_server_config \
        "$SERVER_INTERFACE" \
        "$SERVER_IP" \
        "$SERVER_PORT" \
        "$SERVER_GATEWAY_MAC" \
        "$SERVER_ENCRYPTION" \
        "$SERVER_KCP_MODE" \
        "$SERVER_LOG_LEVEL" \
        "$SECRET_KEY"

    # Setup iptables
    setup_iptables "$SERVER_PORT"

    # Create and start service
    create_systemd_service "server"
    enable_service
    start_service

    # Show summary
    show_server_summary
}

show_server_summary() {
    local status
    status=$(get_service_status)

    print_header "PAQET SERVER INSTALLED"

    echo
    echo -e "  Status:      ${status}"
    echo -e "  Version:     ${LATEST_VERSION}"
    echo -e "  Listening:   ${SERVER_IP}:${SERVER_PORT}"
    echo -e "  Interface:   ${SERVER_INTERFACE}"
    echo

    print_section "SECRET KEY"

    echo
    echo -e "  ${YELLOW}${BOLD}${SECRET_KEY}${NC}"
    echo
    echo -e "  ${RED}Save this key! You need it for client configuration.${NC}"
    echo -e "  ${RED}It will NOT be shown again.${NC}"
    echo

    print_section "CLIENT INSTALLATION"

    echo
    echo "  On your client machine, run this script and select 'Install Client'."
    echo
    echo "  You will need to provide:"
    echo -e "    Server Address: ${CYAN}${SERVER_IP}${NC}"
    echo -e "    Server Port:    ${CYAN}${SERVER_PORT}${NC}"
    echo -e "    Secret Key:     ${CYAN}${SECRET_KEY}${NC}"
    echo

    print_section "USEFUL COMMANDS"

    echo
    echo "  Check status:    sudo systemctl status paqet"
    echo "  View logs:       sudo journalctl -u paqet -f"
    echo "  Restart:         sudo systemctl restart paqet"
    echo "  Edit config:     sudo nano ${CONFIG_FILE}"
    echo

    print_line 70 "═"
    echo
}

# ============================================================================
# Client Installation
# ============================================================================
prompt_client_config() {
    print_section "CLIENT CONFIGURATION"

    # Server details
    prompt CLIENT_SERVER_ADDR "Server IP address" ""
    prompt CLIENT_SERVER_PORT "Server port" "$DEFAULT_PORT"
    prompt CLIENT_SECRET_KEY "Secret key (from server)" ""

    # Validate key format
    if [[ ${#CLIENT_SECRET_KEY} -ne 64 ]] || ! [[ "$CLIENT_SECRET_KEY" =~ ^[a-fA-F0-9]+$ ]]; then
        log_error "Invalid secret key format (expected 64 hex characters)"
        exit 1
    fi

    # Proxy mode
    echo
    echo "  Proxy modes:"
    echo "    1) SOCKS5 - Universal proxy (recommended)"
    echo "    2) TCP Forward - Direct port forwarding"
    echo
    local mode_choice
    read -rp "$(echo -e "  ${CYAN}?${NC} Select proxy mode [1]: ")" mode_choice
    mode_choice="${mode_choice:-1}"

    case "$mode_choice" in
        1)
            CLIENT_MODE="socks5"
            prompt CLIENT_LOCAL_PORT "Local SOCKS5 port" "$DEFAULT_LOCAL_PORT"
            ;;
        2)
            CLIENT_MODE="forward"
            prompt CLIENT_LOCAL_PORT "Local listen port" "$DEFAULT_LOCAL_PORT"
            prompt CLIENT_FORWARD_TARGET "Forward target (host:port)" ""
            ;;
        *)
            CLIENT_MODE="socks5"
            prompt CLIENT_LOCAL_PORT "Local SOCKS5 port" "$DEFAULT_LOCAL_PORT"
            ;;
    esac

    # Interface
    if [[ -n "$DETECTED_IFACE" ]]; then
        prompt CLIENT_INTERFACE "Network interface" "$DETECTED_IFACE"
    else
        echo
        echo "  Available interfaces:"
        ip -o link show | awk -F': ' '{print "    " $2}' | grep -v "^    lo$"
        echo
        prompt CLIENT_INTERFACE "Network interface" ""
    fi

    # Validate interface
    if ! ip link show "$CLIENT_INTERFACE" &>/dev/null; then
        log_error "Interface '$CLIENT_INTERFACE' does not exist"
        exit 1
    fi

    # Get IP for selected interface
    CLIENT_IP=$(ip -4 addr show "$CLIENT_INTERFACE" 2>/dev/null | awk '/inet / {print $2}' | cut -d/ -f1 | head -1)
    if [[ -z "$CLIENT_IP" ]]; then
        prompt CLIENT_IP "Client IP address" ""
    fi

    # Gateway MAC
    if [[ -n "$DETECTED_GATEWAY_MAC" ]]; then
        prompt CLIENT_GATEWAY_MAC "Gateway MAC address" "$DETECTED_GATEWAY_MAC"
    else
        echo
        log_warn "Could not auto-detect gateway MAC address"
        echo "  Find it with: arp -n \$(ip route | awk '/default/ {print \$3}')"
        echo
        prompt CLIENT_GATEWAY_MAC "Gateway MAC address" ""
    fi

    # Encryption
    echo
    echo "  Encryption options: aes, aes-128, aes-192, salsa20, blowfish, twofish, tea, xtea, none"
    prompt CLIENT_ENCRYPTION "Encryption type" "$DEFAULT_ENCRYPTION"

    # KCP Mode
    echo
    echo "  KCP modes: normal, fast, fast2, fast3"
    prompt CLIENT_KCP_MODE "KCP mode" "$DEFAULT_KCP_MODE"

    # Log level
    prompt CLIENT_LOG_LEVEL "Log level (debug/info/warn/error)" "$DEFAULT_LOG_LEVEL"
}

install_client() {
    print_header "CLIENT INSTALLATION"

    detect_network
    get_latest_version || exit 1

    local arch
    arch=$(detect_arch)
    log_info "Architecture: $arch"

    prompt_client_config

    echo
    if ! confirm "Proceed with installation?" "y"; then
        log_info "Installation cancelled"
        exit 0
    fi

    echo

    # Download and install binary
    local temp_file
    temp_file=$(download_binary "$arch" "$LATEST_VERSION") || exit 1
    install_binary "$temp_file"

    # Create configuration
    if [[ "$CLIENT_MODE" == "socks5" ]]; then
        create_client_config_socks \
            "$CLIENT_INTERFACE" \
            "$CLIENT_IP" \
            "$CLIENT_GATEWAY_MAC" \
            "$CLIENT_SERVER_ADDR" \
            "$CLIENT_SERVER_PORT" \
            "$CLIENT_LOCAL_PORT" \
            "$CLIENT_ENCRYPTION" \
            "$CLIENT_KCP_MODE" \
            "$CLIENT_LOG_LEVEL" \
            "$CLIENT_SECRET_KEY"
    else
        create_client_config_forward \
            "$CLIENT_INTERFACE" \
            "$CLIENT_IP" \
            "$CLIENT_GATEWAY_MAC" \
            "$CLIENT_SERVER_ADDR" \
            "$CLIENT_SERVER_PORT" \
            "$CLIENT_LOCAL_PORT" \
            "$CLIENT_FORWARD_TARGET" \
            "$CLIENT_ENCRYPTION" \
            "$CLIENT_KCP_MODE" \
            "$CLIENT_LOG_LEVEL" \
            "$CLIENT_SECRET_KEY"
    fi

    # Create and start service
    create_systemd_service "client"
    enable_service
    start_service

    # Show summary
    show_client_summary
}

show_client_summary() {
    local status
    status=$(get_service_status)

    print_header "PAQET CLIENT INSTALLED"

    echo
    echo -e "  Status:        ${status}"
    echo -e "  Version:       ${LATEST_VERSION}"

    if [[ "$CLIENT_MODE" == "socks5" ]]; then
        echo -e "  Mode:          SOCKS5 Proxy"
        echo -e "  Local Proxy:   127.0.0.1:${CLIENT_LOCAL_PORT}"
    else
        echo -e "  Mode:          TCP Forward"
        echo -e "  Local Port:    127.0.0.1:${CLIENT_LOCAL_PORT}"
        echo -e "  Forward To:    ${CLIENT_FORWARD_TARGET}"
    fi

    echo -e "  Server:        ${CLIENT_SERVER_ADDR}:${CLIENT_SERVER_PORT}"
    echo -e "  Interface:     ${CLIENT_INTERFACE}"
    echo

    print_section "TEST CONNECTION"

    echo
    if [[ "$CLIENT_MODE" == "socks5" ]]; then
        echo "  Test the SOCKS5 proxy:"
        echo
        echo -e "    ${CYAN}curl -x socks5h://127.0.0.1:${CLIENT_LOCAL_PORT} https://httpbin.org/ip${NC}"
        echo
        echo "  Expected: Your server's IP (${CLIENT_SERVER_ADDR})"
    else
        echo "  Test the TCP forward:"
        echo
        echo -e "    ${CYAN}curl http://127.0.0.1:${CLIENT_LOCAL_PORT}${NC}"
        echo
        echo "  This will connect to ${CLIENT_FORWARD_TARGET} through the tunnel"
    fi
    echo

    print_section "USEFUL COMMANDS"

    echo
    echo "  Check status:    sudo systemctl status paqet"
    echo "  View logs:       sudo journalctl -u paqet -f"
    echo "  Restart:         sudo systemctl restart paqet"
    echo "  Edit config:     sudo nano ${CONFIG_FILE}"
    echo

    print_line 70 "═"
    echo
}

# ============================================================================
# Argument Parsing
# ============================================================================
show_help() {
    echo "Paqet Install Script v${SCRIPT_VERSION}"
    echo
    echo "Usage: $0 [OPTIONS]"
    echo
    echo "Options:"
    echo "  --server              Install as server"
    echo "  --client              Install as client"
    echo "  -y, --yes             Non-interactive mode (use defaults)"
    echo "  -p, --port PORT       Server listen port (server) or server port (client)"
    echo "  -s, --server-addr IP  Server address (client only)"
    echo "  -k, --key KEY         Secret key (client only)"
    echo "  --local-port PORT     Local proxy port (client only)"
    echo "  --forward TARGET      Forward target host:port (client only, enables forward mode)"
    echo "  --uninstall           Uninstall paqet"
    echo "  -h, --help            Show this help"
    echo
    echo "Examples:"
    echo "  # Interactive server install"
    echo "  sudo $0"
    echo
    echo "  # Non-interactive server install"
    echo "  sudo $0 --server -y"
    echo
    echo "  # Non-interactive client install (SOCKS5)"
    echo "  sudo $0 --client -y -s 1.2.3.4 -p 9999 -k <secret_key>"
    echo
    echo "  # Non-interactive client install (TCP Forward)"
    echo "  sudo $0 --client -y -s 1.2.3.4 -p 9999 -k <key> --forward example.com:80"
    echo
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --server)
                ARG_MODE="server"
                shift
                ;;
            --client)
                ARG_MODE="client"
                shift
                ;;
            -y|--yes)
                ARG_YES=true
                shift
                ;;
            -p|--port)
                ARG_PORT="$2"
                shift 2
                ;;
            -s|--server-addr)
                ARG_SERVER_ADDR="$2"
                shift 2
                ;;
            -k|--key)
                ARG_SECRET_KEY="$2"
                shift 2
                ;;
            --local-port)
                ARG_LOCAL_PORT="$2"
                shift 2
                ;;
            --forward)
                ARG_FORWARD_TARGET="$2"
                ARG_PROXY_MODE="forward"
                shift 2
                ;;
            --uninstall)
                ARG_MODE="uninstall"
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# ============================================================================
# Non-interactive Installation
# ============================================================================
install_server_noninteractive() {
    print_header "SERVER INSTALLATION (NON-INTERACTIVE)"

    detect_network
    get_latest_version || exit 1

    local arch
    arch=$(detect_arch)
    log_info "Architecture: $arch"

    # Use detected or default values
    SERVER_INTERFACE="${DETECTED_IFACE}"
    SERVER_IP="${DETECTED_IP}"
    SERVER_GATEWAY_MAC="${DETECTED_GATEWAY_MAC}"
    SERVER_PORT="${ARG_PORT:-$DEFAULT_PORT}"
    SERVER_ENCRYPTION="$DEFAULT_ENCRYPTION"
    SERVER_KCP_MODE="$DEFAULT_KCP_MODE"
    SERVER_LOG_LEVEL="$DEFAULT_LOG_LEVEL"

    # Validate required values
    if [[ -z "$SERVER_INTERFACE" ]]; then
        log_error "Could not detect network interface"
        exit 1
    fi
    if [[ -z "$SERVER_IP" ]]; then
        log_error "Could not detect server IP"
        exit 1
    fi
    if [[ -z "$SERVER_GATEWAY_MAC" ]]; then
        log_error "Could not detect gateway MAC address"
        exit 1
    fi

    log_info "Interface: $SERVER_INTERFACE"
    log_info "Server IP: $SERVER_IP"
    log_info "Gateway MAC: $SERVER_GATEWAY_MAC"
    log_info "Port: $SERVER_PORT"

    echo

    # Download and install binary
    local temp_file
    temp_file=$(download_binary "$arch" "$LATEST_VERSION") || exit 1
    install_binary "$temp_file"

    # Generate secret key
    generate_secret

    # Create configuration
    create_server_config \
        "$SERVER_INTERFACE" \
        "$SERVER_IP" \
        "$SERVER_PORT" \
        "$SERVER_GATEWAY_MAC" \
        "$SERVER_ENCRYPTION" \
        "$SERVER_KCP_MODE" \
        "$SERVER_LOG_LEVEL" \
        "$SECRET_KEY"

    # Setup iptables
    setup_iptables "$SERVER_PORT"

    # Create and start service
    create_systemd_service "server"
    enable_service
    start_service

    # Show summary
    show_server_summary
}

install_client_noninteractive() {
    print_header "CLIENT INSTALLATION (NON-INTERACTIVE)"

    # Validate required arguments
    if [[ -z "$ARG_SERVER_ADDR" ]]; then
        log_error "Server address required (use -s or --server-addr)"
        exit 1
    fi
    if [[ -z "$ARG_SECRET_KEY" ]]; then
        log_error "Secret key required (use -k or --key)"
        exit 1
    fi

    detect_network
    get_latest_version || exit 1

    local arch
    arch=$(detect_arch)
    log_info "Architecture: $arch"

    # Use detected or default values
    CLIENT_INTERFACE="${DETECTED_IFACE}"
    CLIENT_IP="${DETECTED_IP}"
    CLIENT_GATEWAY_MAC="${DETECTED_GATEWAY_MAC}"
    CLIENT_SERVER_ADDR="$ARG_SERVER_ADDR"
    CLIENT_SERVER_PORT="${ARG_PORT:-$DEFAULT_PORT}"
    CLIENT_SECRET_KEY="$ARG_SECRET_KEY"
    CLIENT_LOCAL_PORT="${ARG_LOCAL_PORT:-$DEFAULT_LOCAL_PORT}"
    CLIENT_MODE="${ARG_PROXY_MODE:-socks5}"
    CLIENT_FORWARD_TARGET="${ARG_FORWARD_TARGET:-}"
    CLIENT_ENCRYPTION="$DEFAULT_ENCRYPTION"
    CLIENT_KCP_MODE="$DEFAULT_KCP_MODE"
    CLIENT_LOG_LEVEL="$DEFAULT_LOG_LEVEL"

    # Validate required values
    if [[ -z "$CLIENT_INTERFACE" ]]; then
        log_error "Could not detect network interface"
        exit 1
    fi
    if [[ -z "$CLIENT_IP" ]]; then
        log_error "Could not detect client IP"
        exit 1
    fi
    if [[ -z "$CLIENT_GATEWAY_MAC" ]]; then
        log_error "Could not detect gateway MAC address"
        exit 1
    fi

    # Validate key format
    if [[ ${#CLIENT_SECRET_KEY} -ne 64 ]] || ! [[ "$CLIENT_SECRET_KEY" =~ ^[a-fA-F0-9]+$ ]]; then
        log_error "Invalid secret key format (expected 64 hex characters)"
        exit 1
    fi

    # Validate forward target if in forward mode
    if [[ "$CLIENT_MODE" == "forward" ]] && [[ -z "$CLIENT_FORWARD_TARGET" ]]; then
        log_error "Forward target required for forward mode (use --forward)"
        exit 1
    fi

    log_info "Interface: $CLIENT_INTERFACE"
    log_info "Client IP: $CLIENT_IP"
    log_info "Gateway MAC: $CLIENT_GATEWAY_MAC"
    log_info "Server: $CLIENT_SERVER_ADDR:$CLIENT_SERVER_PORT"
    log_info "Mode: $CLIENT_MODE"
    log_info "Local Port: $CLIENT_LOCAL_PORT"

    echo

    # Download and install binary
    local temp_file
    temp_file=$(download_binary "$arch" "$LATEST_VERSION") || exit 1
    install_binary "$temp_file"

    # Create configuration
    if [[ "$CLIENT_MODE" == "socks5" ]]; then
        create_client_config_socks \
            "$CLIENT_INTERFACE" \
            "$CLIENT_IP" \
            "$CLIENT_GATEWAY_MAC" \
            "$CLIENT_SERVER_ADDR" \
            "$CLIENT_SERVER_PORT" \
            "$CLIENT_LOCAL_PORT" \
            "$CLIENT_ENCRYPTION" \
            "$CLIENT_KCP_MODE" \
            "$CLIENT_LOG_LEVEL" \
            "$CLIENT_SECRET_KEY"
    else
        create_client_config_forward \
            "$CLIENT_INTERFACE" \
            "$CLIENT_IP" \
            "$CLIENT_GATEWAY_MAC" \
            "$CLIENT_SERVER_ADDR" \
            "$CLIENT_SERVER_PORT" \
            "$CLIENT_LOCAL_PORT" \
            "$CLIENT_FORWARD_TARGET" \
            "$CLIENT_ENCRYPTION" \
            "$CLIENT_KCP_MODE" \
            "$CLIENT_LOG_LEVEL" \
            "$CLIENT_SECRET_KEY"
    fi

    # Create and start service
    create_systemd_service "client"
    enable_service
    start_service

    # Show summary
    show_client_summary
}

# ============================================================================
# Main
# ============================================================================
main() {
    parse_args "$@"

    show_banner
    run_preflight_checks

    # Handle uninstall mode (works with -y for non-interactive)
    if [[ "$ARG_MODE" == "uninstall" ]]; then
        if is_installed; then
            uninstall_paqet
            exit 0
        else
            log_error "Paqet is not installed"
            exit 1
        fi
    fi

    # Handle non-interactive mode
    if [[ "$ARG_YES" == true ]]; then
        if [[ "$ARG_MODE" == "server" ]]; then
            install_server_noninteractive
        elif [[ "$ARG_MODE" == "client" ]]; then
            install_client_noninteractive
        else
            log_error "Mode required for non-interactive install (use --server or --client)"
            exit 1
        fi
        exit 0
    fi

    # Interactive mode
    if is_installed; then
        show_installed_menu
    else
        if [[ "$ARG_MODE" == "server" ]]; then
            install_server
        elif [[ "$ARG_MODE" == "client" ]]; then
            install_client
        else
            show_main_menu
        fi
    fi
}

# Run main function
main "$@"
