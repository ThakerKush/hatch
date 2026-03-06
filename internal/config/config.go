package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the Hatch daemon.
type Config struct {
	// HTTP API
	HTTPAddr string
	DataDir  string
	// Better Auth endpoints for Go middleware.
	BetterAuthVerifyURL  string
	BetterAuthSessionURL string

	// Database
	DatabaseURL string
	// Per-user quotas.
	MaxVMsPerUser     int
	MaxSnapshotsPerVM int

	// Firecracker
	FirecrackerBinary string
	BridgeName        string
	BridgeCIDR        string
	DefaultVCPU       int
	DefaultMemMib     int
	DefaultBootArgs   string

	// Default image (auto-seeded on startup when both are set)
	DefaultKernelPath string
	DefaultRootfsPath string

	// Reverse proxy
	ProxyAddr        string
	ProxyBaseDomain  string
	ProxyWakeTimeout time.Duration

	// SSH forwarding (host port -> guest:22)
	SSHPortMin     int
	SSHPortMax     int
	SSHAllowedCIDR string

	// S3 snapshot storage
	S3Endpoint  string
	S3Bucket    string
	S3Region    string
	S3AccessKey string
	S3SecretKey string

	// Idle monitor
	IdleCheckInterval time.Duration
	IdleTimeout       time.Duration
}

// LoadFromEnv reads configuration from environment variables with sensible defaults.
func LoadFromEnv() Config {
	return Config{
		HTTPAddr: envOrDefault("HATCH_HTTP_ADDR", ":8080"),
		DataDir:  envOrDefault("HATCH_DATA_DIR", "./data"),
		BetterAuthVerifyURL: envOrDefault(
			"HATCH_BETTER_AUTH_VERIFY_URL",
			"http://127.0.0.1:3000/api/auth/api-key/verify",
		),
		BetterAuthSessionURL: envOrDefault(
			"HATCH_BETTER_AUTH_SESSION_URL",
			"http://127.0.0.1:3000/api/auth/get-session",
		),
		DatabaseURL:       envOrDefault("DATABASE_URL", "postgres://hatch:hatch@localhost:5432/hatch?sslmode=disable"),
		MaxVMsPerUser:     envOrDefaultInt("HATCH_MAX_VMS_PER_USER", 5),
		MaxSnapshotsPerVM: envOrDefaultInt("HATCH_MAX_SNAPSHOTS_PER_VM", 10),
		FirecrackerBinary: envOrDefault("HATCH_FIRECRACKER_BIN", "firecracker"),
		BridgeName:        envOrDefault("HATCH_BRIDGE_NAME", "fcbr0"),
		BridgeCIDR:        envOrDefault("HATCH_BRIDGE_CIDR", "172.16.0.1/24"),
		DefaultVCPU:       envOrDefaultInt("HATCH_DEFAULT_VCPU", 1),
		DefaultMemMib:     envOrDefaultInt("HATCH_DEFAULT_MEM_MIB", 512),
		DefaultBootArgs: envOrDefault(
			"HATCH_DEFAULT_BOOT_ARGS",
			"console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw rootfstype=ext4",
		),

		// Default image
		DefaultKernelPath: envOrDefault("HATCH_DEFAULT_KERNEL_PATH", ""),
		DefaultRootfsPath: envOrDefault("HATCH_DEFAULT_ROOTFS_PATH", ""),

		// Reverse proxy
		ProxyAddr:        envOrDefault("HATCH_PROXY_ADDR", ":9090"),
		ProxyBaseDomain:  envOrDefault("HATCH_PROXY_BASE_DOMAIN", "hatch.local"),
		ProxyWakeTimeout: envOrDefaultDuration("HATCH_PROXY_WAKE_TIMEOUT", 60*time.Second),

		// SSH forwarding
		SSHPortMin:     envOrDefaultInt("HATCH_SSH_PORT_MIN", 16000),
		SSHPortMax:     envOrDefaultInt("HATCH_SSH_PORT_MAX", 26000),
		SSHAllowedCIDR: envOrDefault("HATCH_SSH_ALLOWED_CIDR", "127.0.0.1/32"),

		// S3
		S3Endpoint:  envOrDefault("HATCH_S3_ENDPOINT", ""),
		S3Bucket:    envOrDefault("HATCH_S3_BUCKET", ""),
		S3Region:    envOrDefault("HATCH_S3_REGION", "us-east-1"),
		S3AccessKey: envOrDefault("HATCH_S3_ACCESS_KEY", ""),
		S3SecretKey: envOrDefault("HATCH_S3_SECRET_KEY", ""),

		// Idle monitor
		IdleCheckInterval: envOrDefaultDuration("HATCH_IDLE_CHECK_INTERVAL", 5*time.Minute),
		IdleTimeout:       envOrDefaultDuration("HATCH_IDLE_TIMEOUT", 45*time.Minute),
	}
}

// S3Enabled returns true when the minimal S3 configuration is present.
func (c Config) S3Enabled() bool {
	return c.S3Bucket != ""
}

// DefaultImageConfigured returns true when a default kernel and rootfs are set.
func (c Config) DefaultImageConfigured() bool {
	return c.DefaultKernelPath != "" && c.DefaultRootfsPath != ""
}

// DefaultImageID is the well-known ID used for the auto-seeded default image.
const DefaultImageID = "img_default"

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

func envOrDefaultDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
