package fscache

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type flushResponseRecorder struct {
	header      http.Header
	writes      [][]byte
	flushCalled int
}

func (f *flushResponseRecorder) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *flushResponseRecorder) Write(p []byte) (int, error) {
	copied := append([]byte(nil), p...)
	f.writes = append(f.writes, copied)
	return len(p), nil
}

func (f *flushResponseRecorder) WriteHeader(statusCode int) {
	_ = statusCode
}

func (f *flushResponseRecorder) Flush() {
	f.flushCalled++
}

func TestReaderOnlyReadDelegates(t *testing.T) {
	ro := readerOnly{r: strings.NewReader("abc123")}

	data, err := io.ReadAll(ro)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if got := string(data); got != "abc123" {
		t.Fatalf("data = %q, want %q", got, "abc123")
	}
}

func TestFlushWriterWriteFlushes(t *testing.T) {
	rw := &flushResponseRecorder{}
	fw := flushWriter{w: rw, flusher: rw}

	n, err := fw.Write([]byte("stream-data"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len("stream-data") {
		t.Fatalf("Write() bytes = %d, want %d", n, len("stream-data"))
	}
	if rw.flushCalled != 1 {
		t.Fatalf("flush calls = %d, want 1", rw.flushCalled)
	}
	if len(rw.writes) != 1 || string(rw.writes[0]) != "stream-data" {
		t.Fatalf("writes = %q, want %q", rw.writes, "stream-data")
	}
}
