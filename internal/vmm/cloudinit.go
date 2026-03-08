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

const sshdCADropIn = `TrustedUserCAKeys /etc/ssh/hatch-ca.pub
AuthorizedPrincipalsFile /etc/ssh/auth_principals/%u
AllowUsers agent *
PermitRootLogin no
`

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

// patchAgentUser ensures the cloud-init user-data contains a platform-level
// "agent" user with passwordless sudo. If the caller already defined an
// "agent" user in their cloud-init, we leave it alone.
func patchAgentUser(userData string) (string, error) {
	header := "#cloud-config"
	body := userData
	if strings.HasPrefix(userData, header) {
		body = userData[len(header):]
	}

	var cfg map[interface{}]interface{}
	if err := yaml.Unmarshal([]byte(body), &cfg); err != nil || cfg == nil {
		cfg = make(map[interface{}]interface{})
	}

	userList, _ := cfg["users"].([]interface{})

	for _, u := range userList {
		uMap, ok := u.(map[interface{}]interface{})
		if !ok {
			continue
		}
		if name, _ := uMap["name"].(string); name == "agent" {
			return userData, nil
		}
	}

	agentUser := map[interface{}]interface{}{
		"name":        "agent",
		"groups":      "sudo",
		"shell":       "/bin/bash",
		"sudo":        []interface{}{"ALL=(ALL) NOPASSWD:ALL"},
		"lock_passwd": true,
	}
	cfg["users"] = append(userList, agentUser)

	patched, err := yaml.Marshal(cfg)
	if err != nil {
		return userData, nil
	}
	return header + "\n" + string(patched), nil
}

// InjectCloudInitSeed writes NoCloud seed files into the writable overlay
// image under upper/var/lib/cloud/seed/nocloud/. At boot the guest's
// overlay-init mounts that image and overlays upper/ on top of the shared
// read-only base rootfs, so cloud-init sees the seed files at the normal
// /var/lib/cloud/seed/nocloud/ path.
//
// When caPublicKey is non-nil, SSH CA trust files are also injected into
// the overlay so that platform-level certificate-based SSH access works.
func InjectCloudInitSeed(ctx context.Context, overlayPath, vmDir, instanceID, macAddr, userData string, caPublicKey []byte) error {
	srcDir := filepath.Join(vmDir, "seed-src")
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
	if len(caPublicKey) > 0 {
		var err error
		userData, err = patchAgentUser(userData)
		if err != nil {
			return fmt.Errorf("patch agent user: %w", err)
		}
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

	mountDir := filepath.Join(vmDir, "overlay-mount")
	if err := os.MkdirAll(mountDir, 0o755); err != nil {
		return fmt.Errorf("create mount dir: %w", err)
	}
	defer os.RemoveAll(mountDir)

	if err := run(ctx, "mount", "-o", "loop", overlayPath, mountDir); err != nil {
		return fmt.Errorf("mount overlay: %w", err)
	}
	defer run(context.Background(), "umount", mountDir)

	upperDir := filepath.Join(mountDir, "upper")
	if err := os.MkdirAll(upperDir, 0o755); err != nil {
		return fmt.Errorf("create overlay upper dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(mountDir, "work"), 0o755); err != nil {
		return fmt.Errorf("create overlay work dir: %w", err)
	}

	seedDir := filepath.Join(upperDir, "var", "lib", "cloud", "seed", "nocloud")
	if err := os.MkdirAll(seedDir, 0o755); err != nil {
		return fmt.Errorf("create seed dir in overlay: %w", err)
	}

	for _, name := range []string{"meta-data", "user-data", "network-config"} {
		data, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(seedDir, name), data, 0o644); err != nil {
			return fmt.Errorf("write %s to overlay: %w", name, err)
		}
	}

	if len(caPublicKey) > 0 {
		if err := injectSSHCA(upperDir, instanceID, caPublicKey); err != nil {
			return fmt.Errorf("inject SSH CA: %w", err)
		}
	}

	return nil
}

// injectSSHCA writes the CA public key, sshd drop-in config, and
// per-VM auth_principals file into the overlay upper directory.
func injectSSHCA(upperDir, vmID string, caPublicKey []byte) error {
	sshDir := filepath.Join(upperDir, "etc", "ssh")
	if err := os.MkdirAll(sshDir, 0o755); err != nil {
		return fmt.Errorf("create etc/ssh: %w", err)
	}

	if err := os.WriteFile(filepath.Join(sshDir, "hatch-ca.pub"), caPublicKey, 0o644); err != nil {
		return fmt.Errorf("write CA public key: %w", err)
	}

	dropInDir := filepath.Join(sshDir, "sshd_config.d")
	if err := os.MkdirAll(dropInDir, 0o755); err != nil {
		return fmt.Errorf("create sshd_config.d: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dropInDir, "99-hatch-ca.conf"), []byte(sshdCADropIn), 0o644); err != nil {
		return fmt.Errorf("write sshd drop-in: %w", err)
	}

	principalsDir := filepath.Join(sshDir, "auth_principals")
	if err := os.MkdirAll(principalsDir, 0o755); err != nil {
		return fmt.Errorf("create auth_principals: %w", err)
	}
	if err := os.WriteFile(filepath.Join(principalsDir, "agent"), []byte(vmID+"\n"), 0o644); err != nil {
		return fmt.Errorf("write auth_principals/agent: %w", err)
	}

	return nil
}
