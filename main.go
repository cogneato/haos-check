package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	version = "1.1.0"

	// Timeouts
	dnsTimeout        = 5 * time.Second
	httpTimeout       = 10 * time.Second
	ntpTimeout        = 5 * time.Second
	registryTimeout   = 15 * time.Second
	mtuTimeout        = 5 * time.Second

	// HAOS endpoints
	versionURL       = "https://version.home-assistant.io/stable.json"
	connectivityURL  = "http://checkonline.home-assistant.io/online.txt"
	appArmorURL      = "https://version.home-assistant.io/apparmor_stable.txt"

	// Container registry
	ghcrHost         = "ghcr.io"
	ghcrTokenURL     = "https://ghcr.io/token?scope=repository:home-assistant/green-homeassistant:pull"

	// GitHub (for add-on repos)
	githubAPIURL     = "https://api.github.com"
	githubRepoURL    = "https://github.com/home-assistant/addons"

	// NTP
	ntpServer        = "time.cloudflare.com:123"

	// mDNS
	mdnsPort         = 5353
)

// ANSI colors
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// CheckResult represents the result of a single check
type CheckResult struct {
	Name        string
	Category    string
	Passed      bool
	Message     string
	Details     string
	Duration    time.Duration
	Required    bool
	Info        bool // informational only — failure is expected on some networks
}

// VersionInfo represents the Home Assistant version API response
type VersionInfo struct {
	Channel       string            `json:"channel"`
	Supervisor    string            `json:"supervisor"`
	HomeAssistant map[string]string `json:"homeassistant"`
	CLI           string            `json:"cli"`
	DNS           string            `json:"dns"`
	Audio         string            `json:"audio"`
	Observer      string            `json:"observer"`
	Multicast     string            `json:"multicast"`
}

var (
	useColor    = true
	verboseMode = false
)

func main() {
	// Parse flags
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-h", "--help":
			printHelp()
			return
		case "-v", "--verbose":
			verboseMode = true
		case "--no-color":
			useColor = false
		case "--version":
			fmt.Printf("haos-check version %s\n", version)
			return
		}
	}

	// Disable colors on Windows (unless using Windows Terminal)
	if runtime.GOOS == "windows" && os.Getenv("WT_SESSION") == "" {
		useColor = false
	}

	printBanner()

	results := runAllChecks()

	printSummary(results)
}

func printHelp() {
	fmt.Println(`haos-check - Home Assistant OS Network Readiness Checker

Usage: haos-check [options]

Options:
  -h, --help      Show this help message
  -v, --verbose   Show detailed output for each check
  --no-color      Disable colored output
  --version       Show version information

This tool verifies that your network is ready for a Home Assistant OS
installation by checking connectivity to all required endpoints.

Required endpoints:
  - version.home-assistant.io  (version info, AppArmor profiles)
  - checkonline.home-assistant.io (connectivity validation)
  - ghcr.io (container images)
  - github.com (add-on repositories)
  - time.cloudflare.com (NTP time sync)

For more information, visit: https://github.com/cogneato/haos-check`)
}

func printBanner() {
	cyan := color(colorCyan)
	bold := color(colorBold)
	reset := color(colorReset)

	fmt.Printf(`
%s%s╔═══════════════════════════════════════════════════════════╗
║     Home Assistant OS - Network Readiness Checker         ║
║                       v%s                              ║
╚═══════════════════════════════════════════════════════════╝%s

`, cyan, bold, version, reset)
}

func color(c string) string {
	if useColor {
		return c
	}
	return ""
}

