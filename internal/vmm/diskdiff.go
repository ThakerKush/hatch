package vmm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// capRootfsSize ensures the ext4 image does not exceed maxMiB. While the
// overlay-based flow no longer uses it for per-VM disks, keeping the helper
// is still useful for one-off image maintenance.
func capRootfsSize(ctx context.Context, rootfsPath string, maxMiB int) error {
	if maxMiB <= 0 {
		return nil
	}

	maxBytes := int64(maxMiB) * 1024 * 1024

	fi, err := os.Stat(rootfsPath)
	if err != nil {
		return fmt.Errorf("stat rootfs: %w", err)
	}

	current := fi.Size()
	slog.Info("rootfs size check", "current_mib", current/(1024*1024), "max_mib", maxMiB)

	if current > maxBytes {
		// Shrink: fsck, resize filesystem down, then truncate the file.
		if err := run(ctx, "e2fsck", "-fy", rootfsPath); err != nil {
			slog.Warn("e2fsck failed (may be ok for clean fs)", "error", err)
		}
		sizeArg := fmt.Sprintf("%dM", maxMiB)
		if err := run(ctx, "resize2fs", rootfsPath, sizeArg); err != nil {
			return fmt.Errorf("resize2fs shrink: %w", err)
		}
		if err := os.Truncate(rootfsPath, maxBytes); err != nil {
			return fmt.Errorf("truncate rootfs: %w", err)
		}
		slog.Info("rootfs shrunk", "new_mib", maxMiB)
	} else if current < maxBytes {
		// Grow: extend the sparse file, then expand the filesystem.
		if err := os.Truncate(rootfsPath, maxBytes); err != nil {
			return fmt.Errorf("extend rootfs: %w", err)
		}
		if err := run(ctx, "e2fsck", "-fy", rootfsPath); err != nil {
			slog.Warn("e2fsck failed (may be ok for clean fs)", "error", err)
		}
		if err := run(ctx, "resize2fs", rootfsPath); err != nil {
			return fmt.Errorf("resize2fs grow: %w", err)
		}
		slog.Info("rootfs extended", "new_mib", maxMiB)
	}

	return nil
}
