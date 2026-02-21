package vmm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const defaultUserData = "#cloud-config\n"

// CreateCloudInitDisk builds a NoCloud seed image (ISO 9660 with volume ID
// "cidata") containing meta-data, user-data and network-config. Cloud-init
// auto-detects ISO 9660 volumes labelled "cidata" and applies the config.
//
// Prerequisites on the host: genisoimage.
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

	if err := run(ctx, "genisoimage",
		"-output", imgPath,
		"-volid", "cidata",
		"-joliet",
		"-rock",
		"-quiet",
		srcDir,
	); err != nil {
		return "", fmt.Errorf("create cidata ISO image: %w", err)
	}

	return imgPath, nil
}