func runAllChecks() []CheckResult {
	var results []CheckResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	checks := []struct {
		name     string
		category string
		fn       func() CheckResult
		required bool
		info     bool
	}{
		// DNS Resolution checks
		{"DNS: version.home-assistant.io", "DNS Resolution", checkDNS("version.home-assistant.io"), true, false},
		{"DNS: ghcr.io", "DNS Resolution", checkDNS("ghcr.io"), true, false},
		{"DNS: github.com", "DNS Resolution", checkDNS("github.com"), true, false},
		{"DNS: checkonline.home-assistant.io", "DNS Resolution", checkDNS("checkonline.home-assistant.io"), true, false},
		{"DNS: time.cloudflare.com", "DNS Resolution", checkDNS("time.cloudflare.com"), true, false},

		// HTTPS Connectivity
		{"HTTPS: Version API", "Home Assistant Services", checkVersionAPI, true, false},
		{"HTTPS: AppArmor Profiles", "Home Assistant Services", checkAppArmor, true, false},
		{"HTTP: Connectivity Check", "Home Assistant Services", checkConnectivity, true, false},

		// Container Registry
		{"Registry: GHCR Authentication", "Container Registry (ghcr.io)", checkGHCRAuth, true, false},

		// GitHub
		{"GitHub: API Access", "GitHub (Add-on Repos)", checkGitHubAPI, true, false},
		{"GitHub: Repository Access", "GitHub (Add-on Repos)", checkGitHubRepo, true, false},

		// NTP
		{"NTP: time.cloudflare.com", "Time Synchronization", checkNTP, true, false},

		// Network Quality — informational, failure is normal on many home networks
		{"MTU: Path MTU Discovery", "Network Quality", checkMTU, false, true},
		{"IPv6: Connectivity", "Network Quality", checkIPv6, false, true},

		// Optional checks
		{"mDNS: Port 5353 (local discovery)", "Local Network", checkMDNS, false, false},
	}

	// Print progress header
	fmt.Printf("Running %d checks...\n\n", len(checks))

	// Run checks concurrently
	for _, check := range checks {
		wg.Add(1)
		go func(c struct {
			name     string
			category string
			fn       func() CheckResult
			required bool
			info     bool
		}) {
			defer wg.Done()
			result := c.fn()
			result.Name = c.name
			result.Category = c.category
			result.Required = c.required
			result.Info = c.info

			mu.Lock()
			results = append(results, result)
			printCheckResult(result)
			mu.Unlock()
		}(check)
	}

	wg.Wait()
	fmt.Println()

	return results
}

func printCheckResult(r CheckResult) {
	green := color(colorGreen)
	red := color(colorRed)
	yellow := color(colorYellow)
	reset := color(colorReset)

	cyan := color(colorCyan)
	status := fmt.Sprintf("%s✓ PASS%s", green, reset)
	if !r.Passed {
		if r.Info {
			status = fmt.Sprintf("%sℹ INFO%s", cyan, reset)
		} else if r.Required {
			status = fmt.Sprintf("%s✗ FAIL%s", red, reset)
		} else {
			status = fmt.Sprintf("%s⚠ WARN%s", yellow, reset)
		}
	}

	fmt.Printf("  [%s] %s (%dms)\n", status, r.Name, r.Duration.Milliseconds())

	if verboseMode && r.Details != "" {
		fmt.Printf("           %s\n", r.Details)
	}

	if !r.Passed && r.Message != "" {
		fmt.Printf("           → %s\n", r.Message)
	}
}

