package vmm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
)

const (
	deltaBlockSize = 4096
	deltaMagic     = 0x48415443 // "HATC"
)

// deltaHeader is written at the start of every delta file.
type deltaHeader struct {
	Magic     uint32
	BlockSize uint32
	BaseSize  int64
}

// ComputeDelta compares modifiedPath against basePath block-by-block and
// writes only the changed blocks (with their offsets) to deltaPath.
func ComputeDelta(basePath, modifiedPath, deltaPath string) error {
	baseF, err := os.Open(basePath)
	if err != nil {
		return fmt.Errorf("open base: %w", err)
	}
	defer baseF.Close()

	modF, err := os.Open(modifiedPath)
	if err != nil {
		return fmt.Errorf("open modified: %w", err)
	}
	defer modF.Close()

	baseStat, err := baseF.Stat()
	if err != nil {
		return fmt.Errorf("stat base: %w", err)
	}

	outF, err := os.Create(deltaPath)
	if err != nil {
		return fmt.Errorf("create delta: %w", err)
	}
	defer outF.Close()

	hdr := deltaHeader{
		Magic:     deltaMagic,
		BlockSize: deltaBlockSize,
		BaseSize:  baseStat.Size(),
	}
	if err := binary.Write(outF, binary.LittleEndian, &hdr); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	const ioBufSize = 4 * 1024 * 1024 // 4 MiB read-ahead buffer
	baseReader := bufio.NewReaderSize(baseF, ioBufSize)
	modReader := bufio.NewReaderSize(modF, ioBufSize)
	outWriter := bufio.NewWriterSize(outF, ioBufSize)
	defer outWriter.Flush()

	baseBuf := make([]byte, deltaBlockSize)
	modBuf := make([]byte, deltaBlockSize)
	entryBuf := make([]byte, 12) // 8 bytes offset + 4 bytes length
	var offset int64
	zeroBlock := make([]byte, deltaBlockSize)

	for {
		bn, bErr := io.ReadFull(baseReader, baseBuf)
		mn, mErr := io.ReadFull(modReader, modBuf)

		if bn == 0 && mn == 0 {
			break
		}

		if bErr != nil && bn == 0 {
			copy(baseBuf, zeroBlock)
		}

		if mn > 0 {
			changed := bn != mn || !bytes.Equal(baseBuf[:mn], modBuf[:mn])
			if changed {
				binary.LittleEndian.PutUint64(entryBuf[0:8], uint64(offset))
				binary.LittleEndian.PutUint32(entryBuf[8:12], uint32(mn))
				if _, err := outWriter.Write(entryBuf); err != nil {
					return fmt.Errorf("write entry header: %w", err)
				}
				if _, err := outWriter.Write(modBuf[:mn]); err != nil {
					return fmt.Errorf("write block data: %w", err)
				}
			}
		}

		offset += int64(deltaBlockSize)

		if mErr != nil {
			break
		}
	}

	return outWriter.Flush()
}

// ApplyDelta copies basePath to outputPath using sparse copy, then overwrites
// blocks using the delta file produced by ComputeDelta.
func ApplyDelta(basePath, deltaPath, outputPath string) error {
	if err := run(context.Background(), "cp", "--sparse=always", "--reflink=auto", basePath, outputPath); err != nil {
		// Fall back to plain copy if cp flags aren't supported.
		if err := copyFile(basePath, outputPath); err != nil {
			return fmt.Errorf("copy base: %w", err)
		}
	}

	deltaF, err := os.Open(deltaPath)
	if err != nil {
		return fmt.Errorf("open delta: %w", err)
	}
	defer deltaF.Close()

	deltaReader := bufio.NewReaderSize(deltaF, 4*1024*1024)

	var hdr deltaHeader
	if err := binary.Read(deltaReader, binary.LittleEndian, &hdr); err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	if hdr.Magic != deltaMagic {
		return fmt.Errorf("invalid delta file (bad magic)")
	}

	outF, err := os.OpenFile(outputPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open output for patching: %w", err)
	}
	defer outF.Close()

	outStat, _ := outF.Stat()
	entryBuf := make([]byte, 12)
	data := make([]byte, deltaBlockSize)

	for {
		if _, err := io.ReadFull(deltaReader, entryBuf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return fmt.Errorf("read entry header: %w", err)
		}

		offset := int64(binary.LittleEndian.Uint64(entryBuf[0:8]))
		length := int32(binary.LittleEndian.Uint32(entryBuf[8:12]))

		if int(length) > len(data) {
			data = make([]byte, length)
		}
		if _, err := io.ReadFull(deltaReader, data[:length]); err != nil {
			return fmt.Errorf("read block data: %w", err)
		}

		if offset+int64(length) > outStat.Size() {
			if err := outF.Truncate(offset + int64(length)); err != nil {
				return fmt.Errorf("extend output: %w", err)
			}
			outStat, _ = outF.Stat()
		}

		if _, err := outF.WriteAt(data[:length], offset); err != nil {
			return fmt.Errorf("write block at offset %d: %w", offset, err)
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// capRootfsSize ensures the rootfs image does not exceed maxMiB.
// If the image is larger, it shrinks the ext4 filesystem first, then truncates.
// If smaller, it extends the file and grows the filesystem to use the full space.
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
