package vmm

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
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
//does this rule exist?
func (r rule) exists(ctx context.Context) bool {
	args := append([]string{"-C", string(r.Chain)}, r.args()...)
	return run(ctx, "iptables", args...) == nil
}

//add if rule does not exist
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

// list of rules for SSH port forwarding
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

// flushStaleSSHRules scans the iptables nat and filter tables and removes
// every rule related to SSH port forwarding. This is more robust than
// per-VM teardown because it catches orphaned rules from VMs that were
// deleted from the DB, and rules created with a different SSH allowed CIDR.
func flushStaleSSHRules(ctx context.Context, portMin, portMax int) int {
	type chainSpec struct {
		table string
		chain string
		match func(string) bool
	}

	specs := []chainSpec{
		{
			table: "nat",
			chain: "PREROUTING",
			match: func(line string) bool {
				if !strings.Contains(line, "DNAT") {
					return false
				}
				p := extractDport(line)
				return p >= portMin && p <= portMax
			},
		},
		{
			table: "nat",
			chain: "POSTROUTING",
			match: func(line string) bool {
				return strings.Contains(line, "MASQUERADE") && strings.Contains(line, "--sport 22")
			},
		},
		{
			table: "",
			chain: "FORWARD",
			match: func(line string) bool {
				return strings.Contains(line, "--dport 22") || strings.Contains(line, "--sport 22")
			},
		},
	}

	removed := 0
	for _, s := range specs {
		listArgs := []string{"-S", s.chain}
		if s.table != "" {
			listArgs = []string{"-t", s.table, "-S", s.chain}
		}

		out, err := exec.CommandContext(ctx, "iptables", listArgs...).CombinedOutput()
		if err != nil {
			continue
		}

		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if !strings.HasPrefix(line, "-A ") || !s.match(line) {
				continue
			}
			delLine := "-D" + line[2:]
			delArgs := strings.Fields(delLine)
			if s.table != "" {
				delArgs = append([]string{"-t", s.table}, delArgs...)
			}
			if err := run(ctx, "iptables", delArgs...); err == nil {
				removed++
			}
		}
	}
	return removed
}

func extractDport(line string) int {
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == "--dport" && i+1 < len(fields) {
			port, _ := strconv.Atoi(fields[i+1])
			return port
		}
	}
	return 0
}
