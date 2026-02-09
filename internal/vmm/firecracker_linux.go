//go:build linux

package vmm

import (
	"context"
	"os/exec"
	"path/filepath"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	fcmodels "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

func newMachine(ctx context.Context, binaryPath string, cfg machineConfig) (machineHandle, error) {
	drives := []fcmodels.Drive{
		{
			DriveID:      firecracker.String("rootfs"),
			PathOnHost:   firecracker.String(cfg.rootfsPath),
			IsRootDevice: firecracker.Bool(true),
			IsReadOnly:   firecracker.Bool(false),
		},
	}

	// Attach the cloud-init NoCloud seed disk so the guest can auto-configure
	// networking via DHCP. cloud-init detects it by the "cidata" filesystem label.
	if cfg.cloudInitPath != "" {
		drives = append(drives, fcmodels.Drive{
			DriveID:      firecracker.String("cidata"),
			PathOnHost:   firecracker.String(cfg.cloudInitPath),
			IsRootDevice: firecracker.Bool(false),
			IsReadOnly:   firecracker.Bool(true),
		})
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
			VcpuCount:  firecracker.Int64(cfg.vcpuCount),
			MemSizeMib: firecracker.Int64(cfg.memMib),
			Smt:        firecracker.Bool(false),
		},
		VMID: cfg.vmID,
	}

	if cfg.logDir != "" {
		fcCfg.LogPath = filepath.Join(cfg.logDir, "firecracker.log")
		fcCfg.MetricsPath = filepath.Join(cfg.logDir, "firecracker.metrics")
	}

	cmd := exec.CommandContext(ctx, binaryPath, "--api-sock", cfg.socketPath)
	return firecracker.NewMachine(ctx, fcCfg, firecracker.WithProcessRunner(cmd))
}
