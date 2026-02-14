package fscache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAsyncFileWriterWritesBufferedData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.bin")

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}

	w := newAsyncFileWriter(file, 2)
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := w.Write([]byte("-world")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if got := string(data); got != "hello-world" {
		t.Fatalf("file content = %q, want %q", got, "hello-world")
	}
}

func TestAsyncFileWriterReturnsWriteErrorOnClose(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "payload.bin"))
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}

	w := newAsyncFileWriter(file, 1)

	if err := file.Close(); err != nil {
		t.Fatalf("file.Close() error = %v", err)
	}

	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if err := w.Close(); err == nil {
		t.Fatalf("Close() error = nil, want non-nil")
	}
}
