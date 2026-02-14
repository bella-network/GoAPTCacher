package fscache

import "os"

const (
	cacheDropThreshold = int64(128 * 1024 * 1024) // only drop cache for large downloads
	cacheDropChunk     = int64(16 * 1024 * 1024)  // drop in 16MB ranges
)

// cacheDropWriter writes to a file and hints the kernel to drop written pages
// from the page cache once a threshold is exceeded.
type cacheDropWriter struct {
	f         *os.File
	offset    int64
	pending   int64
	chunk     int64
	threshold int64
	enabled   bool
	dropRange func(file *os.File, offset, length int64) error
}

func newCacheDropWriter(file *os.File, threshold, chunk int64) *cacheDropWriter {
	return newCacheDropWriterWithDropRange(file, threshold, chunk, platformDropCacheRange)
}

func newCacheDropWriterWithDropRange(file *os.File, threshold, chunk int64, dropRange func(file *os.File, offset, length int64) error) *cacheDropWriter {
	if dropRange == nil {
		dropRange = platformDropCacheRange
	}
	return &cacheDropWriter{
		f:         file,
		chunk:     chunk,
		threshold: threshold,
		dropRange: dropRange,
	}
}

func (w *cacheDropWriter) Write(p []byte) (int, error) {
	n, err := w.f.Write(p)
	if n <= 0 {
		return n, err
	}

	w.offset += int64(n)
	if !w.enabled {
		if w.offset < w.threshold {
			return n, err
		}
		w.enabled = true
		w.pending = 0
	}

	w.pending += int64(n)
	if w.pending >= w.chunk {
		_ = w.dropRange(w.f, w.offset-w.pending, w.pending)
		w.pending = 0
	}

	return n, err
}

func (w *cacheDropWriter) DropCache() {
	if !w.enabled {
		return
	}
	if w.pending > 0 {
		_ = w.dropRange(w.f, w.offset-w.pending, w.pending)
		w.pending = 0
	}
	_ = w.dropRange(w.f, 0, 0)
}
