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

	sshMu    sync.Mutex
	sshPorts map[int]string // host ssh port -> vm id
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
		sshPorts:  make(map[int]string),
	}

	// Validate SSH CIDR and seed in-use SSH ports from persisted VMs.
	if _, _, err := net.ParseCIDR(cfg.SSHAllowedCIDR); err != nil {
		return nil, fmt.Errorf("invalid HATCH_SSH_ALLOWED_CIDR: %w", err)
	}
	if cfg.SSHPortMin <= 0 || cfg.SSHPortMax > 65535 || cfg.SSHPortMin > cfg.SSHPortMax {
		return nil, fmt.Errorf("invalid SSH port range: %d-%d", cfg.SSHPortMin, cfg.SSHPortMax)
	}
	if existingVMs, err := db.ListVMs(); err == nil {
		for i := range existingVMs {
			if p := existingVMs[i].SSHPort; p > 0 {
				m.sshPorts[p] = existingVMs[i].ID
			}
		}
	}

	// On startup, mark any VMs left in a transient state (from a prior crash)
	// so they don't appear as "running" when no machine handle exists.
	if stale, err := db.MarkStaleVMs(); err != nil {
		slog.Warn("failed to mark stale VMs", "error", err)
	} else if stale > 0 {
		slog.Warn("marked stale VMs from previous run", "count", stale)
	}

	m.reconcileOnStartup(cfg)

	return m, nil
}

// reconcileOnStartup cleans up all host-side resources from a previous
// process run. After a restart no Firecracker processes are alive, so every
// TAP device and iptables SSH-forward rule on the host is stale. Cleaning
// them here means rebuilds/restarts always begin from a known-good state;
// resources are re-created when VMs are started or restored.
func (m *Manager) reconcileOnStartup(cfg config.Config) {
	ctx := context.Background()

	// 1. Flush ALL iptables SSH rules by scanning the chains. This is more
	//    robust than per-VM teardown: it catches orphaned rules from VMs that
	//    were deleted from the DB, and rules created with a different CIDR.
	if removed := flushStaleSSHRules(ctx, cfg.SSHPortMin, cfg.SSHPortMax); removed > 0 {
		slog.Info("cleaned up stale iptables SSH rules", "count", removed)
	}

	// 2. Remove ALL fctap-* TAP devices. No VMs are running after a restart,
	//    so every TAP on the host is orphaned.
	if removed, err := ReconcileTaps(ctx, "fctap-", nil); err != nil {
		slog.Warn("tap reconciliation failed", "error", err)
	} else if removed > 0 {
		slog.Info("cleaned up stale tap devices", "count", removed)
	}

	// 3. Delete the bridge so it's re-created fresh by ensureBridge.
	//    This avoids stale ARP entries and bridge-level state.
	_ = run(ctx, "ip", "link", "del", cfg.BridgeName)
	m.bridgeReady = false

	// 4. Ensure /dev/loopN device nodes exist. The container gets a fresh
	//    /dev tmpfs on each start, and after a host reboot the nodes may
	//    not be populated even though the loop kernel module is loaded.
	for i := 0; i < 8; i++ {
		_ = run(ctx, "mknod", "-m", "0660", fmt.Sprintf("/dev/loop%d", i), "b", "7", fmt.Sprintf("%d", i))
	}

	slog.Info("startup reconciliation complete")
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
	if opts.EnableNetwork {
		sshPort, err := m.allocateSSHPort(vmID)
		if err != nil {
			_ = os.RemoveAll(vmDir)
			return nil, err
		}
		vm.SSHPort = sshPort
	}

	// Persist the VM record before doing anything else so it's visible even
	// if creation fails midway (state will be marked "error").
	if err := m.db.CreateVM(*vm); err != nil {
		m.releaseSSHPort(vm.SSHPort)
		_ = os.RemoveAll(vmDir)
		return nil, err
	}

	var tapName, guestIP, macAddr string

	if opts.EnableNetwork {
		if err := m.ensureBridge(ctx); err != nil {
			m.markError(vmID, err)
			m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})
			return nil, err
		}

		tapName = fmt.Sprintf("fctap-%s", vmID[:8])
		if err := CreateTap(ctx, tapName, m.cfg.BridgeName); err != nil {
			m.markError(vmID, err)
			m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})
			return nil, err
		}
		vm.TapName = tapName

		if opts.GuestIP != "" {
			guestIP = opts.GuestIP
		} else {
			ip, err := m.allocator.Allocate()
			if err != nil {
				m.markError(vmID, err)
				m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})
				return nil, err
			}
			guestIP = ip.String()
		}
		vm.GuestIP = guestIP

		macAddr = opts.GuestMAC
		if macAddr == "" {
			macAddr = randomMAC()
		}
		vm.GuestMAC = macAddr

		if err := m.dhcp.AddHost(macAddr, guestIP); err != nil {
			m.markError(vmID, err)
			m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})
			return nil, fmt.Errorf("add DHCP reservation: %w", err)
		}

		if vm.SSHPort > 0 {
			if err := setupSSHForward(ctx, vm.SSHPort, guestIP, m.cfg.SSHAllowedCIDR); err != nil {
				m.markError(vmID, err)
				m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})
				return nil, fmt.Errorf("setup ssh forward: %w", err)
			}
		}

		// Update the DB with network details.
		if err := m.db.UpdateVMNetwork(vmID, tapName, guestIP, macAddr); err != nil {
			slog.Warn("failed to persist network info", "vm", vmID, "error", err)
		}
	}

	// Copy the base rootfs so each VM has its own writable disk image.
	vmRootfs := filepath.Join(vmDir, "rootfs.ext4")
	if err := run(ctx, "cp", "--sparse=always", "--reflink=auto", image.RootfsPath, vmRootfs); err != nil {
		m.markError(vmID, err)
		m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})
		return nil, fmt.Errorf("copy rootfs for vm: %w", err)
	}

	if err := InjectCloudInitSeed(ctx, vmRootfs, vmDir, vmID, macAddr, opts.UserData); err != nil {
		m.markError(vmID, err)
		m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})
		return nil, fmt.Errorf("inject cloud-init seed: %w", err)
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
		rootfsPath:    vmRootfs,
		vmID:          vmID,
		vcpuCount:     int64(vm.VCPUCount),
		memMib:        int64(vm.MemMib),
		tapName:       tapName,
		macAddr:       macAddr,
		logDir:        vmDir,
	})
	if err != nil {
		m.markError(vmID, err)
		m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})
		return nil, err
	}

	if err := machine.Start(context.Background()); err != nil {
		m.markError(vmID, err)
		m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})
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
	teardownSSHForward(ctx, vm.SSHPort, vm.GuestIP, m.cfg.SSHAllowedCIDR)

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

	m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: true, removeWorkDir: true})

	m.mu.Lock()
	delete(m.machines, id)
	m.mu.Unlock()

	// Clean up associated proxy routes and snapshots.
	_ = m.db.DeleteRoutesByVM(id)
	snaps, err := m.db.ListSnapshots(id)
	if err != nil {
		return fmt.Errorf("list snapshots for vm delete: %w", err)
	}
	if len(snaps) > 0 {
		if m.s3 == nil {
			return fmt.Errorf("cannot delete %d snapshot(s) from S3: snapshot storage is not configured", len(snaps))
		}
		for _, snap := range snaps {
			for _, key := range []string{snap.StateKey, snap.MemoryKey, snap.DiskKey} {
				if key == "" {
					continue
				}
				if err := m.s3.Delete(ctx, key); err != nil {
					return fmt.Errorf("delete snapshot object %q: %w", key, err)
				}
			}
		}
	}
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

