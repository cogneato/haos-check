#!/bin/bash
#
# HAOS Network Readiness Checker - Docker Simulation Test
#
# This script runs the network checks from inside Docker containers
# to simulate the networking environment that HAOS actually uses.
#
# Usage: ./docker-test.sh
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

echo -e "${CYAN}${BOLD}"
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║  HAOS Network Checker - Docker Simulation Mode                ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: Docker is not installed or not in PATH${NC}"
    echo "Please install Docker to run the container-based tests."
    exit 1
fi

# Check if Docker daemon is running
if ! docker info &> /dev/null; then
    echo -e "${RED}Error: Docker daemon is not running${NC}"
    echo "Please start Docker and try again."
    exit 1
fi

echo "Building test container..."
docker build -t haos-check:latest . -q > /dev/null
echo -e "${GREEN}✓${NC} Container built"
echo ""

# Function to run test and capture exit code
run_test() {
    local mode=$1
    local network_flag=$2
    local description=$3

    echo -e "${BOLD}Test: ${description}${NC}"
    echo "Running checks from inside a Docker container with ${mode} networking..."
    echo ""

    # Run and capture both output and exit code
    set +e
    output=$(docker run --rm --network="${network_flag}" haos-check:latest -v 2>&1)
    exit_code=$?
    set -e

    echo "$output"
    echo ""

    return $exit_code
}

# Store results
BRIDGE_PASSED=false
HOST_PASSED=false

echo "═══════════════════════════════════════════════════════════════"
echo -e "${BOLD}TEST 1: Docker Bridge Network${NC}"
echo "This simulates how HAOS Supervisor containers access the network."
echo "═══════════════════════════════════════════════════════════════"
echo ""

if run_test "bridge" "bridge" "Docker Bridge Network (default)"; then
    BRIDGE_PASSED=true
fi

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo -e "${BOLD}TEST 2: Docker Host Network${NC}"
echo "This bypasses Docker networking and uses the host directly."
echo "If this passes but bridge fails, the issue is Docker-specific."
echo "═══════════════════════════════════════════════════════════════"
echo ""

if run_test "host" "host" "Docker Host Network"; then
    HOST_PASSED=true
fi

# Analysis
echo ""
echo "═══════════════════════════════════════════════════════════════"
echo -e "${BOLD}ANALYSIS${NC}"
echo "═══════════════════════════════════════════════════════════════"
echo ""

if $BRIDGE_PASSED && $HOST_PASSED; then
    echo -e "${GREEN}${BOLD}✓ All tests passed!${NC}"
    echo ""
    echo "Your network should work correctly with HAOS."
    echo "Both direct and Docker-bridged connections can reach all endpoints."

elif ! $BRIDGE_PASSED && $HOST_PASSED; then
    echo -e "${RED}${BOLD}⚠ Docker bridge networking has issues${NC}"
    echo ""
    echo "Host networking works, but Docker bridge networking fails."
    echo "This is a common cause of HAOS installation problems."
    echo ""
    echo "Possible causes:"
    echo "  1. ${BOLD}MTU mismatch${NC} - Docker uses 1500, your network may need lower"
    echo "     Fix: Add to /etc/docker/daemon.json:"
    echo '     {"mtu": 1480}'
    echo ""
    echo "  2. ${BOLD}Firewall blocking Docker${NC} - iptables/nftables rules"
    echo "     Check: sudo iptables -L -n | grep docker"
    echo ""
    echo "  3. ${BOLD}DNS issues in containers${NC}"
    echo "     Check: docker run --rm alpine nslookup google.com"
    echo ""
    echo "  4. ${BOLD}Corporate network restrictions${NC}"
    echo "     Docker bridge uses NAT which some networks block"
    echo ""
    echo "Debug commands:"
    echo "  docker network inspect bridge"
    echo "  docker run --rm alpine ping -c 3 8.8.8.8"
    echo "  docker run --rm alpine nslookup ghcr.io"

elif ! $BRIDGE_PASSED && ! $HOST_PASSED; then
    echo -e "${RED}${BOLD}✗ Both tests failed${NC}"
    echo ""
    echo "Neither host nor Docker networking can reach required endpoints."
    echo "This is likely a general network/firewall issue, not Docker-specific."
    echo ""
    echo "Check:"
    echo "  1. Internet connectivity from this machine"
    echo "  2. DNS resolution (nslookup google.com)"
    echo "  3. Firewall rules blocking outbound HTTPS"
    echo "  4. Proxy configuration requirements"

elif $BRIDGE_PASSED && ! $HOST_PASSED; then
    echo -e "${YELLOW}${BOLD}? Unusual result${NC}"
    echo ""
    echo "Docker bridge works but host networking failed."
    echo "This is unusual - please report this case."
fi

echo ""

# Cleanup
docker rmi haos-check:latest > /dev/null 2>&1 || true
