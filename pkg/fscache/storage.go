package fscache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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

	free := int64(stat.Bavail) * int64(stat.Bsize)
	if free < required {
		return fmt.Errorf("insufficient disk space: need %d bytes, available %d bytes", required, free)
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
