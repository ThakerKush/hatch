package vmm

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ThakerKush/Hatch/internal/config"
	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/util"
)

// CreateOptions are the parameters accepted when creating a new VM.
type CreateOptions struct {
	ImageID        string
	TemplateID     string
	VCPUCount      int
	MemMib         int
	BootArgs       string
	EnableNetwork  bool
	GuestIP        string
	GuestMAC       string
	InstanceSocket string
	UserData       string
}

// Manager orchestrates VM lifecycle operations (create, stop, delete,
// snapshot, restore) and owns the bridge/DHCP/IP-allocator singletons.
type Manager struct {
	cfg       config.Config
	db        *store.DB
	s3        *store.S3Client // nil when S3 is not configured
	allocator *IPAllocator
	dhcp      *DHCPServer

	mu          sync.RWMutex
	machines    map[string]machineHandle // runtime-only; keyed by VM ID
	bridgeReady bool
}

// NewManager creates a Manager backed by the given database.
// The optional S3 client enables snapshot storage; pass nil to disable.
func NewManager(cfg config.Config, db *store.DB, s3 *store.S3Client) (*Manager, error) {
	allocator := NewIPAllocator(cfg.BridgeCIDR)

	dhcp, err := NewDHCPServer(cfg.BridgeName, cfg.BridgeCIDR, cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("init DHCP server: %w", err)
	}

	m := &Manager{
		cfg:       cfg,
		db:        db,
		s3:        s3,
		allocator: allocator,
		dhcp:      dhcp,
		machines:  make(map[string]machineHandle),
	}

	// On startup, mark any VMs left in a transient state (from a prior crash)
	// so they don't appear as "running" when no machine handle exists.
	if stale, err := db.MarkStaleVMs(); err != nil {
		slog.Warn("failed to mark stale VMs", "error", err)
	} else if stale > 0 {
		slog.Warn("marked stale VMs from previous run", "count", stale)
	}

	return m, nil
}

// ---------- Create ----------

