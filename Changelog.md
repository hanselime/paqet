## Key Changes
1.  **SeqTracker:** Intelligent TCP sequence tracking to prevent packet drops by firewalls.
2.  **Port Pool:** Distributes traffic across thousands of ephemeral ports to mimic natural browser behavior.
3.  **Rate Limiter:** Prevents burst traffic and DDoS detection triggers.

## Installation & Usage

**Prerequisites:** Go 1.25+ and libpcap (Linux).

```bash
go mod tidy
go build -o paqet.exe ./cmd/main.go
```

## Quick Configuration (`config.yaml`)

Add these settings to the `network` section:

```yaml
network:
  interface: "eth0"
  
  # TCP State Management (Critical for Server)
  tcp_state:
    enabled: true
    connection_timeout: 5m
    cleanup_interval: 60s

  # Port Pool (Critical for Client)
  port_pool:
    enabled: true
    start_port: 50000
    end_port: 51000

  # Rate Limiting (Prevents IP bans)
  rate_limit:
    enabled: true
    packets_per_second: 2000
    burst: 200
```

## Network Health Check
To ensure your datacenter isn't blocking you, run the following diagnostic script:

```bash
chmod +x scripts/diagnose_network_impact.sh
sudo ./scripts/diagnose_network_impact.sh 9999 60
```
If the output is green (**EXCELLENT**), your traffic is healthy.
