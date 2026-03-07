package vmm

import (
	"context"
	"fmt"
	"strings"
)

func createOverlayImage(ctx context.Context, overlayPath string, maxMiB int) error {

	sizeArg := fmt.Sprintf("%dM", maxMiB)
	if err := run(ctx, "truncate", "-s", sizeArg, overlayPath); err != nil {
		return fmt.Errorf("create sparse overlay: %w", err)
	}
	if err := run(ctx, "mkfs.ext4", "-q", "-F", overlayPath); err != nil {
		return fmt.Errorf("format overlay ext4: %w", err)
	}
	return nil
}

func ensureOverlayBootArgs(bootArgs string) string {
	fields := strings.Fields(bootArgs)
	out := make([]string, 0, len(fields)+4)

	var hasRoot, hasRO, hasRootFSType, hasInit bool
	for _, field := range fields {
		switch {
		case field == "rw":
			out = append(out, "ro")
			hasRO = true
		case field == "ro":
			out = append(out, field)
			hasRO = true
		case strings.HasPrefix(field, "root="):
			out = append(out, "root=/dev/vda")
			hasRoot = true
		case strings.HasPrefix(field, "rootfstype="):
			out = append(out, "rootfstype=ext4")
			hasRootFSType = true
		case strings.HasPrefix(field, "init="):
			out = append(out, "init=/sbin/overlay-init")
			hasInit = true
		default:
			out = append(out, field)
		}
	}

	if !hasRoot {
		out = append(out, "root=/dev/vda")
	}
	if !hasRO {
		out = append(out, "ro")
	}
	if !hasRootFSType {
		out = append(out, "rootfstype=ext4")
	}
	if !hasInit {
		out = append(out, "init=/sbin/overlay-init")
	}

	return strings.Join(out, " ")
}
