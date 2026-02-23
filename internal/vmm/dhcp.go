package vmm

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DHCPServer manages a dnsmasq process that serves DHCP on the VM bridge.
// Static host mappings (MAC → IP) are maintained in a hosts file; dnsmasq is
// signalled (SIGHUP) to reload whenever mappings change.
type DHCPServer struct {
	bridgeName string
	gatewayIP  string
	rangeStart string
	rangeEnd   string
	subnetMask string
	dataDir    string
	hostsFile  string
	leaseFile  string
	logFile    string
	cmd        *exec.Cmd
	exited     chan struct{} // closed when the dnsmasq process exits
	mu         sync.Mutex
}

func NewDHCPServer(bridgeName, bridgeCIDR, dataDir string) (*DHCPServer, error) {
	ip, network, err := net.ParseCIDR(bridgeCIDR)
	if err != nil {
		return nil, fmt.Errorf("parse bridge CIDR %q: %w", bridgeCIDR, err)
	}

	base := network.IP.To4()
	if base == nil {
		return nil, fmt.Errorf("only IPv4 is supported")
	}

	dhcpDir := filepath.Join(dataDir, "dhcp")

	return &DHCPServer{
		bridgeName: bridgeName,
		gatewayIP:  ip.String(),
		rangeStart: net.IPv4(base[0], base[1], base[2], 2).String(),
		rangeEnd:   net.IPv4(base[0], base[1], base[2], 254).String(),
		subnetMask: net.IP(network.Mask).String(),
		dataDir:    dhcpDir,
		hostsFile:  filepath.Join(dhcpDir, "hosts"),
		leaseFile:  filepath.Join(dhcpDir, "leases"),
		logFile:    filepath.Join(dhcpDir, "dnsmasq.log"),
	}, nil
}

// Start launches dnsmasq in the foreground as a managed subprocess.
func (d *DHCPServer) Start() error {
	if _, err := exec.LookPath("dnsmasq"); err != nil {
		return fmt.Errorf("dnsmasq not found in PATH; install it (e.g. apt install dnsmasq-base): %w", err)
	}

	if err := os.MkdirAll(d.dataDir, 0o755); err != nil {
		return fmt.Errorf("create DHCP data dir: %w", err)
	}

	// Truncate the hosts file (fresh start, entries added per-VM) and the
	// lease file (all leases from a previous run are stale — VMs are dead
	// after a daemon restart). Without truncating leases, dnsmasq honours
	// old leases for different MACs and ignores our static allocations.
	if err := os.WriteFile(d.hostsFile, []byte{}, 0o644); err != nil {
		return fmt.Errorf("create DHCP hosts file: %w", err)
	}
	if err := os.WriteFile(d.leaseFile, []byte{}, 0o644); err != nil {
		return fmt.Errorf("truncate DHCP lease file: %w", err)
	}

	dhcpRange := fmt.Sprintf("%s,%s,%s,12h", d.rangeStart, d.rangeEnd, d.subnetMask)

	d.cmd = exec.Command("dnsmasq",
		"--interface="+d.bridgeName,
		"--bind-interfaces",
		"--except-interface=lo",
		"--dhcp-range="+dhcpRange,
		"--dhcp-hostsfile="+d.hostsFile,
		"--dhcp-leasefile="+d.leaseFile,
		"--dhcp-option=3,"+d.gatewayIP,    // default gateway
		"--dhcp-option=6,8.8.8.8,8.8.4.4", // DNS servers
		"--dhcp-authoritative",
		"--no-resolv",
		"--no-hosts",
		"--no-daemon",
		"--log-dhcp",
		"--log-facility="+d.logFile,
	)

	if err := d.cmd.Start(); err != nil {
		return fmt.Errorf("start dnsmasq: %w", err)
	}

	log.Printf("dnsmasq started (pid %d) serving DHCP on %s [%s – %s]",
		d.cmd.Process.Pid, d.bridgeName, d.rangeStart, d.rangeEnd)

	// Channel to detect early exit.
	d.exited = make(chan struct{})

	// Reap the process in the background to avoid zombies.
	go func() {
		if err := d.cmd.Wait(); err != nil {
			log.Printf("dnsmasq exited: %v", err)
		}
		close(d.exited)
	}()

	// Give dnsmasq a moment to crash (if it's going to).
	select {
	case <-d.exited:
		logData, _ := os.ReadFile(d.logFile)
		return fmt.Errorf("dnsmasq exited immediately after start; log: %s", string(logData))
	case <-time.After(200 * time.Millisecond):
		// Still running — good.
	}

	return nil
}

// AddHost registers a static MAC → IP mapping and signals dnsmasq to reload.
func (d *DHCPServer) AddHost(mac, ip string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, _ := os.ReadFile(d.hostsFile)
	entry := mac + "," + ip + "\n"
	data = append(data, []byte(entry)...)

	if err := os.WriteFile(d.hostsFile, data, 0o644); err != nil {
		return fmt.Errorf("write DHCP host entry: %w", err)
	}

	return d.reload()
}

// RemoveHost removes the mapping for the given MAC and signals dnsmasq to reload.
func (d *DHCPServer) RemoveHost(mac string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, err := os.ReadFile(d.hostsFile)
	if err != nil {
		return nil // nothing to remove
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var keep []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, mac+",") {
			keep = append(keep, line)
		}
	}

	content := ""
	if len(keep) > 0 {
		content = strings.Join(keep, "\n") + "\n"
	}

	if err := os.WriteFile(d.hostsFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("rewrite DHCP hosts file: %w", err)
	}

	return d.reload()
}

// reload sends SIGHUP to dnsmasq so it re-reads the hosts file.
func (d *DHCPServer) reload() error {
	if d.cmd == nil || d.cmd.Process == nil {
		return fmt.Errorf("dnsmasq is not running")
	}
	// Check if the process has already exited.
	select {
	case <-d.exited:
		logData, _ := os.ReadFile(d.logFile)
		return fmt.Errorf("dnsmasq process has exited; log: %s", string(logData))
	default:
	}
	return d.cmd.Process.Signal(syscall.SIGHUP)
}

// Stop terminates the dnsmasq process.
func (d *DHCPServer) Stop() {
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Signal(syscall.SIGTERM)
	}
}
