package vmm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

const defaultUserData = "#cloud-config\n"

// patchHomeOwnership parses a cloud-config user-data string and, for every
// user defined in the "users" block, prepends runcmd entries that fix
// /home/<user> ownership. This is necessary because cloud-init's write_files
// module runs before home directories are created, so any write_files entry
// that targets /home/<user>/... causes cloud-init to create the directory as
// root:root. The prepended chown/chmod commands run early in runcmd and
// correct ownership before anything else touches the home directory.
func patchHomeOwnership(userData string) (string, error) {
	header := "#cloud-config"
	body := userData
	if strings.HasPrefix(userData, header) {
		body = userData[len(header):]
	}

	var cfg map[interface{}]interface{}
	if err := yaml.Unmarshal([]byte(body), &cfg); err != nil || cfg == nil {
		// Not parseable YAML – return as-is so we never break valid configs.
		return userData, nil
	}

	usersRaw, ok := cfg["users"]
	if !ok {
		return userData, nil
	}
	userList, ok := usersRaw.([]interface{})
	if !ok {
		return userData, nil
	}

	var fixCmds []interface{}
	for _, u := range userList {
		uMap, ok := u.(map[interface{}]interface{})
		if !ok {
			continue
		}
		name, ok := uMap["name"].(string)
		if !ok || name == "" {
			continue
		}
		home := fmt.Sprintf("/home/%s", name)
		if h, ok := uMap["home"].(string); ok && h != "" {
			home = h
		}
		fixCmds = append(fixCmds,
			[]interface{}{"chown", "-R", fmt.Sprintf("%s:%s", name, name), home},
			[]interface{}{"chmod", "700", home},
		)
	}

	if len(fixCmds) == 0 {
		return userData, nil
	}

	existing, _ := cfg["runcmd"].([]interface{})
	cfg["runcmd"] = append(fixCmds, existing...)

	patched, err := yaml.Marshal(cfg)
	if err != nil {
		return userData, nil
	}
	return header + "\n" + string(patched), nil
}

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
	var err error
	userData, err = patchHomeOwnership(userData)
	if err != nil {
		return fmt.Errorf("patch home ownership: %w", err)
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
