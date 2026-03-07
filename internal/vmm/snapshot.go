package vmm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/util"
	"golang.org/x/sync/errgroup"
)

// snapshotVMConfig is the JSON blob persisted alongside each snapshot so
// we have everything we need to restore the VM later.
type snapshotVMConfig struct {
	ImageID    string `json:"image_id"`
	TemplateID string `json:"template_id,omitempty"`
	VCPUCount  int    `json:"vcpu_count"`
	MemMib     int    `json:"mem_mib"`
	GuestIP    string `json:"guest_ip"`
	GuestMAC   string `json:"guest_mac"`
	TapName    string `json:"tap_name"`
	BootArgs   string `json:"boot_args"`
	KernelPath string `json:"kernel_path"`
	RootfsPath string `json:"rootfs_path"`
}

// Snapshot pauses a running VM, creates a Firecracker snapshot, uploads
// the artefacts to S3, and transitions the VM to "snapshotted" state.
func (m *Manager) Snapshot(ctx context.Context, vmID string) (*store.Snapshot, error) {
	if m.s3 == nil {
		return nil, fmt.Errorf("snapshot storage (S3) is not configured")
	}

	vm, err := m.db.GetVM(vmID)
	if err != nil {
		return nil, err
	}
	if vm == nil {
		return nil, fmt.Errorf("vm not found: %s", vmID)
	}
	if vm.State != store.VMStateRunning {
		return nil, fmt.Errorf("vm must be running to snapshot (current state: %s)", vm.State)
	}

	m.mu.RLock()
	machine := m.machines[vmID]
	m.mu.RUnlock()
	if machine == nil {
		return nil, fmt.Errorf("no machine handle for vm %s", vmID)
	}

	// Look up the image so we can persist paths in the snapshot config.
	image, err := m.db.GetImage(vm.ImageID)
	if err != nil || image == nil {
		return nil, fmt.Errorf("image not found for snapshot: %s", vm.ImageID)
	}

	snapID := util.RandomID("snap")
	slog.Info("creating snapshot", "vm", vmID, "snapshot", snapID)

	// Local paths for the snapshot artefacts.
	snapDir := filepath.Join(vm.WorkDir, "snapshots", snapID)
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return nil, fmt.Errorf("create snapshot dir: %w", err)
	}

	memPath := filepath.Join(snapDir, "memory")
	statePath := filepath.Join(snapDir, "vmstate")

	// 1. Pause the VM.
	if err := machine.PauseVM(ctx); err != nil {
		return nil, fmt.Errorf("pause vm: %w", err)
	}

	// 2. Create the Firecracker snapshot.
	if err := machine.CreateSnapshot(ctx, memPath, statePath); err != nil {
		return nil, fmt.Errorf("create snapshot: %w", err)
	}

	// 3. Persist only the writable overlay image; the base rootfs remains the
	// shared image referenced by RootfsPath in the snapshot config.
	overlayPath := filepath.Join(vm.WorkDir, "overlay.ext4")

	// 4. Upload artefacts to S3 in parallel (compressed where beneficial).
	prefix := fmt.Sprintf("snapshots/%s/%s", vmID, snapID)
	stateKey := prefix + "/vmstate"
	memKey := prefix + "/memory.gz"
	diskKey := prefix + "/overlay.ext4.gz"

	g, uploadCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return m.s3.UploadFile(uploadCtx, stateKey, statePath) })
	g.Go(func() error { return m.s3.UploadFileCompressed(uploadCtx, memKey, memPath) })
	g.Go(func() error { return m.s3.UploadFileCompressed(uploadCtx, diskKey, overlayPath) })
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Calculate total size of local artefacts before compression.
	var totalSize int64
	for _, p := range []string{statePath, memPath, overlayPath} {
		if fi, err := os.Stat(p); err == nil {
			totalSize += fi.Size()
		}
	}

	// 4. Build and persist the snapshot record.
	cfgJSON, _ := json.Marshal(snapshotVMConfig{
		ImageID:    vm.ImageID,
		TemplateID: vm.TemplateID,
		VCPUCount:  vm.VCPUCount,
		MemMib:     vm.MemMib,
		GuestIP:    vm.GuestIP,
		GuestMAC:   vm.GuestMAC,
		TapName:    vm.TapName,
		BootArgs:   image.BootArgs,
		KernelPath: image.KernelPath,
		RootfsPath: image.RootfsPath,
	})

	snap := &store.Snapshot{
		ID:        snapID,
		VMID:      vmID,
		StateKey:  stateKey,
		MemoryKey: memKey,
		DiskKey:   diskKey,
		VMConfig:  string(cfgJSON),
		SizeBytes: totalSize,
		CreatedAt: time.Now().UTC(),
	}

	if err := m.db.CreateSnapshot(*snap); err != nil {
		return nil, err
	}

	// 5. Clean up local resources and mark the VM as snapshotted.
	m.cleanupResources(ctx, vm, cleanupOpts{releaseIP: false, removeWorkDir: false})
	m.reserveSSHPort(vm.ID, vm.SSHPort)
	m.mu.Lock()
	delete(m.machines, vmID)
	m.mu.Unlock()

	_ = m.db.UpdateVMState(vmID, store.VMStateSnapshotted)
	slog.Info("snapshot created", "vm", vmID, "snapshot", snapID)

	return snap, nil
}