func (m *Manager) CreateAndStart(ctx context.Context, opts CreateOptions) (*store.VM, error) {
	// Look up the base image.
	image, err := m.db.GetImage(opts.ImageID)
	if err != nil {
		return nil, fmt.Errorf("lookup image: %w", err)
	}
	if image == nil {
		return nil, fmt.Errorf("image not found: %s", opts.ImageID)
	}

	vmID := util.RandomID("vm")
	vmDir := filepath.Join(m.cfg.DataDir, "vms", vmID)
	if err := os.MkdirAll(vmDir, 0o755); err != nil {
		return nil, err
	}

	socketPath := filepath.Join(vmDir, "firecracker.socket")
	if opts.InstanceSocket != "" {
		socketPath = opts.InstanceSocket
	}
	_ = os.RemoveAll(socketPath)

	now := time.Now().UTC()
	vm := &store.VM{
		ID:            vmID,
		ImageID:       image.ID,
		TemplateID:    opts.TemplateID,
		State:         store.VMStateStarting,
		VCPUCount:     clampInt(opts.VCPUCount, 1, 32, m.cfg.DefaultVCPU),
		MemMib:        clampInt(opts.MemMib, 128, 65536, m.cfg.DefaultMemMib),
		SocketPath:    socketPath,
		WorkDir:       vmDir,
		UserData:      opts.UserData,
		EnableNetwork: opts.EnableNetwork,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Persist the VM record before doing anything else so it's visible even
	// if creation fails midway (state will be marked "error").
	if err := m.db.CreateVM(*vm); err != nil {
		_ = os.RemoveAll(vmDir)
		return nil, err
	}

	var tapName, guestIP, macAddr, cloudInitPath string

	if opts.EnableNetwork {
		if err := m.ensureBridge(ctx); err != nil {
			m.markError(vmID, err)
			return nil, err
		}

		tapName = fmt.Sprintf("fctap-%s", vmID[:8])
		if err := CreateTap(ctx, tapName, m.cfg.BridgeName); err != nil {
			m.markError(vmID, err)
			return nil, err
		}

		if opts.GuestIP != "" {
			guestIP = opts.GuestIP
		} else {
			ip, err := m.allocator.Allocate()
			if err != nil {
				_ = DeleteTap(ctx, tapName)
				m.markError(vmID, err)
				return nil, err
			}
			guestIP = ip.String()
		}

		macAddr = opts.GuestMAC
		if macAddr == "" {
			macAddr = randomMAC()
		}

		if err := m.dhcp.AddHost(macAddr, guestIP); err != nil {
			_ = DeleteTap(ctx, tapName)
			if ip := net.ParseIP(guestIP); ip != nil {
				m.allocator.Release(ip)
			}
			m.markError(vmID, err)
			return nil, fmt.Errorf("add DHCP reservation: %w", err)
		}

		ciPath, err := CreateCloudInitDisk(ctx, vmDir, vmID, macAddr, opts.UserData)
		if err != nil {
			_ = m.dhcp.RemoveHost(macAddr)
			_ = DeleteTap(ctx, tapName)
			if ip := net.ParseIP(guestIP); ip != nil {
				m.allocator.Release(ip)
			}
			m.markError(vmID, err)
			return nil, fmt.Errorf("create cloud-init disk: %w", err)
		}
		cloudInitPath = ciPath

		vm.TapName = tapName
		vm.GuestIP = guestIP
		vm.GuestMAC = macAddr

		// Update the DB with network details.
		if err := m.db.UpdateVMNetwork(vmID, tapName, guestIP, macAddr); err != nil {
			slog.Warn("failed to persist network info", "vm", vmID, "error", err)
		}
	}

	bootArgs := image.BootArgs
	if opts.BootArgs != "" {
		bootArgs = opts.BootArgs
	}
	if bootArgs == "" {
		bootArgs = m.cfg.DefaultBootArgs
	}

	machine, err := newMachine(ctx, m.cfg.FirecrackerBinary, machineConfig{
		socketPath:    socketPath,
		kernelPath:    image.KernelPath,
		kernelArgs:    bootArgs,
		rootfsPath:    image.RootfsPath,
		vmID:          vmID,
		vcpuCount:     int64(vm.VCPUCount),
		memMib:        int64(vm.MemMib),
		tapName:       tapName,
		macAddr:       macAddr,
		logDir:        vmDir,
		cloudInitPath: cloudInitPath,
	})
	if err != nil {
		m.markError(vmID, err)
		m.cleanupResources(ctx, vm, true)
		return nil, err
	}

	if err := machine.Start(ctx); err != nil {
		m.markError(vmID, err)
		m.cleanupResources(ctx, vm, true)
		return nil, err
	}

	m.mu.Lock()
	m.machines[vmID] = machine
	m.mu.Unlock()

	vm.State = store.VMStateRunning
	vm.UpdatedAt = time.Now().UTC()
	_ = m.db.UpdateVMState(vmID, store.VMStateRunning)

	slog.Info("vm started", "id", vmID, "ip", guestIP)
	return vm, nil
}

// ---------- Stop ----------

func (m *Manager) Stop(ctx context.Context, id string) (*store.VM, error) {
	vm, err := m.db.GetVM(id)
	if err != nil {
		return nil, err
	}
	if vm == nil {
		return nil, fmt.Errorf("vm not found: %s", id)
	}

	_ = m.db.UpdateVMState(id, store.VMStateStopping)

	if err := m.shutdownMachine(ctx, id); err != nil {
		m.markError(id, err)
		return vm, err
	}

	_ = m.db.UpdateVMState(id, store.VMStateStopped)
	vm.State = store.VMStateStopped
	vm.UpdatedAt = time.Now().UTC()

	slog.Info("vm stopped", "id", id)
	return vm, nil
}

// ---------- Delete ----------

func (m *Manager) Delete(ctx context.Context, id string) error {
	vm, err := m.db.GetVM(id)
	if err != nil {
		return err
	}
	if vm == nil {
		return fmt.Errorf("vm not found: %s", id)
	}

	m.cleanupResources(ctx, vm, true)

	m.mu.Lock()
	delete(m.machines, id)
	m.mu.Unlock()

	// Clean up associated proxy routes and snapshots.
	_ = m.db.DeleteRoutesByVM(id)
	_ = m.db.DeleteSnapshotsByVM(id)
	if err := m.db.DeleteVM(id); err != nil {
		return err
	}

	slog.Info("vm deleted", "id", id)
	return nil
}

// ---------- Get / List ----------

func (m *Manager) Get(id string) (*store.VM, bool) {
	vm, err := m.db.GetVM(id)
	if err != nil || vm == nil {
		return nil, false
	}
	return vm, true
}

func (m *Manager) List() []*store.VM {
	vms, err := m.db.ListVMs()
	if err != nil {
		slog.Error("list vms", "error", err)
		return nil
	}
	ptrs := make([]*store.VM, len(vms))
	for i := range vms {
		ptrs[i] = &vms[i]
	}
	return ptrs
}

// ---------- internals ----------

func (m *Manager) ensureBridge(ctx context.Context) error {
	if m.bridgeReady {
		return nil
	}

	if err := EnsureBridge(ctx, m.cfg.BridgeName, m.cfg.BridgeCIDR); err != nil {
		return err
	}

	if err := m.dhcp.Start(); err != nil {
		return fmt.Errorf("start DHCP server: %w", err)
	}

	m.bridgeReady = true
	return nil
}

func (m *Manager) cleanupResources(ctx context.Context, vm *store.VM, releaseIP bool) {
	m.mu.RLock()
	machine := m.machines[vm.ID]
	m.mu.RUnlock()

	if machine != nil {
		_ = machine.StopVMM()
		_ = machine.Wait(ctx)
	}

	if vm.TapName != "" {
		_ = DeleteTap(ctx, vm.TapName)
	}

	if vm.GuestMAC != "" {
		_ = m.dhcp.RemoveHost(vm.GuestMAC)
	}

	if releaseIP && vm.GuestIP != "" {
		if ip := net.ParseIP(vm.GuestIP); ip != nil {
			m.allocator.Release(ip)
		}
	}

	if vm.WorkDir != "" {
		_ = os.RemoveAll(vm.WorkDir)
	}
}

// Shutdown stops long-running resources owned by the manager (e.g. dnsmasq).
func (m *Manager) Shutdown() {
	if m.dhcp != nil {
		m.dhcp.Stop()
	}
}

func (m *Manager) markError(id string, cause error) {
	slog.Error("vm error", "id", id, "error", cause)
	_ = m.db.UpdateVMState(id, store.VMStateError)
}

func (m *Manager) shutdownMachine(ctx context.Context, id string) error {
	m.mu.RLock()
	machine := m.machines[id]
	m.mu.RUnlock()
	if machine == nil {
		return nil // machine handle lost (e.g. after restart); nothing to stop
	}
	return machine.Shutdown(ctx)
}

func clampInt(value, min, max, fallback int) int {
	if value == 0 {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// machineHandle abstracts the Firecracker SDK Machine so the rest of the
// manager doesn't depend on it directly.
type machineHandle interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
	StopVMM() error
	Wait(ctx context.Context) error
	PauseVM(ctx context.Context) error
	CreateSnapshot(ctx context.Context, memFilePath, snapshotPath string) error
}

type machineConfig struct {
	socketPath    string
	kernelPath    string
	kernelArgs    string
	rootfsPath    string
	vmID          string
	vcpuCount     int64
	memMib        int64
	tapName       string
	macAddr       string
	logDir        string
	cloudInitPath string // NoCloud seed disk (attached as /dev/vdb)
}
