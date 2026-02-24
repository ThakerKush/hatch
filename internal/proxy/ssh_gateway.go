package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/vmm"
)

// SSHGateway accepts TCP connections on VM SSH ports and can wake
// snapshotted VMs on demand before proxying traffic to guest :22.
type SSHGateway struct {
	db          *store.DB
	vmm         *vmm.Manager
	wakeTimeout time.Duration
	interval    time.Duration

	mu        sync.Mutex
	listeners map[int]net.Listener
	stopCh    chan struct{}
	wg        sync.WaitGroup

	// Serialises wake operations per VM to avoid concurrent restore storms.
	wakeMu sync.Map // map[string]*sync.Mutex
}

func NewSSHGateway(db *store.DB, mgr *vmm.Manager, wakeTimeout time.Duration) *SSHGateway {
	return &SSHGateway{
		db:          db,
		vmm:         mgr,
		wakeTimeout: wakeTimeout,
		interval:    2 * time.Second,
		listeners:   make(map[int]net.Listener),
		stopCh:      make(chan struct{}),
	}
}

// Start launches the reconciliation loop and begins listening on active VM SSH ports.
func (g *SSHGateway) Start() {
	slog.Info("ssh wake gateway started")
	g.reconcile()
	g.wg.Add(1)
	go g.loop()
}

// Stop stops listeners and waits for background goroutines to exit.
func (g *SSHGateway) Stop() {
	close(g.stopCh)
	g.wg.Wait()
}

func (g *SSHGateway) loop() {
	defer g.wg.Done()
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			g.closeAllListeners()
			slog.Info("ssh wake gateway stopped")
			return
		case <-ticker.C:
			g.reconcile()
		}
	}
}

func (g *SSHGateway) reconcile() {
	vms, err := g.db.ListVMs()
	if err != nil {
		slog.Error("ssh gateway: list vms", "error", err)
		return
	}

	want := make(map[int]struct{})
	for i := range vms {
		if p := vms[i].SSHPort; p > 0 {
			want[p] = struct{}{}
		}
	}

	// Copy listener map snapshot under lock so we can reconcile without
	// holding the mutex while calling net.Listen / Close.
	g.mu.Lock()
	current := make(map[int]net.Listener, len(g.listeners))
	for p, ln := range g.listeners {
		current[p] = ln
	}
	g.mu.Unlock()

	// Add missing listeners.
	for p := range want {
		if _, ok := current[p]; ok {
			continue
		}

		ln, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(p)))
		if err != nil {
			slog.Warn("ssh gateway: listen failed", "port", p, "error", err)
			continue
		}

		g.mu.Lock()
		g.listeners[p] = ln
		g.mu.Unlock()

		slog.Info("ssh gateway: listening", "port", p)
		g.wg.Add(1)
		go g.servePort(p, ln)
	}

	// Remove stale listeners for ports no longer attached to any VM.
	for p, ln := range current {
		if _, ok := want[p]; ok {
			continue
		}
		_ = ln.Close()
		g.mu.Lock()
		delete(g.listeners, p)
		g.mu.Unlock()
	}
}

func (g *SSHGateway) servePort(port int, ln net.Listener) {
	defer g.wg.Done()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-g.stopCh:
				return
			default:
			}
			slog.Debug("ssh gateway: accept failed", "port", port, "error", err)
			return
		}
		go g.handleConn(port, conn)
	}
}

func (g *SSHGateway) handleConn(port int, conn net.Conn) {
	defer conn.Close()

	vm, err := g.db.GetVMBySSHPort(port)
	if err != nil || vm == nil {
		return
	}

	switch vm.State {
	case store.VMStateRunning:
		// Ready to proxy.
	case store.VMStateSnapshotted:
		if err := g.wakeVM(context.Background(), vm.ID); err != nil {
			slog.Error("ssh gateway: wake vm failed", "vm", vm.ID, "port", port, "error", err)
			return
		}
		vm, err = g.db.GetVM(vm.ID)
		if err != nil || vm == nil {
			return
		}
		if vm.State != store.VMStateRunning {
			return
		}
	default:
		return
	}

	if vm.GuestIP == "" {
		return
	}

	upstream, err := net.DialTimeout("tcp", net.JoinHostPort(vm.GuestIP, "22"), 10*time.Second)
	if err != nil {
		slog.Debug("ssh gateway: dial guest failed", "vm", vm.ID, "guest_ip", vm.GuestIP, "error", err)
		return
	}
	defer upstream.Close()

	pipeBidirectional(conn, upstream)
}

func (g *SSHGateway) wakeVM(ctx context.Context, vmID string) error {
	val, _ := g.wakeMu.LoadOrStore(vmID, &sync.Mutex{})
	mu := val.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	vm, ok := g.vmm.Get(vmID)
	if !ok {
		return fmt.Errorf("vm not found: %s", vmID)
	}
	if vm.State == store.VMStateRunning {
		return nil
	}
	if vm.State != store.VMStateSnapshotted {
		return fmt.Errorf("vm %s state %q is not wakeable", vmID, vm.State)
	}

	ctx, cancel := context.WithTimeout(ctx, g.wakeTimeout)
	defer cancel()

	slog.Info("ssh gateway: waking snapshotted vm", "vm", vmID)
	if _, err := g.vmm.Restore(ctx, vmID); err != nil {
		g.vmm.MarkError(vmID, err)
		return fmt.Errorf("restore vm %s: %w", vmID, err)
	}

	// Brief pause so restored networking/sshd is ready before proxying.
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (g *SSHGateway) closeAllListeners() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for port, ln := range g.listeners {
		_ = ln.Close()
		delete(g.listeners, port)
	}
}

func pipeBidirectional(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		if tc, ok := a.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		if tc, ok := b.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()

	wg.Wait()
}
