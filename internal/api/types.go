package api

import (
	"errors"
	"os"
	"time"

	"github.com/ThakerKush/Hatch/internal/config"
	"github.com/ThakerKush/Hatch/internal/store"
	"github.com/ThakerKush/Hatch/internal/util"
	"github.com/ThakerKush/Hatch/internal/vmm"
)

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
	if r.ImageID == "" && r.TemplateID == "" {
		return errors.New("image_id or template_id is required")
	}
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
	if r.ImageID == "" {
		return errors.New("image_id is required")
	}
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

// --- Proxy Route ---

type createRouteRequest struct {
	Subdomain  string `json:"subdomain"`
	TargetPort int    `json:"target_port"`
	AutoWake   *bool  `json:"auto_wake"`
}

func (r createRouteRequest) Validate() error {
	if r.Subdomain == "" {
		return errors.New("subdomain is required")
	}
	if r.TargetPort <= 0 || r.TargetPort > 65535 {
		return errors.New("target_port must be between 1 and 65535")
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
