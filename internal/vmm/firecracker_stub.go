//go:build !linux

package vmm

import (
	"context"
	"fmt"
)

func newMachine(_ context.Context, _ string, _ machineConfig) (machineHandle, error) {
	return nil, fmt.Errorf("firecracker SDK is linux-only; run on a Linux host for VM operations")
}

func newMachineFromSnapshot(_ context.Context, _ string, _ machineConfig, _, _ string) (machineHandle, error) {
	return nil, fmt.Errorf("firecracker SDK is linux-only; run on a Linux host for VM operations")
}
