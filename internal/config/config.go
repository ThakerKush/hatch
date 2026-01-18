package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr          string
	DataDir           string
	FirecrackerBinary string
	BridgeName        string
	BridgeCIDR        string
	DefaultVCPU       int
	DefaultMemMib     int
	DefaultBootArgs   string
}

func LoadFromEnv() Config {
	return Config{
		HTTPAddr:          envOrDefault("HATCH_HTTP_ADDR", ":8080"),
		DataDir:           envOrDefault("HATCH_DATA_DIR", "./data"),
		FirecrackerBinary: envOrDefault("HATCH_FIRECRACKER_BIN", "firecracker"),
		BridgeName:        envOrDefault("HATCH_BRIDGE_NAME", "fcbr0"),
		BridgeCIDR:        envOrDefault("HATCH_BRIDGE_CIDR", "172.16.0.1/24"),
		DefaultVCPU:       envOrDefaultInt("HATCH_DEFAULT_VCPU", 1),
		DefaultMemMib:     envOrDefaultInt("HATCH_DEFAULT_MEM_MIB", 256),
		DefaultBootArgs: envOrDefault(
			"HATCH_DEFAULT_BOOT_ARGS",
			"console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw rootfstype=ext4",
		),
	}
}

func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func envOrDefaultInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
