package vmm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const defaultUserData = "#cloud-config\n"

// CreateCloudInitDisk builds a NoCloud seed image (FAT32 filesystem labelled
// "cidata") containing meta-data, user-data and network-config. Cloud-init's
// NoCloud datasource auto-detects vfat volumes with the "cidata" label.
//
// Prerequisites on the host: mkfs.vfat (dosfstools), mcopy (mtools).
func CreateCloudInitDisk(ctx context.Context, vmDir, instanceID, macAddr, userData string) (string, error) {
	srcDir := filepath.Join(vmDir, "cidata-src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return "", fmt.Errorf("create cidata source dir: %w", err)
	}
	defer os.RemoveAll(srcDir)

	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceID, instanceID)
	if err := os.WriteFile(filepath.Join(srcDir, "meta-data"), []byte(metaData), 0o644); err != nil {
		return "", fmt.Errorf("write meta-data: %w", err)
	}

	if userData == "" {
		userData = defaultUserData
	}
	if err := os.WriteFile(filepath.Join(srcDir, "user-data"), []byte(userData), 0o644); err != nil {
		return "", fmt.Errorf("write user-data: %w", err)
	}

	networkConfig := fmt.Sprintf(
		"version: 2\nethernets:\n  eth0:\n    match:\n      macaddress: \"%s\"\n    dhcp4: true\n",
		macAddr,
	)
	if err := os.WriteFile(filepath.Join(srcDir, "network-config"), []byte(networkConfig), 0o644); err != nil {
		return "", fmt.Errorf("write network-config: %w", err)
	}

	imgPath := filepath.Join(vmDir, "cidata.img")

	if err := run(ctx, "truncate", "-s", "8M", imgPath); err != nil {
		return "", fmt.Errorf("create cidata image: %w", err)
	}

	if err := run(ctx, "mkfs.vfat", "-n", "CIDATA", imgPath); err != nil {
		return "", fmt.Errorf("format cidata image: %w", err)
	}

	// Copy seed files into the FAT image without mounting (mcopy from mtools).
	for _, name := range []string{"meta-data", "user-data", "network-config"} {
		src := filepath.Join(srcDir, name)
		if err := run(ctx, "mcopy", "-i", imgPath, src, "::"+name); err != nil {
			return "", fmt.Errorf("copy %s into cidata image: %w", name, err)
		}
	}

	return imgPath, nil
}
