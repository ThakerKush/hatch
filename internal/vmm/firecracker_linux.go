//go:build linux

package vmm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	fcmodels "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	ops "github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
)

// fcMachine wraps the SDK Machine to satisfy machineHandle including snapshot methods.
type fcMachine struct {
	*firecracker.Machine
}

func (f *fcMachine) PauseVM(ctx context.Context) error {
	return f.Machine.PauseVM(ctx)
}

func (f *fcMachine) CreateSnapshot(ctx context.Context, memFilePath, snapshotPath string) error {
	return f.Machine.CreateSnapshot(ctx, memFilePath, snapshotPath,
		func(params *ops.CreateSnapshotParams) {
			params.Body.SnapshotType = fcmodels.SnapshotCreateParamsSnapshotTypeDiff
		},
	)
}

func newMachine(ctx context.Context, binaryPath string, cfg machineConfig) (machineHandle, error) {
	if cfg.overlayPath == "" {
		return nil, fmt.Errorf("overlay path is required")
	}

	drives := []fcmodels.Drive{
		{
			DriveID:      firecracker.String("rootfs"),
			PathOnHost:   firecracker.String(cfg.rootfsPath),
			IsRootDevice: firecracker.Bool(true),
			IsReadOnly:   firecracker.Bool(true),
		},
		{
			DriveID:      firecracker.String("overlay"),
			PathOnHost:   firecracker.String(cfg.overlayPath),
			IsRootDevice: firecracker.Bool(false),
			IsReadOnly:   firecracker.Bool(false),
		},
	}

	var networkInterfaces firecracker.NetworkInterfaces
	if cfg.tapName != "" {
		networkInterfaces = firecracker.NetworkInterfaces{
			{
				StaticConfiguration: &firecracker.StaticNetworkConfiguration{
					HostDevName: cfg.tapName,
					MacAddress:  cfg.macAddr,
				},
			},
		}
	}

	fcCfg := firecracker.Config{
		SocketPath:        cfg.socketPath,
		KernelImagePath:   cfg.kernelPath,
		KernelArgs:        cfg.kernelArgs,
		Drives:            drives,
		NetworkInterfaces: networkInterfaces,
		MachineCfg: fcmodels.MachineConfiguration{
			VcpuCount:       firecracker.Int64(cfg.vcpuCount),
			MemSizeMib:      firecracker.Int64(cfg.memMib),
			Smt:             firecracker.Bool(false),
			TrackDirtyPages: true,
		},
		VMID: cfg.vmID,
	}

	if cfg.logDir != "" {
		fcCfg.LogPath = filepath.Join(cfg.logDir, "firecracker.log")
		fcCfg.MetricsPath = filepath.Join(cfg.logDir, "firecracker.metrics")
	}

	// Use a background context for both the process and the SDK so the VM
	// outlives the HTTP request that triggered creation. The VM lifecycle is
	// managed explicitly via machineHandle.Shutdown / StopVMM.
	vmCtx := context.Background()
	cmd := exec.Command(binaryPath, "--api-sock", cfg.socketPath)
	if cfg.logDir != "" {
		if f, err := os.Create(filepath.Join(cfg.logDir, "serial.log")); err == nil {
			cmd.Stdout = f
			cmd.Stderr = f
		}
	}
	m, err := firecracker.NewMachine(vmCtx, fcCfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		return nil, err
	}
	return &fcMachine{Machine: m}, nil
}

// newMachineFromSnapshot creates a Firecracker machine that resumes from a
// previously-created snapshot (memory + vmstate files).
func newMachineFromSnapshot(ctx context.Context, binaryPath string, cfg machineConfig, memPath, statePath string) (machineHandle, error) {
	if cfg.overlayPath == "" {
		return nil, fmt.Errorf("overlay path is required")
	}

	drives := []fcmodels.Drive{
		{
			DriveID:      firecracker.String("rootfs"),
			PathOnHost:   firecracker.String(cfg.rootfsPath),
			IsRootDevice: firecracker.Bool(true),
			IsReadOnly:   firecracker.Bool(true),
		},
		{
			DriveID:      firecracker.String("overlay"),
			PathOnHost:   firecracker.String(cfg.overlayPath),
			IsRootDevice: firecracker.Bool(false),
			IsReadOnly:   firecracker.Bool(false),
		},
	}

	var networkInterfaces firecracker.NetworkInterfaces
	if cfg.tapName != "" {
		networkInterfaces = firecracker.NetworkInterfaces{
			{
				StaticConfiguration: &firecracker.StaticNetworkConfiguration{
					HostDevName: cfg.tapName,
					MacAddress:  cfg.macAddr,
				},
			},
		}
	}

	fcCfg := firecracker.Config{
		SocketPath:        cfg.socketPath,
		KernelImagePath:   cfg.kernelPath,
		Drives:            drives,
		NetworkInterfaces: networkInterfaces,
		MachineCfg: fcmodels.MachineConfiguration{
			VcpuCount:  firecracker.Int64(cfg.vcpuCount),
			MemSizeMib: firecracker.Int64(cfg.memMib),
			Smt:        firecracker.Bool(false),
		},
		Snapshot: firecracker.SnapshotConfig{
			MemFilePath:         memPath,
			SnapshotPath:        statePath,
			EnableDiffSnapshots: true,
			ResumeVM:            true,
		},
		VMID: cfg.vmID,
	}

	if cfg.logDir != "" {
		fcCfg.LogPath = filepath.Join(cfg.logDir, "firecracker.log")
		fcCfg.MetricsPath = filepath.Join(cfg.logDir, "firecracker.metrics")
	}

	vmCtx := context.Background()
	cmd := exec.Command(binaryPath, "--api-sock", cfg.socketPath)
	if cfg.logDir != "" {
		if f, err := os.Create(filepath.Join(cfg.logDir, "serial.log")); err == nil {
			cmd.Stdout = f
			cmd.Stderr = f
		}
	}

	m, err := firecracker.NewMachine(vmCtx, fcCfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		return nil, err
	}

	// NewMachine always installs defaultHandlers which target a fresh boot:
	// CreateMachine → CreateBootSource → AttachDrives → CreateNetworkInterfaces
	// → (no LoadSnapshot).  startInstance() is a no-op when Snapshot is set,
	// so the VM is never actually loaded or started.
	//
	// Override with the correct chain: start the VMM, then load the snapshot.
	// Drives and network interfaces must NOT be configured via the API before
	// snapshot load — Firecracker reads those from the vmstate file.
	m.Handlers.Validation = firecracker.HandlerList{}.Append(
		firecracker.NetworkConfigValidationHandler,
		firecracker.LoadSnapshotConfigValidationHandler,
	)
	m.Handlers.FcInit = firecracker.HandlerList{}.Append(
		firecracker.SetupNetworkHandler,
		firecracker.StartVMMHandler,
		firecracker.CreateLogFilesHandler,
		firecracker.BootstrapLoggingHandler,
		firecracker.LoadSnapshotHandler,
	)

	return &fcMachine{Machine: m}, nil
}
