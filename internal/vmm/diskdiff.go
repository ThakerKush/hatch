package vmm

import (
	"encoding/binary"
	"fmt"
	"io"
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

	baseBuf := make([]byte, deltaBlockSize)
	modBuf := make([]byte, deltaBlockSize)
	var offset int64

	for {
		bn, bErr := io.ReadFull(baseF, baseBuf)
		mn, mErr := io.ReadFull(modF, modBuf)

		if bn == 0 && mn == 0 {
			break
		}

		// Modified image is larger than base — all extra blocks are "changed".
		if bErr != nil && bn == 0 {
			bn = 0
			for i := range baseBuf {
				baseBuf[i] = 0
			}
		}

		if mn > 0 {
			changed := bn != mn
			if !changed {
				for i := 0; i < mn; i++ {
					if baseBuf[i] != modBuf[i] {
						changed = true
						break
					}
				}
			}
			if changed {
				if err := binary.Write(outF, binary.LittleEndian, offset); err != nil {
					return fmt.Errorf("write offset: %w", err)
				}
				if err := binary.Write(outF, binary.LittleEndian, int32(mn)); err != nil {
					return fmt.Errorf("write length: %w", err)
				}
				if _, err := outF.Write(modBuf[:mn]); err != nil {
					return fmt.Errorf("write block data: %w", err)
				}
			}
		}

		offset += int64(deltaBlockSize)

		if mErr != nil {
			break
		}
	}

	return nil
}

// ApplyDelta copies basePath to outputPath, then overwrites blocks using
// the delta file produced by ComputeDelta.
func ApplyDelta(basePath, deltaPath, outputPath string) error {
	if err := copyFile(basePath, outputPath); err != nil {
		return fmt.Errorf("copy base: %w", err)
	}

	deltaF, err := os.Open(deltaPath)
	if err != nil {
		return fmt.Errorf("open delta: %w", err)
	}
	defer deltaF.Close()

	var hdr deltaHeader
	if err := binary.Read(deltaF, binary.LittleEndian, &hdr); err != nil {
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

	// If modified image was larger than base, extend the output.
	outStat, _ := outF.Stat()
	for {
		var offset int64
		if err := binary.Read(deltaF, binary.LittleEndian, &offset); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read offset: %w", err)
		}

		var length int32
		if err := binary.Read(deltaF, binary.LittleEndian, &length); err != nil {
			return fmt.Errorf("read length: %w", err)
		}

		data := make([]byte, length)
		if _, err := io.ReadFull(deltaF, data); err != nil {
			return fmt.Errorf("read block data: %w", err)
		}

		// Extend file if writing beyond current size.
		if offset+int64(length) > outStat.Size() {
			if err := outF.Truncate(offset + int64(length)); err != nil {
				return fmt.Errorf("extend output: %w", err)
			}
		}

		if _, err := outF.WriteAt(data, offset); err != nil {
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
