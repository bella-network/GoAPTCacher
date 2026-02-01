//go:build linux

package fscache

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func platformPreallocate(file *os.File, required int64) error {
	if required <= 0 {
		return nil
	}

	if err := unix.Fallocate(int(file.Fd()), 0, 0, required); err != nil {
		if err == unix.ENOSYS || err == unix.EOPNOTSUPP {
			if err := file.Truncate(required); err != nil {
				return fmt.Errorf("truncate: %w", err)
			}
			return nil
		}
		return fmt.Errorf("fallocate: %w", err)
	}

	return nil
}

func platformDropCacheRange(file *os.File, offset, length int64) error {
	return unix.Fadvise(int(file.Fd()), offset, length, unix.FADV_DONTNEED)
}