type cleanupOpts struct {
	releaseIP     bool
	removeWorkDir bool
}

func (m *Manager) cleanupResources(ctx context.Context, vm *store.VM, opts cleanupOpts) {
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

	teardownSSHForward(ctx, vm.SSHPort, vm.GuestIP, m.cfg.SSHAllowedCIDR)
	m.releaseSSHPort(vm.SSHPort)

	if opts.releaseIP && vm.GuestIP != "" {
		if ip := net.ParseIP(vm.GuestIP); ip != nil {
			m.allocator.Release(ip)
		}
	}

	if opts.removeWorkDir && vm.WorkDir != "" {
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

// MarkError transitions a VM to the error state. Exported for use by the
// proxy layer when a wake-on-request restore fails.
func (m *Manager) MarkError(id string, cause error) {
	m.markError(id, cause)
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

func (m *Manager) allocateSSHPort(vmID string) (int, error) {
	m.sshMu.Lock()
	defer m.sshMu.Unlock()

	for p := m.cfg.SSHPortMin; p <= m.cfg.SSHPortMax; p++ {
		if _, used := m.sshPorts[p]; used {
			continue
		}
		if portInUse(p) {
			continue
		}
		m.sshPorts[p] = vmID
		return p, nil
	}
	return 0, fmt.Errorf("no available ssh ports in range %d-%d", m.cfg.SSHPortMin, m.cfg.SSHPortMax)
}

func (m *Manager) releaseSSHPort(port int) {
	if port <= 0 {
		return
	}
	m.sshMu.Lock()
	defer m.sshMu.Unlock()
	delete(m.sshPorts, port)
}

func (m *Manager) reserveSSHPort(vmID string, port int) {
	if port <= 0 {
		return
	}
	m.sshMu.Lock()
	defer m.sshMu.Unlock()
	m.sshPorts[port] = vmID
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
