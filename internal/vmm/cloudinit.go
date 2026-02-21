package vmm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const defaultUserData = "#cloud-config\n"

// InjectCloudInitSeed writes NoCloud seed files (meta-data, user-data,
// network-config) directly into the per-VM rootfs at the well-known path
// /var/lib/cloud/seed/nocloud/. Cloud-init checks this directory before
// scanning for labeled disks, so no external cidata drive is needed.
// This avoids filesystem-format compatibility issues with minimal Firecracker
// kernels that lack vfat/iso9660 support.
//
// The rootfs must be an ext4 image file (not currently mounted by a VM).
func InjectCloudInitSeed(ctx context.Context, rootfsPath, vmDir, instanceID, macAddr, userData string) error {
	srcDir := filepath.Join(vmDir, "cidata-src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return fmt.Errorf("create seed source dir: %w", err)
	}
	defer os.RemoveAll(srcDir)

	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceID, instanceID)
	if err := os.WriteFile(filepath.Join(srcDir, "meta-data"), []byte(metaData), 0o644); err != nil {
		return fmt.Errorf("write meta-data: %w", err)
	}

	if userData == "" {
		userData = defaultUserData
	}
	if err := os.WriteFile(filepath.Join(srcDir, "user-data"), []byte(userData), 0o644); err != nil {
		return fmt.Errorf("write user-data: %w", err)
	}

	networkConfig := fmt.Sprintf(
		"version: 2\nethernets:\n  eth0:\n    match:\n      macaddress: \"%s\"\n    dhcp4: true\n",
		macAddr,
	)
	if err := os.WriteFile(filepath.Join(srcDir, "network-config"), []byte(networkConfig), 0o644); err != nil {
		return fmt.Errorf("write network-config: %w", err)
	}

	mountDir := filepath.Join(vmDir, "rootfs-mount")
	if err := os.MkdirAll(mountDir, 0o755); err != nil {
		return fmt.Errorf("create mount dir: %w", err)
	}
	defer os.RemoveAll(mountDir)

	if err := run(ctx, "mount", "-o", "loop", rootfsPath, mountDir); err != nil {
		return fmt.Errorf("mount rootfs: %w", err)
	}
	defer run(context.Background(), "umount", mountDir)

	seedDir := filepath.Join(mountDir, "var", "lib", "cloud", "seed", "nocloud")
	if err := os.MkdirAll(seedDir, 0o755); err != nil {
		return fmt.Errorf("create seed dir in rootfs: %w", err)
	}

	for _, name := range []string{"meta-data", "user-data", "network-config"} {
		data, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(seedDir, name), data, 0o644); err != nil {
			return fmt.Errorf("write %s to rootfs: %w", name, err)
		}
	}

	return nil
}