func printSummary(results []CheckResult) {
	green := color(colorGreen)
	red := color(colorRed)
	yellow := color(colorYellow)
	bold := color(colorBold)
	reset := color(colorReset)

	passed := 0
	failed := 0
	warnings := 0
	infos := 0

	for _, r := range results {
		if r.Passed {
			passed++
		} else if r.Info {
			infos++
		} else if r.Required {
			failed++
		} else {
			warnings++
		}
	}

	fmt.Printf("%s%s═══════════════════════════════════════════════════════════════%s\n", bold, color(colorCyan), reset)
	fmt.Printf("%sSummary:%s\n", bold, reset)
	fmt.Printf("  %s✓ Passed:%s  %d\n", green, reset, passed)
	if failed > 0 {
		fmt.Printf("  %s✗ Failed:%s  %d\n", red, reset, failed)
	}
	if warnings > 0 {
		fmt.Printf("  %s⚠ Warnings:%s %d\n", yellow, reset, warnings)
	}
	if infos > 0 {
		fmt.Printf("  %sℹ Info:%s    %d (normal for many home networks)\n", color(colorCyan), reset, infos)
	}
	fmt.Println()

	if failed == 0 {
		fmt.Printf("%s%s🎉 Your network is ready for Home Assistant OS!%s\n\n", bold, green, reset)
		fmt.Println("You can proceed with the installation. HAOS should be able to:")
		fmt.Println("  • Download the Home Assistant Core container")
		fmt.Println("  • Sync time with NTP servers")
		fmt.Println("  • Access add-on repositories")
		fmt.Println("  • Reach the Home Assistant update servers")
	} else {
		fmt.Printf("%s%s❌ Network issues detected that may prevent HAOS installation%s\n\n", bold, red, reset)
		fmt.Println("Please resolve the following issues:")
		fmt.Println()

		for _, r := range results {
			if !r.Passed && r.Required {
				fmt.Printf("  • %s\n", r.Name)
				if r.Message != "" {
					fmt.Printf("    %s\n", r.Message)
				}
			}
		}

		fmt.Println()
		fmt.Println("Common causes:")
		fmt.Println("  • Firewall blocking outbound HTTPS (port 443)")
		fmt.Println("  • DNS server not resolving external domains")
		fmt.Println("  • Captive portal requiring authentication")
		fmt.Println("  • Corporate proxy requiring configuration")
	}

	if warnings > 0 && failed == 0 {
		fmt.Println()
		fmt.Printf("%sNote:%s Some optional checks had warnings. HAOS will likely install,\n", yellow, reset)
		fmt.Println("but you may need to address these for optimal operation.")
	} else if infos > 0 && failed == 0 && warnings == 0 {
		fmt.Println()
		fmt.Printf("%sNote:%s MTU and IPv6 checks are informational — these commonly fail\n", color(colorCyan), reset)
		fmt.Println("on home networks and do not affect HAOS first-boot.")
	}

	// Check for MTU issues and provide specific guidance
	for _, r := range results {
		if strings.Contains(r.Name, "MTU") && r.Message != "" && strings.Contains(r.Message, "not standard") {
			fmt.Println()
			fmt.Printf("%s%sIMPORTANT - MTU Issue Detected:%s\n", bold, yellow, reset)
			fmt.Println("Your network MTU is lower than the standard 1500 bytes.")
			fmt.Println("Docker defaults to 1500, which can cause container networking failures.")
			fmt.Println()
			fmt.Println("Solutions (choose one):")
			fmt.Println()
			fmt.Printf("  %s1. Fix at Router (Recommended):%s\n", bold, reset)
			fmt.Println("     • Log into your router settings")
			fmt.Println("     • Find MTU settings (often under WAN/Internet)")
			fmt.Println("     • Set to match your connection (PPPoE=1492, etc.)")
			fmt.Println("     • Or enable 'MSS Clamping' / 'Clamp MSS to PMTU'")
			fmt.Println()
			fmt.Printf("  %s2. Fix in HAOS (if router can't be changed):%s\n", bold, reset)
			fmt.Println("     • SSH into HAOS after installation")
			fmt.Println("     • This requires advanced configuration")
			fmt.Println()
			break
		}
	}

	fmt.Println()
}

// DNS check function
func checkDNS(hostname string) func() CheckResult {
	return func() CheckResult {
		start := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), dnsTimeout)
		defer cancel()

		resolver := &net.Resolver{}
		ips, err := resolver.LookupIPAddr(ctx, hostname)

		duration := time.Since(start)

		if err != nil {
			return CheckResult{
				Passed:   false,
				Message:  fmt.Sprintf("Failed to resolve: %v", err),
				Duration: duration,
			}
		}

		var ipStrs []string
		for _, ip := range ips {
			ipStrs = append(ipStrs, ip.String())
		}

		return CheckResult{
			Passed:   true,
			Details:  fmt.Sprintf("Resolved to: %s", strings.Join(ipStrs, ", ")),
			Duration: duration,
		}
	}
}

// Version API check
func checkVersionAPI() CheckResult {
	start := time.Now()

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(versionURL)
	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect: %v", err),
			Duration: duration,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("HTTP %d response", resp.StatusCode),
			Duration: duration,
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to read response: %v", err),
			Duration: duration,
		}
	}

	var versionInfo VersionInfo
	if err := json.Unmarshal(body, &versionInfo); err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to parse JSON: %v", err),
			Duration: duration,
		}
	}

	return CheckResult{
		Passed:   true,
		Details:  fmt.Sprintf("Latest: Supervisor %s, HA Core %s", versionInfo.Supervisor, versionInfo.HomeAssistant["default"]),
		Duration: duration,
	}
}

