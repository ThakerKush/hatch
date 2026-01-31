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

var (
	errInvalidPath      = errors.New("invalid path")
	errMethodNotAllowed = errors.New("method not allowed")
	errNotFound         = errors.New("not found")
)

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

type createVMRequest struct {
	ImageID       string `json:"image_id"`
	VCPUCount     int    `json:"vcpu_count"`
	MemMib        int    `json:"mem_mib"`
	BootArgs      string `json:"boot_args"`
	EnableNetwork *bool  `json:"enable_network"`
	GuestIP       string `json:"guest_ip"`
	GuestMAC      string `json:"guest_mac"`
}

func (r createVMRequest) Validate() error {
	if r.ImageID == "" {
		return errors.New("image_id is required")
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
		VCPUCount:     r.VCPUCount,
		MemMib:        r.MemMib,
		BootArgs:      r.BootArgs,
		EnableNetwork: enableNetwork,
		GuestIP:       r.GuestIP,
		GuestMAC:      r.GuestMAC,
	}
}
