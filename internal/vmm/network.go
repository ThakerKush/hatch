package vmm

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
)
// EnsureBridge creates a Linux bridge (virtual switch) with the given name and IP range, or verifies it exists
func EnsureBridge(ctx context.Context, name, cidr string) error {
	if name == "" {
		return errors.New("bridge name required")
	}
	if cidr == "" {
		return errors.New("bridge cidr required")
	}

	if err := run(ctx, "ip", "link", "show", name); err != nil {
		if err := run(ctx, "ip", "link", "add", "name", name, "type", "bridge"); err != nil {
			return err
		}
	}
	//TODO: kinda seems redudant, we already checked if the bridge exists above
	if err := run(ctx, "ip", "addr", "show", "dev", name); err != nil {
		return err
	}

	if err := run(ctx, "ip", "addr", "add", cidr, "dev", name); err != nil {
		if !strings.Contains(err.Error(), "File exists") {
			return err
		}
	}

	return run(ctx, "ip", "link", "set", name, "up")
}

// Create a virtual NIC, activate it and plug it into the specified bridge
func CreateTap(ctx context.Context, name, bridge string) error {
	if name == "" {
		return errors.New("tap name required")
	}
	if bridge == "" {
		return errors.New("bridge name required")
	}

	if err := run(ctx, "ip", "tuntap", "add", "dev", name, "mode", "tap"); err != nil {
		return err
	}
	if err := run(ctx, "ip", "link", "set", name, "up"); err != nil {
		return err
	}
	return run(ctx, "ip", "link", "set", name, "master", bridge)
}

func DeleteTap(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	return run(ctx, "ip", "link", "del", name)
}

func run(ctx context.Context, command string, args ...string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", command, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

type IPAllocator struct {
	mu       sync.Mutex
	network  *net.IPNet
	nextHost byte
	inUse    map[string]bool
}

func NewIPAllocator(bridgeCIDR string) *IPAllocator {
	_, network, _ := net.ParseCIDR(bridgeCIDR)
	return &IPAllocator{
		network:  network,
		nextHost: 2,
		inUse:    make(map[string]bool),
	}
}

func (a *IPAllocator) Allocate() (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.network == nil {
		return nil, errors.New("bridge cidr is invalid")
	}

	base := a.network.IP.To4()
	if base == nil {
		return nil, errors.New("only ipv4 is supported for now")
	}

	for i := 0; i < 250; i++ {
		ip := net.IPv4(base[0], base[1], base[2], a.nextHost)
		a.nextHost++
		if a.nextHost >= 254 {
			a.nextHost = 2
		}
		if !a.inUse[ip.String()] {
			a.inUse[ip.String()] = true
			return ip, nil
		}
	}

	return nil, errors.New("no available IPs")
}

func (a *IPAllocator) Release(ip net.IP) {
	if ip == nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.inUse, ip.String())
}

func randomMAC() string {
	mac := make([]byte, 6)
	_, _ = rand.Read(mac)
	mac[0] = (mac[0] | 0x02) & 0xfe
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5],
	)
}