// Restore brings a snapshotted VM back to life by downloading its snapshot
// from S3, re-establishing networking, and loading the snapshot into a new
// Firecracker process.
func (m *Manager) Restore(ctx context.Context, vmID string) (*store.VM, error) {
	if m.s3 == nil {
		return nil, fmt.Errorf("snapshot storage (S3) is not configured")
	}

	vm, err := m.db.GetVM(vmID)
	if err != nil {
		return nil, err
	}
	if vm == nil {
		return nil, fmt.Errorf("vm not found: %s", vmID)
	}
	if vm.State != store.VMStateSnapshotted {
		return nil, fmt.Errorf("vm must be snapshotted to restore (current state: %s)", vm.State)
	}

	snap, err := m.db.GetLatestSnapshot(vmID)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, fmt.Errorf("no snapshot found for vm %s", vmID)
	}

	// Parse the stored config.
	var cfg snapshotVMConfig
	if err := json.Unmarshal([]byte(snap.VMConfig), &cfg); err != nil {
		return nil, fmt.Errorf("decode snapshot config: %w", err)
	}

	slog.Info("restoring vm from snapshot", "vm", vmID, "snapshot", snap.ID)

	// Prepare a work directory.
	vmDir := filepath.Join(m.cfg.DataDir, "vms", vmID)
	if err := os.MkdirAll(vmDir, 0o755); err != nil {
		return nil, err
	}

	socketPath := filepath.Join(vmDir, "firecracker.socket")
	_ = os.RemoveAll(socketPath)

	// Download snapshot artefacts (memory and overlay are gzip-compressed).
	memPath := filepath.Join(vmDir, "memory")
	statePath := filepath.Join(vmDir, "vmstate")
	overlayPath := filepath.Join(vmDir, "overlay.ext4")

	g, dlCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return m.s3.DownloadCompressed(dlCtx, snap.MemoryKey, memPath) })
	g.Go(func() error { return m.s3.Download(dlCtx, snap.StateKey, statePath) })
	g.Go(func() error { return m.s3.DownloadCompressed(dlCtx, snap.DiskKey, overlayPath) })
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Re-establish networking.
	tapName := fmt.Sprintf("fctap-%s", vmID[:8])
	if vm.EnableNetwork && cfg.GuestMAC != "" {
		if err := m.ensureBridge(ctx); err != nil {
			return nil, err
		}
		// Delete any leftover TAP from a previous failed restore attempt.
		_ = DeleteTap(ctx, tapName)
		if err := CreateTap(ctx, tapName, m.cfg.BridgeName); err != nil {
			return nil, err
		}
		if err := m.dhcp.AddHost(cfg.GuestMAC, cfg.GuestIP); err != nil {
			_ = DeleteTap(ctx, tapName)
			return nil, err
		}
	}

	// Start a new Firecracker instance from the snapshot.
	machine, err := newMachineFromSnapshot(ctx, m.cfg.FirecrackerBinary, machineConfig{
		socketPath: socketPath,
		kernelPath: cfg.KernelPath,
		rootfsPath: cfg.RootfsPath,
		overlayPath: overlayPath,
		vmID:       vmID,
		vcpuCount:  int64(cfg.VCPUCount),
		memMib:     int64(cfg.MemMib),
		tapName:    tapName,
		macAddr:    cfg.GuestMAC,
		logDir:     vmDir,
	}, memPath, statePath)
	if err != nil {
		_ = DeleteTap(ctx, tapName)
		return nil, fmt.Errorf("create machine from snapshot: %w", err)
	}

	if err := machine.Start(context.Background()); err != nil {
		_ = DeleteTap(ctx, tapName)
		return nil, fmt.Errorf("start restored machine: %w", err)
	}

	if vm.SSHPort > 0 && vm.GuestIP != "" {
		if err := setupSSHForward(ctx, vm.SSHPort, vm.GuestIP, m.cfg.SSHAllowedCIDR); err != nil {
			_ = machine.StopVMM()
			_ = machine.Wait(ctx)
			_ = DeleteTap(ctx, tapName)
			return nil, fmt.Errorf("setup ssh forward after restore: %w", err)
		}
		m.reserveSSHPort(vm.ID, vm.SSHPort)
	}

	m.mu.Lock()
	m.machines[vmID] = machine
	m.mu.Unlock()

	_ = m.db.UpdateVMState(vmID, store.VMStateRunning)
	vm.State = store.VMStateRunning
	vm.UpdatedAt = time.Now().UTC()

	slog.Info("vm restored", "vm", vmID)
	return vm, nil
}