// AppArmor check
func checkAppArmor() CheckResult {
	start := time.Now()

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(appArmorURL)
	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect: %v", err),
			Duration: duration,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("HTTP %d response", resp.StatusCode),
			Duration: duration,
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to read response: %v", err),
			Duration: duration,
		}
	}

	lines := strings.Split(string(body), "\n")

	return CheckResult{
		Passed:   true,
		Details:  fmt.Sprintf("Profile available (%d lines)", len(lines)),
		Duration: duration,
	}
}

// Connectivity check (HTTP)
func checkConnectivity() CheckResult {
	start := time.Now()

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(connectivityURL)
	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect: %v", err),
			Duration: duration,
		}
	}
	defer resp.Body.Close()

	// Check for captive portal (redirect to different domain)
	if resp.Request.URL.Host != "checkonline.home-assistant.io" {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Redirected to %s (captive portal?)", resp.Request.URL.Host),
			Duration: duration,
		}
	}

	if resp.StatusCode != 200 {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("HTTP %d response", resp.StatusCode),
			Duration: duration,
		}
	}

	return CheckResult{
		Passed:   true,
		Details:  "Connectivity confirmed",
		Duration: duration,
	}
}

// GHCR authentication check
func checkGHCRAuth() CheckResult {
	start := time.Now()

	client := &http.Client{Timeout: registryTimeout}
	resp, err := client.Get(ghcrTokenURL)
	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect: %v", err),
			Duration: duration,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("HTTP %d response (token endpoint)", resp.StatusCode),
			Duration: duration,
		}
	}

	return CheckResult{
		Passed:   true,
		Details:  "Anonymous token obtained",
		Duration: duration,
	}
}

// GHCR manifest check
// GitHub API check
func checkGitHubAPI() CheckResult {
	start := time.Now()

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(githubAPIURL)
	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect: %v", err),
			Duration: duration,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("HTTP %d response", resp.StatusCode),
			Duration: duration,
		}
	}

	return CheckResult{
		Passed:   true,
		Details:  "API accessible",
		Duration: duration,
	}
}

// GitHub repo check
func checkGitHubRepo() CheckResult {
	start := time.Now()

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(githubRepoURL)
	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect: %v", err),
			Duration: duration,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("HTTP %d response", resp.StatusCode),
			Duration: duration,
		}
	}

	return CheckResult{
		Passed:   true,
		Details:  "Repository accessible",
		Duration: duration,
	}
}

// NTP check
func checkNTP() CheckResult {
	start := time.Now()

	conn, err := net.DialTimeout("udp", ntpServer, ntpTimeout)
	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect: %v", err),
			Duration: time.Since(start),
		}
	}
	defer conn.Close()

	// NTP request packet (simplified - just checking connectivity)
	// 48 bytes, first byte: LI=0, VN=4, Mode=3 (client)
	req := make([]byte, 48)
	req[0] = 0x23 // LI=0, VN=4, Mode=3

	conn.SetDeadline(time.Now().Add(ntpTimeout))

	_, err = conn.Write(req)
	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("Failed to send request: %v", err),
			Duration: time.Since(start),
		}
	}

	resp := make([]byte, 48)
	_, err = conn.Read(resp)
	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  fmt.Sprintf("No response: %v", err),
			Duration: duration,
		}
	}

	return CheckResult{
		Passed:   true,
		Details:  "NTP server responding",
		Duration: duration,
	}
}

// mDNS check (optional)
func checkMDNS() CheckResult {
	start := time.Now()

	// Just check if we can bind to the mDNS port or connect to it
	// This is a basic check - full mDNS would require multicast
	conn, err := net.DialTimeout("udp", fmt.Sprintf("224.0.0.251:%d", mdnsPort), 2*time.Second)
	duration := time.Since(start)

	if err != nil {
		// Try to check if port is available for listening
		listener, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", mdnsPort))
		if err != nil {
			return CheckResult{
				Passed:   false,
				Message:  fmt.Sprintf("mDNS port %d may be blocked or in use", mdnsPort),
				Duration: duration,
			}
		}
		listener.Close()

		return CheckResult{
			Passed:   true,
			Details:  "mDNS port available for binding",
			Duration: duration,
		}
	}
	defer conn.Close()

	return CheckResult{
		Passed:   true,
		Details:  "mDNS multicast reachable",
		Duration: duration,
	}
}

