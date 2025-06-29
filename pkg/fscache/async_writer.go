package fscache

import (
	"io"
	"os"
)

// asyncFileWriter writes data to a file asynchronously using a buffered channel
// to avoid blocking the calling goroutine. The Write method copies the provided
// bytes and sends them to the writer goroutine. Close waits until all pending
// data has been written to disk.
type asyncFileWriter struct {
	ch   chan []byte
	done chan error
}

// newAsyncFileWriter creates a new asynchronous writer for the provided file.
// buf controls the number of buffered chunks kept in memory. Each chunk has the
// same size as the byte slices passed to Write.
func newAsyncFileWriter(file *os.File, buf int) *asyncFileWriter {
	w := &asyncFileWriter{
		ch:   make(chan []byte, buf),
		done: make(chan error, 1),
	}

	go func() {
		var err error
		for b := range w.ch {
			if err != nil {
				continue
			}
			_, err = file.Write(b)
		}
		file.Close()
		w.done <- err
	}()

	return w
}

// Write implements io.Writer. The slice is copied to avoid data races with the
// caller reusing the buffer.
func (w *asyncFileWriter) Write(p []byte) (int, error) {
	buf := make([]byte, len(p))
	copy(buf, p)
	w.ch <- buf
	return len(p), nil
}

// Close waits until all pending writes have completed and returns any error
// from the writing goroutine.
func (w *asyncFileWriter) Close() error {
	close(w.ch)
	return <-w.done
}

var _ io.WriteCloser = (*asyncFileWriter)(nil)
