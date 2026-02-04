#!/usr/bin/env bash
set -euo pipefail

# --- Colors & Styling ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# --- Logging Helpers ---
log_header() { echo -e "\n${BLUE}${BOLD}=== $1 ===${NC}"; }
log_step() { echo -e "\n${BLUE}${BOLD}>> $1${NC}"; }
log_info() { echo -e "${CYAN}ℹ $1${NC}"; }
log_success() { echo -e "${GREEN}✔ $1${NC}"; }
log_warn() { echo -e "${YELLOW}⚠ $1${NC}"; }
log_error() { echo -e "${RED}✖ $1${NC}"; }

# Check for root privileges
if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
   log_error "This script must be run as root (use sudo) to configure iptables rules."
   exit 1
fi



# --- Helper Functions ---

prompt() {
  local label="$1"
  local default="${2:-}"
  local value=""
  
  if [[ -n "$default" ]]; then
    # Print to stderr to ensure visibility
    >&2 echo -ne "${BOLD}$label${NC} [${YELLOW}$default${NC}]: "
    read -r value
    value="${value:-$default}"
  else
    >&2 echo -ne "${BOLD}$label${NC}: "
    read -r value
  fi
  printf '%s' "$value"
}

validate_port() {
  local port="$1"
  [[ "$port" =~ ^[0-9]+$ ]] && ((port >= 1 && port <= 65535))
}

check_port_free() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    if ss -lnt | grep -q ":$port "; then
      return 1
    fi
  elif command -v netstat >/dev/null 2>&1; then
    if netstat -lnt | grep -q ":$port "; then
      return 1
    fi
  fi
  return 0
}

