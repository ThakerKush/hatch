package proxy

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/vmm"
)

// IdleMonitor periodically checks for VMs that have been idle (no proxy
// traffic) longer than a configured timeout and snapshots them.
type IdleMonitor struct {
	db       *store.DB
	vmm      *vmm.Manager
	proxy    *Proxy
	interval time.Duration
	timeout  time.Duration
	stopCh   chan struct{}
}

// NewIdleMonitor creates an idle monitor that checks every interval and
// snapshots VMs idle for longer than timeout.
func NewIdleMonitor(db *store.DB, mgr *vmm.Manager, proxy *Proxy, interval, timeout time.Duration) *IdleMonitor {
	return &IdleMonitor{
		db:       db,
		vmm:      mgr,
		proxy:    proxy,
		interval: interval,
		timeout:  timeout,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the idle-check loop in a background goroutine.
func (m *IdleMonitor) Start() {
	slog.Info("idle monitor started", "interval", m.interval, "timeout", m.timeout)
	go m.loop()
}

// Stop signals the monitor to stop and blocks until it does.
func (m *IdleMonitor) Stop() {
	close(m.stopCh)
}

func (m *IdleMonitor) loop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			slog.Info("idle monitor stopped")
			return
		case <-ticker.C:
			m.check()
		}
	}
}

func (m *IdleMonitor) check() {
	// Get all proxy routes to see which VMs are proxied.
	routes, err := m.db.ListAllRoutes()
	if err != nil {
		slog.Error("idle monitor: list routes", "error", err)
		return
	}

	now := time.Now().Unix()

	for _, route := range routes {
		// Only consider running VMs.
		vm, ok := m.vmm.Get(route.VMID)
		if !ok || vm.State != store.VMStateRunning {
			continue
		}

		lastAccess := m.proxy.LastAccessTime(route.Subdomain)
		if lastAccess == 0 {
			// Never accessed through proxy; use the VM's created_at as baseline.
			lastAccess = vm.CreatedAt.Unix()
		}

		idleSeconds := now - lastAccess
		if idleSeconds < int64(m.timeout.Seconds()) {
			continue
		}

		slog.Info("vm idle, triggering snapshot",
			"vm", route.VMID,
			"subdomain", route.Subdomain,
			"idle_seconds", idleSeconds,
		)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		if _, err := m.vmm.Snapshot(ctx, route.VMID); err != nil {
			slog.Error("idle snapshot failed", "vm", route.VMID, "error", err)
		}
		cancel()
	}
}