// MTU check - discovers path MTU to detect potential Docker networking issues
func checkMTU() CheckResult {
	start := time.Now()

	// Test target - use a reliable host
	target := "8.8.8.8"

	// MTU values to test (payload sizes, actual MTU = size + 28 for IP+ICMP headers)
	// 1472 = 1500 MTU (standard Ethernet)
	// 1464 = 1492 MTU (PPPoE)
	// 1452 = 1480 MTU (some VPNs)
	// 1400 = 1428 MTU (conservative)
	testSizes := []struct {
		size int
		mtu  int
		desc string
	}{
		{1472, 1500, "Standard (1500)"},
		{1464, 1492, "PPPoE (1492)"},
		{1452, 1480, "VPN/Tunnel (1480)"},
		{1400, 1428, "Conservative (1428)"},
	}

	var detectedMTU int
	var detectedDesc string

	for _, test := range testSizes {
		if pingWithSize(target, test.size) {
			detectedMTU = test.mtu
			detectedDesc = test.desc
			break
		}
	}

	duration := time.Since(start)

	if detectedMTU == 0 {
		return CheckResult{
			Passed:   false,
			Message:  "Could not determine MTU (ping may be blocked)",
			Details:  "Unable to send ICMP packets - firewall may block ping",
			Duration: duration,
		}
	}

	if detectedMTU < 1500 {
		// MTU is lower than standard - this could cause Docker issues
		return CheckResult{
			Passed:   true, // Still "passes" but with a warning in details
			Message:  fmt.Sprintf("MTU is %d, not standard 1500", detectedMTU),
			Details:  fmt.Sprintf("Detected: %s - Docker default is 1500, may need adjustment", detectedDesc),
			Duration: duration,
		}
	}

	return CheckResult{
		Passed:   true,
		Details:  fmt.Sprintf("Path MTU: %s", detectedDesc),
		Duration: duration,
	}
}

// pingWithSize sends a ping with specific payload size and DF (don't fragment) bit
func pingWithSize(target string, size int) bool {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		// macOS: -D sets DF bit, -s sets payload size
		cmd = exec.Command("ping", "-c", "1", "-t", "3", "-D", "-s", fmt.Sprintf("%d", size), target)
	case "linux":
		// Linux: -M do sets DF bit
		cmd = exec.Command("ping", "-c", "1", "-W", "3", "-M", "do", "-s", fmt.Sprintf("%d", size), target)
	case "windows":
		// Windows: -f sets DF bit, -l sets payload size
		cmd = exec.Command("ping", "-n", "1", "-w", "3000", "-f", "-l", fmt.Sprintf("%d", size), target)
	default:
		return false
	}

	err := cmd.Run()
	return err == nil
}

// IPv6 connectivity check
func checkIPv6() CheckResult {
	start := time.Now()

	// Try to resolve and connect to an IPv6 address
	// Using Google's IPv6 DNS as a test target
	target := "ipv6.google.com"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First check if we can resolve AAAA records
	resolver := &net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, target)
	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  "No IPv6 DNS resolution",
			Details:  "Could not resolve IPv6 address - may not have IPv6",
			Duration: time.Since(start),
		}
	}

	// Find an IPv6 address
	var ipv6Addr string
	for _, ip := range ips {
		if ip.IP.To4() == nil { // It's IPv6
			ipv6Addr = ip.String()
			break
		}
	}

	if ipv6Addr == "" {
		return CheckResult{
			Passed:   false,
			Message:  "No IPv6 address found",
			Details:  "DNS returned only IPv4 addresses",
			Duration: time.Since(start),
		}
	}

	// Try to connect via IPv6
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp6", "["+ipv6Addr+"]:80")
	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Passed:   false,
			Message:  "IPv6 connection failed",
			Details:  fmt.Sprintf("Resolved %s but couldn't connect", ipv6Addr),
			Duration: duration,
		}
	}
	conn.Close()

	return CheckResult{
		Passed:   true,
		Details:  fmt.Sprintf("IPv6 working (%s)", ipv6Addr),
		Duration: duration,
	}
}

// TLS version check helper (for verbose mode)
func getTLSVersion(state *tls.ConnectionState) string {
	switch state.Version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return "Unknown"
	}
}
