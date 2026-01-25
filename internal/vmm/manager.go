package vmm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ThakerKush/Hatch/internal/config"
	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/util"
)

type VMState string

const (
	VMStateStarting VMState = "starting"
	VMStateRunning  VMState = "running"
	VMStateStopping VMState = "stopping"
	VMStateStopped  VMState = "stopped"
	VMStateError    VMState = "error"
)

type VM struct {
	ID         string    `json:"id"`
	ImageID    string    `json:"image_id"`
	State      VMState   `json:"state"`
	SocketPath string    `json:"socket_path"`
	WorkDir    string    `json:"-"`
	TapName    string    `json:"tap_name,omitempty"`
	GuestIP    string    `json:"guest_ip,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CreateOptions struct {
	ImageID        string
	VCPUCount      int
	MemMib         int
	BootArgs       string
	EnableNetwork  bool
	GuestIP        string
	GuestMAC       string
	InstanceSocket string
}

type Manager struct {
	cfg         config.Config
	images      *store.ImageStore
	allocator   *IPAllocator
	mu          sync.RWMutex
	vms         map[string]*VM
	machines    map[string]machineHandle
	bridgeReady bool
}

func NewManager(cfg config.Config, images *store.ImageStore) *Manager {
	allocator := NewIPAllocator(cfg.BridgeCIDR)
	return &Manager{
		cfg:       cfg,
		images:    images,
		allocator: allocator,
		vms:       make(map[string]*VM),
		machines:  make(map[string]machineHandle),
	}
}

func (m *Manager) CreateAndStart(ctx context.Context, opts CreateOptions) (*VM, error) {
	image, ok := m.images.Get(opts.ImageID)
	if !ok {
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

	if err := os.RemoveAll(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	var tapName string
	var guestIP string
	if opts.EnableNetwork {
		if err := m.ensureBridge(ctx); err != nil {
			return nil, err
		}

		tapName = fmt.Sprintf("fctap-%s", vmID[:8])
		if err := CreateTap(ctx, tapName, m.cfg.BridgeName); err != nil {
			return nil, err
		}

		if opts.GuestIP != "" {
			guestIP = opts.GuestIP
		} else {
			ip, err := m.allocator.Allocate()
			if err != nil {
				_ = DeleteTap(ctx, tapName)
				return nil, err
			}
			guestIP = ip.String()
		}
	}

	bootArgs := image.BootArgs
	if opts.BootArgs != "" {
		bootArgs = opts.BootArgs
	}
	if bootArgs == "" {
		bootArgs = m.cfg.DefaultBootArgs
	}

	vm := &VM{
		ID:         vmID,
		ImageID:    image.ID,
		State:      VMStateStarting,
		SocketPath: socketPath,
		WorkDir:    vmDir,
		TapName:    tapName,
		GuestIP:    guestIP,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	m.mu.Lock()
	m.vms[vm.ID] = vm
	m.mu.Unlock()

	vcpus := int64(clampInt(opts.VCPUCount, 1, 32, m.cfg.DefaultVCPU))
	mem := int64(clampInt(opts.MemMib, 128, 65536, m.cfg.DefaultMemMib))

	var macAddr string
	if opts.EnableNetwork && tapName != "" {
		macAddr = opts.GuestMAC
		if macAddr == "" {
			macAddr = randomMAC()
		}
	}

	machine, err := newMachine(ctx, m.cfg.FirecrackerBinary, machineConfig{
		socketPath: socketPath,
		kernelPath: image.KernelPath,
		kernelArgs: bootArgs,
		rootfsPath: image.RootfsPath,
		vmID:       vmID,
		vcpuCount:  vcpus,
		memMib:     mem,
		tapName:    tapName,
		macAddr:    macAddr,
		logDir:     vmDir,
	})
	if err != nil {
		m.markError(vm.ID, err)
		m.cleanupResources(ctx, vm, true)
		return nil, err
	}

	if err := machine.Start(ctx); err != nil {
		m.markError(vm.ID, err)
		m.cleanupResources(ctx, vm, true)
		return nil, err
	}

	m.mu.Lock()
	m.machines[vm.ID] = machine
	m.mu.Unlock()

	m.mu.Lock()
	vm.State = VMStateRunning
	vm.UpdatedAt = time.Now().UTC()
	m.mu.Unlock()

	return vm, nil
}

func (m *Manager) Stop(ctx context.Context, id string) (*VM, error) {
	vm, ok := m.Get(id)
	if !ok {
		return nil, fmt.Errorf("vm not found: %s", id)
	}

	m.mu.Lock()
	vm.State = VMStateStopping
	vm.UpdatedAt = time.Now().UTC()
	m.mu.Unlock()

	if err := m.shutdownMachine(ctx, vm.ID); err != nil {
		m.markError(vm.ID, err)
		return vm, err
	}

	m.mu.Lock()
	vm.State = VMStateStopped
	vm.UpdatedAt = time.Now().UTC()
	m.mu.Unlock()

	return vm, nil
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	vm, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("vm not found: %s", id)
	}

	m.cleanupResources(ctx, vm, true)

	m.mu.Lock()
	delete(m.vms, id)
	delete(m.machines, id)
	m.mu.Unlock()

	return nil
}

func (m *Manager) Get(id string) (*VM, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	vm, ok := m.vms[id]
	if !ok {
		return nil, false
	}
	copyVM := *vm
	return &copyVM, true
}

func (m *Manager) List() []*VM {
	m.mu.RLock()
	defer m.mu.RUnlock()
	vms := make([]*VM, 0, len(m.vms))
	for _, vm := range m.vms {
		copyVM := *vm
		vms = append(vms, &copyVM)
	}
	return vms
}

func (m *Manager) ensureBridge(ctx context.Context) error {
	if m.bridgeReady {
		return nil
	}

	if err := EnsureBridge(ctx, m.cfg.BridgeName, m.cfg.BridgeCIDR); err != nil {
		return err
	}
	m.bridgeReady = true
	return nil
}

func (m *Manager) cleanupResources(ctx context.Context, vm *VM, releaseIP bool) {
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

	if releaseIP && vm.GuestIP != "" {
		if ip := net.ParseIP(vm.GuestIP); ip != nil {
			m.allocator.Release(ip)
		}
	}

	if vm.WorkDir != "" {
		_ = os.RemoveAll(vm.WorkDir)
	}
}

func (m *Manager) markError(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if vm, ok := m.vms[id]; ok {
		vm.State = VMStateError
		vm.UpdatedAt = time.Now().UTC()
	}
}

func (m *Manager) shutdownMachine(ctx context.Context, id string) error {
	m.mu.RLock()
	machine := m.machines[id]
	m.mu.RUnlock()
	if machine == nil {
		return fmt.Errorf("vm not found: %s", id)
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

type machineHandle interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
	StopVMM() error
	Wait(ctx context.Context) error
}

type machineConfig struct {
	socketPath string
	kernelPath string
	kernelArgs string
	rootfsPath string
	vmID       string
	vcpuCount  int64
	memMib     int64
	tapName    string
	macAddr    string
	logDir     string
}
