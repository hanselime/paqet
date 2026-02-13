#!/usr/bin/env bash
# paqet Network Impact Diagnostic Tool

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT="${1:-9999}"
DURATION="${2:-60}"

COLOR_RED='\033[0;31m'
COLOR_GREEN='\033[0;32m'
COLOR_YELLOW='\033[1;33m'
COLOR_BLUE='\033[0;34m'
COLOR_RESET='\033[0m'

echo -e "${COLOR_BLUE}=== paqet Network Impact Diagnostic ===${COLOR_RESET}"
echo "Port: $PORT"
echo "Duration: ${DURATION}s"
echo ""

if [ "$EUID" -ne 0 ]; then
    echo -e "${COLOR_RED}ERROR: This script must be run as root${COLOR_RESET}"
    exit 1
fi

echo -e "${COLOR_BLUE}[1/6] Checking dependencies...${COLOR_RESET}"
for cmd in conntrack iptables ss netstat; do
    if ! command -v $cmd &> /dev/null; then
        echo -e "${COLOR_YELLOW}WARNING: $cmd not found (some checks will be skipped)${COLOR_RESET}"
    else
        echo -e "  ✓ $cmd found"
    fi
done
echo ""

echo -e "${COLOR_BLUE}[2/6] Taking baseline measurements...${COLOR_RESET}"

if command -v conntrack &> /dev/null; then
    BASELINE_CONNTRACK=$(conntrack -L 2>/dev/null | wc -l)
    BASELINE_PORT_TRACK=$(conntrack -L 2>/dev/null | grep -c "dport=$PORT" || echo "0")
    echo "  Total connections: $BASELINE_CONNTRACK"
    echo "  Port $PORT connections: $BASELINE_PORT_TRACK"
else
    BASELINE_CONNTRACK="N/A"
    BASELINE_PORT_TRACK="N/A"
fi

BASELINE_IPTABLES_DROPS=$(iptables -L -v -n -x 2>/dev/null | grep -i drop | head -1 | awk '{print $1}' || echo "0")
echo "  IPTables drops: $BASELINE_IPTABLES_DROPS"
echo ""

echo -e "${COLOR_BLUE}[3/6] Monitoring for ${DURATION} seconds...${COLOR_RESET}"
echo "  (Start paqet now if not already running)"
echo ""

TEMP_FILE="/tmp/paqet_diag_$$.txt"
rm -f "$TEMP_FILE"

for i in $(seq 1 $DURATION); do
    if command -v conntrack &> /dev/null; then
        COUNT=$(conntrack -L 2>/dev/null | grep -c "dport=$PORT" || echo "0")
        echo "$i $COUNT" >> "$TEMP_FILE"
    fi
    
    if [ $((i % 10)) -eq 0 ]; then
        echo -e "  ${COLOR_YELLOW}Progress: $i/${DURATION}s${COLOR_RESET}"
    fi
    
    sleep 1
done

echo ""

echo -e "${COLOR_BLUE}[4/6] Analyzing connection patterns...${COLOR_RESET}"

if [ -f "$TEMP_FILE" ]; then
    MAX_CONN=$(awk '{if ($2 > max) max = $2} END {print max}' "$TEMP_FILE")
    AVG_CONN=$(awk '{sum += $2; count++} END {print int(sum/count)}' "$TEMP_FILE")
    
    echo "  Max concurrent connections: $MAX_CONN"
    echo "  Average connections: $AVG_CONN"
    
    if [ "$MAX_CONN" -lt 100 ]; then
        echo -e "  ${COLOR_GREEN}✓ EXCELLENT: Low connection count (< 100)${COLOR_RESET}"
        CONN_SCORE=10
    elif [ "$MAX_CONN" -lt 500 ]; then
        echo -e "  ${COLOR_YELLOW}⚠ MODERATE: Acceptable connection count (< 500)${COLOR_RESET}"
        CONN_SCORE=6
    elif [ "$MAX_CONN" -lt 2000 ]; then
        echo -e "  ${COLOR_YELLOW}⚠ HIGH: Elevated connection count (< 2000)${COLOR_RESET}"
        CONN_SCORE=3
    else
        echo -e "  ${COLOR_RED}✗ CRITICAL: Very high connection count (>= 2000)${COLOR_RESET}"
        echo -e "    ${COLOR_RED}Indicates TCP state tracking may not be working${COLOR_RESET}"
        CONN_SCORE=0
    fi
else
    echo "  (conntrack not available)"
    CONN_SCORE=5
fi
echo ""

echo -e "${COLOR_BLUE}[5/6] Checking port distribution...${COLOR_RESET}"

