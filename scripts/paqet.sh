set -euo pipefail

cmd="${1:-}"
arg2="${2:-}"
arg3="${3:-}"
arg4="${4:-}"

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin_path="${root_dir}/paqet"
logs_dir="${root_dir}/logs"
client_cfg="${root_dir}/client.yaml"
server_cfg="${root_dir}/server.yaml"

usage() {
  echo "Usage:"
  echo "  bash scripts/paqet.sh build"
  echo "  bash scripts/paqet.sh setup-client [--force]"
  echo "  bash scripts/paqet.sh setup-server [--force]"
  echo "  bash scripts/paqet.sh run-client"
  echo "  bash scripts/paqet.sh run-server"
  echo "  bash scripts/paqet.sh supervise-client"
  echo "  bash scripts/paqet.sh supervise-server"
  echo "  bash scripts/paqet.sh monitor-client"
  echo "  bash scripts/paqet.sh monitor-server"
  echo "  bash scripts/paqet.sh monitor-packets <iface> <port>"
  echo "  bash scripts/paqet.sh ping [payload]"
}

ensure_bin() {
  if [ ! -x "$bin_path" ]; then
    (cd "$root_dir" && go build -o "$bin_path" ./cmd)
  fi
}

case "$cmd" in
  build)
    (cd "$root_dir" && go build -o "$bin_path" ./cmd)
    ;;
  setup-client)
    ensure_bin
    if [ -f "$client_cfg" ] && [ "$arg2" != "--force" ]; then
      echo "client.yaml already exists"
      exit 0
    fi
    cp "$root_dir/example/client.yaml.example" "$client_cfg"
    echo "client.yaml created"
    "$bin_path" iface || true
    ;;
  setup-server)
    ensure_bin
    if [ -f "$server_cfg" ] && [ "$arg2" != "--force" ]; then
      echo "server.yaml already exists"
      exit 0
    fi
    cp "$root_dir/example/server.yaml.example" "$server_cfg"
    echo "server.yaml created"
    "$bin_path" iface || true
    ;;
  run-client)
    ensure_bin
    mkdir -p "$logs_dir"
    sudo "$bin_path" run -c "$client_cfg" 2>&1 | tee -a "$logs_dir/client.log"
    ;;
  run-server)
    ensure_bin
    mkdir -p "$logs_dir"
    sudo "$bin_path" run -c "$server_cfg" 2>&1 | tee -a "$logs_dir/server.log"
    ;;
  supervise-client)
    ensure_bin
    mkdir -p "$logs_dir"
    while true; do
      sudo "$bin_path" run -c "$client_cfg" 2>&1 | tee -a "$logs_dir/client.log"
      echo "client exited, restarting in 3s"
      sleep 3
    done
    ;;
  supervise-server)
    ensure_bin
    mkdir -p "$logs_dir"
    while true; do
      sudo "$bin_path" run -c "$server_cfg" 2>&1 | tee -a "$logs_dir/server.log"
      echo "server exited, restarting in 3s"
      sleep 3
    done
    ;;
  monitor-client)
    mkdir -p "$logs_dir"
    tail -f "$logs_dir/client.log"
    ;;
  monitor-server)
    mkdir -p "$logs_dir"
    tail -f "$logs_dir/server.log"
    ;;
  monitor-packets)
    if [ -z "$arg2" ] || [ -z "$arg3" ]; then
      echo "monitor-packets requires interface and port"
      exit 1
    fi
    ensure_bin
    sudo "$bin_path" dump -i "$arg2" -p "$arg3"
    ;;
  ping)
    ensure_bin
    payload="${arg2:-PING}"
    sudo "$bin_path" ping -c "$client_cfg" --payload "$payload"
    ;;
  *)
    usage
    exit 1
    ;;
esac
