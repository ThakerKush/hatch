package vmm

import (
	"context"
	"fmt"
	"net"
	"strconv"
)

type table string

const (
	tableFilter table = ""
	tableNAT    table = "nat"
)

type chain string

const (
	chainPrerouting  chain = "PREROUTING"
	chainPostrouting chain = "POSTROUTING"
	chainForward     chain = "FORWARD"
)

type action string

const (
	actionAccept     action = "ACCEPT"
	actionDrop       action = "DROP"
	actionDNAT       action = "DNAT"
	actionMasquerade action = "MASQUERADE"
)

type rule struct {
	Table    table
	Chain    chain
	Protocol string
	Source   string
	Dest     string
	SrcPort  int
	DstPort  int
	Action   action
	ToDest   string

	// Extra matcher flags (for example conntrack state matching).
	Matches []string
	// Insert at chain position 1 instead of appending.
	Insert bool
}

func (r rule) args() []string {
	a := make([]string, 0, 24)

	if r.Table != tableFilter && r.Table != "" {
		a = append(a, "-t", string(r.Table))
	}
	if r.Protocol != "" {
		a = append(a, "-p", r.Protocol)
	}
	if r.Source != "" {
		a = append(a, "-s", r.Source)
	}
	if r.Dest != "" {
		a = append(a, "-d", r.Dest)
	}
	if r.SrcPort != 0 {
		a = append(a, "--sport", strconv.Itoa(r.SrcPort))
	}
	if r.DstPort != 0 {
		a = append(a, "--dport", strconv.Itoa(r.DstPort))
	}

	a = append(a, r.Matches...)
	a = append(a, "-j", string(r.Action))

	if r.Action == actionDNAT && r.ToDest != "" {
		a = append(a, "--to-destination", r.ToDest)
	}
	return a
}

func (r rule) exists(ctx context.Context) bool {
	args := append([]string{"-C", string(r.Chain)}, r.args()...)
	return run(ctx, "iptables", args...) == nil
}

func (r rule) add(ctx context.Context) error {
	if r.exists(ctx) {
		return nil
	}

	op := []string{"-A", string(r.Chain)}
	if r.Insert {
		op = []string{"-I", string(r.Chain), "1"}
	}
	args := append(op, r.args()...)
	return run(ctx, "iptables", args...)
}

func (r rule) delete(ctx context.Context) error {
	if !r.exists(ctx) {
		return nil
	}
	args := append([]string{"-D", string(r.Chain)}, r.args()...)
	return run(ctx, "iptables", args...)
}

// ensureIPForwarding enables IPv4 forwarding needed for host->guest DNAT.
func ensureIPForwarding(ctx context.Context) error {
	return run(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1")
}

func portInUse(port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

func sshRules(hostPort int, guestIP, allowedCIDR string) []rule {
	return []rule{
		{
			Table:    tableNAT,
			Chain:    chainPrerouting,
			Protocol: "tcp",
			Source:   allowedCIDR,
			DstPort:  hostPort,
			Action:   actionDNAT,
			ToDest:   net.JoinHostPort(guestIP, "22"),
		},
		{
			Table:    tableNAT,
			Chain:    chainPostrouting,
			Protocol: "tcp",
			Source:   guestIP,
			SrcPort:  22,
			Action:   actionMasquerade,
		},
		{
			Table:    tableFilter,
			Chain:    chainForward,
			Protocol: "tcp",
			Source:   allowedCIDR,
			Dest:     guestIP,
			DstPort:  22,
			Matches:  []string{"-m", "conntrack", "--ctstate", "NEW,ESTABLISHED"},
			Action:   actionAccept,
			Insert:   true,
		},
		{
			Table:    tableFilter,
			Chain:    chainForward,
			Protocol: "tcp",
			Source:   guestIP,
			SrcPort:  22,
			Matches:  []string{"-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED"},
			Action:   actionAccept,
			Insert:   true,
		},
		{
			Table:    tableFilter,
			Chain:    chainForward,
			Protocol: "tcp",
			Dest:     guestIP,
			DstPort:  22,
			Action:   actionDrop,
		},
	}
}

func setupRules(ctx context.Context, rules []rule) error {
	for _, r := range rules {
		if err := r.add(ctx); err != nil {
			return err
		}
	}
	return nil
}

func teardownRules(ctx context.Context, rules []rule) {
	for _, r := range rules {
		_ = r.delete(ctx)
	}
}

func setupSSHForward(ctx context.Context, hostPort int, guestIP, allowedCIDR string) error {
	if hostPort <= 0 || hostPort > 65535 {
		return fmt.Errorf("invalid ssh port: %d", hostPort)
	}
	if net.ParseIP(guestIP) == nil {
		return fmt.Errorf("invalid guest ip: %q", guestIP)
	}
	if _, _, err := net.ParseCIDR(allowedCIDR); err != nil {
		return fmt.Errorf("invalid HATCH_SSH_ALLOWED_CIDR %q: %w", allowedCIDR, err)
	}
	if err := ensureIPForwarding(ctx); err != nil {
		return err
	}

	return setupRules(ctx, sshRules(hostPort, guestIP, allowedCIDR))
}

func teardownSSHForward(ctx context.Context, hostPort int, guestIP, allowedCIDR string) {
	if hostPort <= 0 || net.ParseIP(guestIP) == nil {
		return
	}
	teardownRules(ctx, sshRules(hostPort, guestIP, allowedCIDR))
}