if command -v ss &> /dev/null; then
    UNIQUE_PORTS=$(ss -tn | grep ":$PORT" | awk '{print $4}' | cut -d: -f2 | sort -u | wc -l)
    echo "  Unique source ports in use: $UNIQUE_PORTS"
    
    if [ "$UNIQUE_PORTS" -gt 100 ]; then
        echo -e "  ${COLOR_GREEN}✓ EXCELLENT: Port pool appears active${COLOR_RESET}"
        PORT_SCORE=10
    elif [ "$UNIQUE_PORTS" -gt 10 ]; then
        echo -e "  ${COLOR_YELLOW}⚠ MODERATE: Some port diversity${COLOR_RESET}"
        PORT_SCORE=6
    else
        echo -e "  ${COLOR_YELLOW}⚠ LOW: Limited port diversity${COLOR_RESET}"
        echo -e "    ${COLOR_YELLOW}Consider enabling port_pool in configuration${COLOR_RESET}"
        PORT_SCORE=3
    fi
else
    echo "  (ss not available)"
    PORT_SCORE=5
fi
echo ""

echo -e "${COLOR_BLUE}[6/6] Checking packet drops...${COLOR_RESET}"

CURRENT_DROPS=$(iptables -L -v -n -x 2>/dev/null | grep -i drop | head -1 | awk '{print $1}' || echo "0")
DROPS_DELTA=$((CURRENT_DROPS - BASELINE_IPTABLES_DROPS))

echo "  IPTables drops during test: $DROPS_DELTA"

if [ "$DROPS_DELTA" -lt 100 ]; then
    echo -e "  ${COLOR_GREEN}✓ EXCELLENT: Minimal packet drops${COLOR_RESET}"
    DROP_SCORE=10
elif [ "$DROPS_DELTA" -lt 1000 ]; then
    echo -e "  ${COLOR_YELLOW}⚠ MODERATE: Some packet drops${COLOR_RESET}"
    DROP_SCORE=6
else
    echo -e "  ${COLOR_RED}✗ HIGH: Significant packet drops${COLOR_RESET}"
    echo -e "    ${COLOR_RED}May indicate rate limiting needed or network congestion${COLOR_RESET}"
    DROP_SCORE=2
fi
echo ""

echo -e "${COLOR_BLUE}=== DIAGNOSTIC REPORT ===${COLOR_RESET}"
echo ""

TOTAL_SCORE=$((CONN_SCORE + PORT_SCORE + DROP_SCORE))
MAX_SCORE=30

echo "  Connection Management: $CONN_SCORE/10"
echo "  Port Distribution:     $PORT_SCORE/10"
echo "  Packet Drops:          $DROP_SCORE/10"
echo "  ─────────────────────────────"
echo "  Total Score:           $TOTAL_SCORE/$MAX_SCORE"
echo ""

if [ "$TOTAL_SCORE" -ge 25 ]; then
    echo -e "${COLOR_GREEN}✓ OVERALL: EXCELLENT${COLOR_RESET}"
    echo "  paqet is operating in a firewall-friendly manner"
    echo "  Minimal impact on datacenter network infrastructure"
elif [ "$TOTAL_SCORE" -ge 18 ]; then
    echo -e "${COLOR_YELLOW}⚠ OVERALL: GOOD${COLOR_RESET}"
    echo "  paqet is mostly firewall-friendly"
    echo "  Consider reviewing recommendations below"
elif [ "$TOTAL_SCORE" -ge 12 ]; then
    echo -e "${COLOR_YELLOW}⚠ OVERALL: NEEDS IMPROVEMENT${COLOR_RESET}"
    echo "  paqet may be causing elevated router load"
    echo "  Review and implement recommendations below"
else
    echo -e "${COLOR_RED}✗ OVERALL: CRITICAL${COLOR_RESET}"
    echo "  paqet is likely causing significant datacenter disruption"
    echo "  URGENT: Implement recommendations below"
fi
echo ""

echo -e "${COLOR_BLUE}=== RECOMMENDATIONS ===${COLOR_RESET}"
echo ""

if [ "$CONN_SCORE" -lt 8 ]; then
    echo -e "${COLOR_YELLOW}→ Enable TCP state tracking:${COLOR_RESET}"
    echo "  network:"
    echo "    tcp_state:"
    echo "      enabled: true"
    echo "      connection_timeout: 5m"
    echo "      cleanup_interval: 60s"
    echo ""
fi

if [ "$PORT_SCORE" -lt 8 ]; then
    echo -e "${COLOR_YELLOW}→ Enable port pooling:${COLOR_RESET}"
    echo "  network:"
    echo "    port_pool:"
    echo "      enabled: true"
    echo "      start_port: 50000"
    echo "      end_port: 51000"
    echo ""
fi

if [ "$DROP_SCORE" -lt 8 ]; then
    echo -e "${COLOR_YELLOW}→ Enable rate limiting:${COLOR_RESET}"
    echo "  network:"
    echo "    rate_limit:"
    echo "      enabled: true"
    echo "      packets_per_second: 2000"
    echo "      burst: 200"
    echo ""
fi

echo -e "${COLOR_BLUE}For detailed implementation guide, see:${COLOR_RESET}"
echo "  - NETWORK_ISSUES_AND_SOLUTIONS.md"
echo "  - MIGRATION_GUIDE.md"
echo ""

rm -f "$TEMP_FILE"

exit 0
