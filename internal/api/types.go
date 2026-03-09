package api

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/ThakerKush/Hatch/internal/config"
	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/util"
	"github.com/ThakerKush/Hatch/internal/vmm"
)

var errNoDefaultImage = fmt.Errorf("no image_id provided and no default image configured (set HATCH_DEFAULT_KERNEL_PATH and HATCH_DEFAULT_ROOTFS_PATH)")

// --- sentinel errors ---

var (
	errInvalidPath      = errors.New("invalid path")
	errMethodNotAllowed = errors.New("method not allowed")
	errNotFound         = errors.New("not found")
)

// --- Image ---

type createImageRequest struct {
	ID         string `json:"id"`
	KernelPath string `json:"kernel_path"`
	RootfsPath string `json:"rootfs_path"`
	BootArgs   string `json:"boot_args"`
}

func (r createImageRequest) Validate() error {
	if r.KernelPath == "" || r.RootfsPath == "" {
		return errors.New("kernel_path and rootfs_path are required")
	}
	if _, err := os.Stat(r.KernelPath); err != nil {
		return err
	}
	if _, err := os.Stat(r.RootfsPath); err != nil {
		return err
	}
	return nil
}

func (r createImageRequest) ToImage(cfg config.Config) store.Image {
	id := r.ID
	if id == "" {
		id = util.RandomID("img")
	}
	bootArgs := r.BootArgs
	if bootArgs == "" {
		bootArgs = cfg.DefaultBootArgs
	}
	return store.Image{
		ID:         id,
		KernelPath: r.KernelPath,
		RootfsPath: r.RootfsPath,
		BootArgs:   bootArgs,
		CreatedAt:  time.Now().UTC(),
	}
}

// --- VM ---

type createVMRequest struct {
	ImageID       string `json:"image_id"`
	TemplateID    string `json:"template_id"`
	VCPUCount     int    `json:"vcpu_count"`
	MemMib        int    `json:"mem_mib"`
	BootArgs      string `json:"boot_args"`
	EnableNetwork *bool  `json:"enable_network"`
	GuestIP       string `json:"guest_ip"`
	GuestMAC      string `json:"guest_mac"`
	UserData      string `json:"user_data"`
}

func (r createVMRequest) Validate() error {
	// image_id, template_id, or neither (uses default image) are all valid.
	return nil
}

// ResolveImageID fills in ImageID from the default when not explicitly set.
func (r *createVMRequest) ResolveImageID(cfg config.Config) error {
	if r.ImageID != "" {
		return nil
	}
	if !cfg.DefaultImageConfigured() {
		return errNoDefaultImage
	}
	r.ImageID = config.DefaultImageID
	return nil
}

func (r createVMRequest) ToOptions(cfg config.Config) vmm.CreateOptions {
	enableNetwork := true
	if r.EnableNetwork != nil {
		enableNetwork = *r.EnableNetwork
	}
	return vmm.CreateOptions{
		ImageID:       r.ImageID,
		TemplateID:    r.TemplateID,
		VCPUCount:     r.VCPUCount,
		MemMib:        r.MemMib,
		BootArgs:      r.BootArgs,
		EnableNetwork: enableNetwork,
		GuestIP:       r.GuestIP,
		GuestMAC:      r.GuestMAC,
		UserData:      r.UserData,
	}
}

// --- Template ---

type createTemplateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ImageID     string `json:"image_id"`
	UserData    string `json:"user_data"`
	VCPUCount   int    `json:"vcpu_count"`
	MemMib      int    `json:"mem_mib"`
}

func (r createTemplateRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	// image_id is optional; defaults to the auto-seeded default image.
	return nil
}

// ResolveImageID fills in ImageID from the default when not explicitly set.
func (r *createTemplateRequest) ResolveImageID(cfg config.Config) error {
	if r.ImageID != "" {
		return nil
	}
	if !cfg.DefaultImageConfigured() {
		return errNoDefaultImage
	}
	r.ImageID = config.DefaultImageID
	return nil
}

func (r createTemplateRequest) ToTemplate(cfg config.Config) store.Template {
	vcpu := r.VCPUCount
	if vcpu == 0 {
		vcpu = cfg.DefaultVCPU
	}
	mem := r.MemMib
	if mem == 0 {
		mem = cfg.DefaultMemMib
	}
	now := time.Now().UTC()
	return store.Template{
		ID:          util.RandomID("tpl"),
		Name:        r.Name,
		Description: r.Description,
		ImageID:     r.ImageID,
		UserData:    r.UserData,
		VCPUCount:   vcpu,
		MemMib:      mem,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// --- Connect (SSH certificate signing) ---

type connectRequest struct {
	PublicKey string `json:"public_key"`
}

type connectResponse struct {
	Certificate string `json:"certificate"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	ValidUntil  int64  `json:"valid_until"`
}

// --- Exec (run command in VM via SSH) ---

type execRequest struct {
	Command    string `json:"command"`
	TimeoutMs  int    `json:"timeout_ms,omitempty"`
	Background bool   `json:"background,omitempty"`
}

const defaultExecTimeoutMs = 10_000

type execResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
	PID      int    `json:"pid,omitempty"`
	LogFile  string `json:"log_file,omitempty"`
}

// --- Proxy Route ---

var (
	subdomainRe       = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	reservedSubdomains = map[string]struct{}{
		"api": {}, "www": {}, "admin": {}, "app": {}, "dashboard": {},
		"mail": {}, "smtp": {}, "ftp": {}, "ssh": {}, "ns1": {}, "ns2": {},
		"status": {}, "docs": {}, "help": {}, "support": {}, "billing": {},
		"login": {}, "auth": {}, "oauth": {}, "cdn": {}, "static": {},
		"assets": {}, "media": {}, "hatch": {}, "console": {}, "panel": {},
		"grafana": {}, "prometheus": {}, "traefik": {}, "minio": {},
		"postgres": {}, "db": {}, "redis": {}, "internal": {},
	}
)

type createRouteRequest struct {
	Subdomain  string `json:"subdomain,omitempty"`
	TargetPort int    `json:"target_port"`
	AutoWake   *bool  `json:"auto_wake"`
}

func (r *createRouteRequest) Validate() error {
	if r.TargetPort <= 0 || r.TargetPort > 65535 {
		return errors.New("target_port must be between 1 and 65535")
	}
	if r.Subdomain == "" {
		r.Subdomain = util.RandomID("")[:12]
	}
	if !subdomainRe.MatchString(r.Subdomain) {
		return errors.New("subdomain must be lowercase alphanumeric with optional hyphens (max 63 chars)")
	}
	if _, ok := reservedSubdomains[r.Subdomain]; ok {
		return fmt.Errorf("subdomain %q is reserved", r.Subdomain)
	}
	return nil
}

func (r createRouteRequest) ToRoute(vmID string) store.ProxyRoute {
	autoWake := true
	if r.AutoWake != nil {
		autoWake = *r.AutoWake
	}
	now := time.Now().UTC()
	return store.ProxyRoute{
		ID:         util.RandomID("rt"),
		VMID:       vmID,
		Subdomain:  r.Subdomain,
		TargetPort: r.TargetPort,
		AutoWake:   autoWake,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
