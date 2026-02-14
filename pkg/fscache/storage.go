package fscache

import (
	"fmt"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/unix"
)

// ensureDiskSpace verifies that the filesystem backing the provided path has
// at least required bytes available.
func ensureDiskSpace(path string, required int64) error {
	if required <= 0 {
		return nil
	}

	dir := path
	info, err := os.Stat(path)
	switch {
	case err == nil && !info.IsDir():
		dir = filepath.Dir(path)
	case err != nil:
		dir = filepath.Dir(path)
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return fmt.Errorf("statfs %s: %w", dir, err)
	}

	if stat.Bsize <= 0 {
		return fmt.Errorf("invalid block size: %d", stat.Bsize)
	}

	blockSize, err := strconv.ParseUint(strconv.FormatInt(stat.Bsize, 10), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid block size: %w", err)
	}
	requiredBytes, err := strconv.ParseUint(strconv.FormatInt(required, 10), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid required size: %w", err)
	}

	hi, freeBytes := bits.Mul64(stat.Bavail, blockSize)
	if hi != 0 {
		freeBytes = ^uint64(0)
	}

	if freeBytes < requiredBytes {
		return fmt.Errorf("insufficient disk space: need %d bytes, available %d bytes", required, freeBytes)
	}

	return nil
}

// preallocateFile attempts to reserve required bytes on disk for the provided file.
func preallocateFile(file *os.File, required int64) error {
	if required <= 0 {
		return nil
	}

	if err := platformPreallocate(file, required); err != nil {
		return err
	}

	_, err := file.Seek(0, io.SeekStart)
	return err
}
