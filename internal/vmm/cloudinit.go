package vmm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const defaultUserData = "#cloud-config\n"

// CreateCloudInitDisk builds a NoCloud seed image (ext4 filesystem labelled
// "cidata") containing meta-data, user-data and network-config. cloud-init
// inside the guest auto-detects this disk by its label and applies the
// configuration – in our case, DHCP on the interface matching the given MAC.
//
// Prerequisites on the host: truncate, mkfs.ext4 (e2fsprogs ≥ 1.43 for -d).
func CreateCloudInitDisk(ctx context.Context, vmDir, instanceID, macAddr, userData string) (string, error) {
	// Prepare a temporary source directory that mkfs.ext4 -d will copy into the image.
	srcDir := filepath.Join(vmDir, "cidata-src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return "", fmt.Errorf("create cidata source dir: %w", err)
	}
	defer os.RemoveAll(srcDir)

	// ── meta-data ────────────────────────────────────────────────────────
	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceID, instanceID)
	if err := os.WriteFile(filepath.Join(srcDir, "meta-data"), []byte(metaData), 0o644); err != nil {
		return "", fmt.Errorf("write meta-data: %w", err)
	}

	// ── user-data ────────────────────────────────────────────────────────
	if userData == "" {
		userData = defaultUserData
	}
	if err := os.WriteFile(filepath.Join(srcDir, "user-data"), []byte(userData), 0o644); err != nil {
		return "", fmt.Errorf("write user-data: %w", err)
	}

	// ── network-config (v2 / Netplan) ────────────────────────────────────
	// Match by MAC so the config works regardless of the guest interface name.
	networkConfig := fmt.Sprintf(
		"version: 2\nethernets:\n  eth0:\n    match:\n      macaddress: \"%s\"\n    dhcp4: true\n",
		macAddr,
	)
	if err := os.WriteFile(filepath.Join(srcDir, "network-config"), []byte(networkConfig), 0o644); err != nil {
		return "", fmt.Errorf("write network-config: %w", err)
	}

	// ── build the ext4 image ─────────────────────────────────────────────
	imgPath := filepath.Join(vmDir, "cidata.img")

	if err := run(ctx, "truncate", "-s", "8M", imgPath); err != nil {
		return "", fmt.Errorf("create cidata image: %w", err)
	}

	// -d copies the source directory contents into the new filesystem.
	// -F allows operating on a regular file instead of a block device.
	if err := run(ctx, "mkfs.ext4", "-L", "cidata", "-d", srcDir, "-F", "-q", imgPath); err != nil {
		return "", fmt.Errorf("format cidata image: %w", err)
	}

	return imgPath, nil
}
