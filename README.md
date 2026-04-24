# HAOS Network Readiness Checker

> **Unofficial tool.** Not affiliated with or endorsed by the Home Assistant project or Nabu Casa. Built and maintained by [@cogneato](https://github.com/cogneato).

A tool to verify your network is ready for a Home Assistant Operating System (HAOS) installation.

## Two Testing Modes

### 1. Basic Mode (standalone binary)
Run directly on any machine on your network. Quick and easy, but tests from the host's perspective.

### 2. Docker Simulation Mode (recommended)
Run from inside Docker containers to simulate how HAOS actually works. This catches Docker-specific networking issues that the basic mode misses.

**Why does this matter?** HAOS runs the Supervisor inside a Docker container. Even if your host can reach the internet, Docker's bridge networking might not be able to. Common causes:
- MTU mismatches (Docker uses 1500, your network might need 1480)
- Firewall rules blocking Docker's NAT
- Corporate networks that don't recognize container traffic
- DNS resolution differences inside containers

## What it checks

Before HAOS can complete its first boot and reach the onboarding screen, it needs to:

1. **Resolve DNS** for Home Assistant services
2. **Fetch version information** from `version.home-assistant.io`
3. **Download container images** from GitHub Container Registry (`ghcr.io`)
4. **Clone add-on repositories** from GitHub
5. **Sync time** via NTP

This tool tests all of these requirements from your network.

## Quick Start

### Download a pre-built binary

Download the appropriate binary for your system from the [Releases](https://github.com/cogneato/haos-check/releases) page:

| Platform | Architecture | Download |
|----------|-------------|----------|
| Linux | x86_64 (Intel/AMD) | `haos-check-linux-amd64` |
| Linux | ARM64 (Pi 4/5) | `haos-check-linux-arm64` |
| Linux | ARMv7 (Pi 3) | `haos-check-linux-armv7` |
| macOS | Intel | `haos-check-darwin-amd64` |
| macOS | Apple Silicon | `haos-check-darwin-arm64` |
| Windows | x86_64 | `haos-check-windows-amd64.exe` |

Then run it:

```bash
# Linux/macOS
chmod +x haos-check-*
./haos-check-linux-amd64

# Windows
haos-check-windows-amd64.exe
```

### Build from source

Requires Go 1.21 or later:

```bash
git clone https://github.com/cogneato/haos-check.git
cd haos-check
make build
./haos-check
```

### Docker Simulation Mode (Recommended)

This is the most accurate test because it runs inside Docker containers, simulating how HAOS actually works:

```bash
# Requires Docker to be installed and running
./docker-test.sh
```

This runs two tests:
1. **Bridge network** - How HAOS containers actually communicate (via Docker NAT)
2. **Host network** - Direct host networking (bypasses Docker)

If bridge fails but host passes, you have a Docker networking issue that would affect HAOS.

## Usage

```
haos-check [options]

Options:
  -h, --help      Show help message
  -v, --verbose   Show detailed output for each check
  --no-color      Disable colored output
  --version       Show version information
```

## Example Output

### All checks passing:

```
╔═══════════════════════════════════════════════════════════╗
║     Home Assistant OS - Network Readiness Checker         ║
╚═══════════════════════════════════════════════════════════╝

Running 14 checks...

  [✓ PASS] DNS: version.home-assistant.io (52ms)
  [✓ PASS] DNS: ghcr.io (48ms)
  [✓ PASS] DNS: github.com (45ms)
  [✓ PASS] HTTPS: Version API (156ms)
  [✓ PASS] Registry: GHCR Authentication (203ms)
  [✓ PASS] Registry: GHCR Manifest Access (312ms)
  ...

Summary:
  ✓ Passed:  14

🎉 Your network is ready for Home Assistant OS!
```

### With failures:

```
  [✗ FAIL] DNS: ghcr.io (5001ms)
           → Failed to resolve: lookup ghcr.io: no such host

Summary:
  ✓ Passed:  10
  ✗ Failed:  4

❌ Network issues detected that may prevent HAOS installation

Please resolve the following issues:
  • DNS: ghcr.io
    Failed to resolve: lookup ghcr.io: no such host
```

## Endpoints Tested

| Endpoint | Protocol | Purpose |
|----------|----------|---------|
| `version.home-assistant.io` | HTTPS | Version info, AppArmor profiles |
| `checkonline.home-assistant.io` | HTTP | Connectivity validation |
| `ghcr.io` | HTTPS | Container images |
| `github.com` | HTTPS | Add-on repositories |
| `time.cloudflare.com` | NTP (UDP 123) | Time synchronization |

## Common Issues

### Firewall blocking connections

Ensure outbound access on:
- TCP port 443 (HTTPS)
- TCP port 80 (HTTP)
- UDP port 123 (NTP)

### Corporate proxy

HAOS doesn't support HTTP proxies out of the box. You may need to configure your network to allow direct access to the required endpoints.

### Captive portal

If you're on a network with a captive portal (hotel, airport, etc.), you'll need to authenticate through the portal first.

### DNS filtering

Some DNS providers (like Pi-hole or corporate DNS) may block certain domains. Ensure the required endpoints are whitelisted.

### Docker bridge networking fails (but host works)

This is the most common "hidden" issue. The basic check passes, but HAOS fails during first boot.

**MTU mismatch** (most common):
```bash
# Check your network MTU
ping -M do -s 1472 google.com

# If it fails, try lower values until it works
# Then set Docker's MTU in /etc/docker/daemon.json:
{"mtu": 1480}

# Restart Docker
sudo systemctl restart docker
```

**Firewall blocking Docker**:
```bash
# Check iptables rules
sudo iptables -L -n | grep -i docker

# Check if Docker traffic is being blocked
sudo iptables -L FORWARD -n -v
```

**Test Docker DNS directly**:
```bash
docker run --rm alpine nslookup ghcr.io
docker run --rm alpine nslookup version.home-assistant.io
```

**Test Docker connectivity**:
```bash
docker run --rm alpine ping -c 3 8.8.8.8
docker run --rm alpine wget -q -O- https://version.home-assistant.io/stable.json | head
```

## Building for all platforms

```bash
make build-all
```

This creates binaries in the `build/` directory for:
- Linux (amd64, arm64, armv7, armv6)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

## License

Apache 2.0
