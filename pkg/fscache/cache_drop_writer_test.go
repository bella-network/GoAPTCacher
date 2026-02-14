package fscache

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type dropCall struct {
	offset int64
	length int64
}

func TestCacheDropWriterSkipsDropsBelowThreshold(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "payload.bin"))
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}
	defer file.Close()

	calls := []dropCall{}
	w := newCacheDropWriterWithDropRange(file, 10, 4, func(_ *os.File, offset, length int64) error {
		calls = append(calls, dropCall{offset: offset, length: length})
		return nil
	})

	if _, err := w.Write([]byte("12345")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	w.DropCache()

	if len(calls) != 0 {
		t.Fatalf("drop calls = %v, want none", calls)
	}
}

func TestNewCacheDropWriterDefaultConstructor(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "payload.bin"))
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}
	defer file.Close()

	w := newCacheDropWriter(file, 1024, 256)
	if w == nil {
		t.Fatalf("newCacheDropWriter() returned nil")
	}
}

func TestCacheDropWriterDropsOnChunkAndFinalFlush(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "payload.bin"))
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}
	defer file.Close()

	calls := []dropCall{}
	w := newCacheDropWriterWithDropRange(file, 10, 4, func(_ *os.File, offset, length int64) error {
		calls = append(calls, dropCall{offset: offset, length: length})
		return nil
	})

	if _, err := w.Write([]byte("123456")); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	if _, err := w.Write([]byte("78901")); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	if _, err := w.Write([]byte("234")); err != nil {
		t.Fatalf("third Write() error = %v", err)
	}
	w.DropCache()

	want := []dropCall{
		{offset: 6, length: 5},
		{offset: 11, length: 3},
		{offset: 0, length: 0},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("drop calls = %#v, want %#v", calls, want)
	}
}

func TestCacheDropWriterPropagatesWriteError(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "payload.bin"))
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("file.Close() error = %v", err)
	}

	calls := 0
	w := newCacheDropWriterWithDropRange(file, 1, 1, func(_ *os.File, _, _ int64) error {
		calls++
		return nil
	})

	n, err := w.Write([]byte("x"))
	if err == nil {
		t.Fatalf("Write() error = nil, want non-nil")
	}
	if n != 0 {
		t.Fatalf("Write() bytes = %d, want 0", n)
	}
	if calls != 0 {
		t.Fatalf("drop calls = %d, want 0", calls)
	}
}
