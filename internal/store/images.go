package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Image represents a registered VM base image (kernel + rootfs).
type Image struct {
	ID         string    `json:"id"`
	KernelPath string    `json:"kernel_path"`
	RootfsPath string    `json:"rootfs_path"`
	BootArgs   string    `json:"boot_args"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateImage inserts a new image record.
func (d *DB) CreateImage(img Image) error {
	_, err := d.db.Exec(
		`INSERT INTO images (id, kernel_path, rootfs_path, boot_args, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		img.ID, img.KernelPath, img.RootfsPath, img.BootArgs, img.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create image: %w", err)
	}
	return nil
}

// GetImage retrieves an image by ID. Returns nil if not found.
func (d *DB) GetImage(id string) (*Image, error) {
	row := d.db.QueryRow(
		`SELECT id, kernel_path, rootfs_path, boot_args, created_at FROM images WHERE id = $1`, id,
	)
	img := &Image{}
	err := row.Scan(&img.ID, &img.KernelPath, &img.RootfsPath, &img.BootArgs, &img.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get image: %w", err)
	}
	return img, nil
}

// ListImages returns all registered images.
func (d *DB) ListImages() ([]Image, error) {
	rows, err := d.db.Query(
		`SELECT id, kernel_path, rootfs_path, boot_args, created_at FROM images ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	defer rows.Close()

	var images []Image
	for rows.Next() {
		var img Image
		if err := rows.Scan(&img.ID, &img.KernelPath, &img.RootfsPath, &img.BootArgs, &img.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan image: %w", err)
		}
		images = append(images, img)
	}
	return images, rows.Err()
}

// DeleteImage removes an image by ID. Returns true if a row was deleted.
func (d *DB) DeleteImage(id string) (bool, error) {
	res, err := d.db.Exec(`DELETE FROM images WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete image: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
