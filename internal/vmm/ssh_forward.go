package vmm

import (
	"context"
	"fmt"
	"net"
	"strconv"
)

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

	port := strconv.Itoa(hostPort)
	dnatTarget := net.JoinHostPort(guestIP, "22")

	// NAT: host:<port> -> guest:22
	if err := addIPTablesRuleIfMissing(ctx, true,
		"PREROUTING", "-p", "tcp", "-s", allowedCIDR, "--dport", port, "-j", "DNAT", "--to-destination", dnatTarget,
	); err != nil {
		return err
	}

	// Filter: allow only configured source CIDR to reach guest:22.
	if err := addIPTablesRuleIfMissing(ctx, false,
		"FORWARD", "-p", "tcp", "-s", allowedCIDR, "-d", guestIP, "--dport", "22", "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT",
	); err != nil {
		return err
	}
	if err := addIPTablesRuleIfMissing(ctx, false,
		"FORWARD", "-p", "tcp", "-s", guestIP, "--sport", "22", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT",
	); err != nil {
		return err
	}
	// Explicitly drop non-allowed traffic to guest:22 for this VM.
	if err := addIPTablesRuleIfMissing(ctx, false,
		"FORWARD", "-p", "tcp", "-d", guestIP, "--dport", "22", "-j", "DROP",
	); err != nil {
		return err
	}
	return nil
}

func teardownSSHForward(ctx context.Context, hostPort int, guestIP, allowedCIDR string) {
	if hostPort <= 0 || net.ParseIP(guestIP) == nil {
		return
	}
	port := strconv.Itoa(hostPort)
	dnatTarget := net.JoinHostPort(guestIP, "22")

	_ = deleteIPTablesRuleIfPresent(ctx, true,
		"PREROUTING", "-p", "tcp", "-s", allowedCIDR, "--dport", port, "-j", "DNAT", "--to-destination", dnatTarget,
	)
	_ = deleteIPTablesRuleIfPresent(ctx, false,
		"FORWARD", "-p", "tcp", "-s", allowedCIDR, "-d", guestIP, "--dport", "22", "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT",
	)
	_ = deleteIPTablesRuleIfPresent(ctx, false,
		"FORWARD", "-p", "tcp", "-s", guestIP, "--sport", "22", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT",
	)
	_ = deleteIPTablesRuleIfPresent(ctx, false,
		"FORWARD", "-p", "tcp", "-d", guestIP, "--dport", "22", "-j", "DROP",
	)
}

func addIPTablesRuleIfMissing(ctx context.Context, nat bool, chain string, spec ...string) error {
	if err := checkIPTablesRule(ctx, nat, chain, spec...); err == nil {
		return nil
	}
	return appendIPTablesRule(ctx, nat, chain, spec...)
}

func deleteIPTablesRuleIfPresent(ctx context.Context, nat bool, chain string, spec ...string) error {
	if err := checkIPTablesRule(ctx, nat, chain, spec...); err != nil {
		return nil
	}
	return deleteIPTablesRule(ctx, nat, chain, spec...)
}

func checkIPTablesRule(ctx context.Context, nat bool, chain string, spec ...string) error {
	args := []string{}
	if nat {
		args = append(args, "-t", "nat")
	}
	args = append(args, "-C", chain)
	args = append(args, spec...)
	return run(ctx, "iptables", args...)
}

func appendIPTablesRule(ctx context.Context, nat bool, chain string, spec ...string) error {
	args := []string{}
	if nat {
		args = append(args, "-t", "nat")
	}
	args = append(args, "-A", chain)
	args = append(args, spec...)
	return run(ctx, "iptables", args...)
}

func deleteIPTablesRule(ctx context.Context, nat bool, chain string, spec ...string) error {
	args := []string{}
	if nat {
		args = append(args, "-t", "nat")
	}
	args = append(args, "-D", chain)
	args = append(args, spec...)
	return run(ctx, "iptables", args...)
}
