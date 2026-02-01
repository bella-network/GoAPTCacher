//go:build !linux

package fscache

import "os"

func platformPreallocate(file *os.File, required int64) error {
	if required <= 0 {
		return nil
	}

	return file.Truncate(required)
}

func platformDropCacheRange(file *os.File, offset, length int64) error {
	return nil
}