validate_ipv4() {
  local ip="$1"
  local IFS='.'
  local -a octets=($ip)
  [[ ${#octets[@]} -eq 4 ]] || return 1
  local o
  for o in "${octets[@]}"; do
    [[ "$o" =~ ^[0-9]+$ ]] || return 1
    ((o >= 0 && o <= 255)) || return 1
  done
  return 0
}

validate_mac() {
  local mac="$1"
  [[ "$mac" =~ ^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$ ]]
}

detect_ipv4() {
  local iface="$1"
  ip -4 -o addr show dev "$iface" 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -n1
}

detect_gateway_ip() {
  local iface="$1"
  ip route show default dev "$iface" 2>/dev/null | awk '/default/ {print $3; exit}'
}

detect_gateway_mac() {
  local iface="$1"
  local gw="$2"
  [[ -z "$gw" ]] && return 1
  
  # Try ip neigh first
  local mac
  mac=$(ip neigh show "$gw" dev "$iface" 2>/dev/null | awk '{print $5; exit}')
  if [[ -n "$mac" && "$mac" != "FAILED" && "$mac" != "INCOMPLETE" ]]; then
      echo "$mac"
      return 0
  fi
  
  # Try arp
  if command -v arp >/dev/null 2>&1; then
    # arp -n output: Address HWtype HWaddress Flags Mask Iface
    # strict matching on IP to avoid header "Address"
    mac=$(arp -n "$gw" 2>/dev/null | grep -F "$gw" | awk '{print $3; exit}')
    if [[ -n "$mac" && "$mac" != "(incomplete)" ]]; then
       echo "$mac"
       return 0
    fi
  fi
  return 1
}

generate_key() {
  # 128-bit key = 16 bytes. Hex representation is 32 characters.
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 16
    return
  fi
  if command -v xxd >/dev/null 2>&1; then
    head -c 16 /dev/urandom | xxd -p
    return
  fi
  head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n'
}

prompt_port() {
  local value
  while true; do
    value="$(prompt "$1" "$2")"
    if validate_port "$value"; then
      if check_port_free "$value"; then
        printf '%s' "$value"
        return 0
      else
         >&2 log_warn "Port $value is already in use. Please select another port."
      fi
    else
      >&2 log_warn "Invalid port. Enter a number between 1 and 65535."
    fi
  done
}

list_ifaces() {
  if ! command -v ip >/dev/null 2>&1; then
    log_error "'ip' command not found; this script expects Linux with iproute2 installed"
    exit 1
  fi
  ip -o link show | awk -F': ' '{print $2}' | cut -d@ -f1 | grep -vE '^lo'
}

select_iface() {
  local -a ifaces=("$@")
  >&2 echo "Available network interfaces:"
  local i iface ip
  for i in "${!ifaces[@]}"; do
    iface="${ifaces[$i]}"
    ip="$(detect_ipv4 "$iface" || true)"
    if [[ -n "$ip" ]]; then
      >&2 printf "  [${BOLD}%d${NC}] ${GREEN}%s${NC} (IP: %s)\n" "$((i + 1))" "$iface" "$ip"
    else
      >&2 printf "  [${BOLD}%d${NC}] %s\n" "$((i + 1))" "$iface"
    fi
  done

  local choice
  while true; do
    read -r -p "Select interface to use [1]: " choice
    choice="${choice:-1}"
    if [[ "$choice" =~ ^[0-9]+$ ]] && ((choice >= 1 && choice <= ${#ifaces[@]})); then
      printf '%s' "${ifaces[$((choice - 1))]}"
      return 0
    fi
    >&2 log_warn "Invalid choice. Please enter a valid number."
  done
}

# --- Main Logic ---

log_header "paqet Server Configuration"

# 1. Ask for listen port
log_step "Step 1: Server Settings"
listen_port="$(prompt_port "Enter server listen port" "9999")"
log_info "Selected Port: ${GREEN}$listen_port${NC}"

# 2. Provide list of interfaces and ask user
log_step "Step 2: Network Interface"
ifaces=()
while IFS= read -r line; do
  [[ -n "$line" ]] && ifaces+=("$line")
done < <(list_ifaces)

if [[ ${#ifaces[@]} -eq 0 ]]; then
  log_error "No network interfaces found."
  exit 1
fi

selected_iface="$(select_iface "${ifaces[@]}")"
log_info "Selected Interface: ${GREEN}$selected_iface${NC}"

# 3. Extract IP address and router MAC
log_step "Step 3: Network Discovery"
log_info "Detecting network details for ${BOLD}$selected_iface${NC}..."

iface_ip="$(detect_ipv4 "$selected_iface")"
if [[ -z "$iface_ip" ]]; then
    log_warn "Could not detect IP address for $selected_iface."
    iface_ip="$(prompt "Enter IP address for $selected_iface" "")"
    while ! validate_ipv4 "$iface_ip"; do
        log_warn "Invalid IP."
        iface_ip="$(prompt "Enter IP address for $selected_iface" "")"
    done
else
    log_success "Detected IP: $iface_ip"
fi

# Detect Gateway/Router MAC
gateway_ip="$(detect_gateway_ip "$selected_iface")"
# Attempt to ping gateway to populate ARP table
if [[ -n "$gateway_ip" ]] && command -v ping >/dev/null 2>&1; then
    ping -c 1 -W 1 "$gateway_ip" >/dev/null 2>&1 || true
fi

router_mac="$(detect_gateway_mac "$selected_iface" "$gateway_ip" || true)"

if [[ -n "$router_mac" ]]; then
    log_success "Detected Router MAC: $router_mac"
else
    log_warn "Could not detect Router MAC address for gateway ${gateway_ip:-unknown}."
    router_mac="$(prompt "Enter Router MAC address" "")"
    while ! validate_mac "$router_mac"; do
         log_warn "Invalid MAC."
         router_mac="$(prompt "Enter Router MAC address" "")"
    done
fi

# 4. Ask for key
log_step "Step 4: Encryption"
auto_key="$(generate_key)"
kcp_key="$(prompt "Enter KCP key (128-bit)" "$auto_key")"
log_info "Key set."

# Output Configuration
output_file="$(prompt "Output configuration file" "server.config.yaml")"
cat > "$output_file" <<EOF
role: "server"
log:
  level: "info"
listen:
  addr: ":$listen_port"
network:
  interface: "$selected_iface"
  ipv4:
    addr: "$iface_ip:$listen_port"
    router_mac: "$router_mac"
  tcp:
    local_flag: ["PA"]
transport:
  protocol: "kcp"
  conn: 1
  kcp:
    mode: "fast"
    key: "$kcp_key"
EOF

log_step "Step 5: Finalizing"
log_success "Configuration saved to ${BOLD}$output_file${NC}"

# Apply iptables rules
log_header "Applying Firewall Rules"
log_info "These rules are required to bypass kernel connection tracking."

# Define rules
rule1="iptables -t raw -A PREROUTING -p tcp --dport $listen_port -j NOTRACK"
rule2="iptables -t raw -A OUTPUT -p tcp --sport $listen_port -j NOTRACK"
rule3="iptables -t mangle -A OUTPUT -p tcp --sport $listen_port --tcp-flags RST RST -j DROP"

# Function to run rule
apply_rule() {
    local cmd="$1"
    local desc="$2"
    log_info "Executing: $cmd"
    if $cmd; then
        log_success "Applied: $desc"
    else
        log_error "Failed to apply: $desc"
    fi
}

apply_rule "$rule1" "Ignore incoming tracking on port $listen_port"
apply_rule "$rule2" "Ignore outgoing tracking on port $listen_port"
apply_rule "$rule3" "Drop kernel RST packets on port $listen_port"

log_header "Setup Complete!"
log_info "You can now run the server with:"
echo -e "${BOLD}sudo ./paqet run -c $output_file${NC}"
