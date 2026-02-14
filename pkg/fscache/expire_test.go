package fscache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetUnusedFilesZeroDays(t *testing.T) {
	cache := newTestFSCache(t)

	files, err := cache.GetUnusedFiles(0)
	if err != nil {
		t.Fatalf("GetUnusedFiles() error = %v", err)
	}
	if files != nil {
		t.Fatalf("GetUnusedFiles() = %v, want nil", files)
	}
}

func TestGetUnusedFilesFiltersByLastAccessed(t *testing.T) {
	cache := newTestFSCache(t)
	oldURL := mustParseURL(t, "https://example.com/pool/main/p/old.deb")
	newURL := mustParseURL(t, "https://example.com/pool/main/p/new.deb")

	if err := cache.Set(DetermineProtocolFromURL(oldURL), oldURL.Host, oldURL.Path, AccessEntry{
		URL:          oldURL,
		LastAccessed: time.Now().Add(-48 * time.Hour),
		Size:         10,
	}); err != nil {
		t.Fatalf("Set(old) error = %v", err)
	}
	if err := cache.Set(DetermineProtocolFromURL(newURL), newURL.Host, newURL.Path, AccessEntry{
		URL:          newURL,
		LastAccessed: time.Now().Add(-2 * time.Hour),
		Size:         20,
	}); err != nil {
		t.Fatalf("Set(new) error = %v", err)
	}

	files, err := cache.GetUnusedFiles(1)
	if err != nil {
		t.Fatalf("GetUnusedFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("unused files len = %d, want 1", len(files))
	}
	if got := files[0].String(); got != oldURL.String() {
		t.Fatalf("unused file = %q, want %q", got, oldURL.String())
	}
}

func TestDeleteUnreferencedFilesByFilesystem(t *testing.T) {
	cache := newTestFSCache(t)
	keepURL := mustParseURL(t, "https://example.com/pool/main/p/keep.deb")
	removeURL := mustParseURL(t, "https://example.com/pool/main/p/remove.deb")

	keepPath := cache.buildLocalPath(keepURL)
	removePath := cache.buildLocalPath(removeURL)

	if err := os.MkdirAll(filepath.Dir(keepPath), 0o755); err != nil {
		t.Fatalf("mkdir keep failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(removePath), 0o755); err != nil {
		t.Fatalf("mkdir remove failed: %v", err)
	}
	if err := os.WriteFile(keepPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep failed: %v", err)
	}
	if err := os.WriteFile(removePath, []byte("remove"), 0o644); err != nil {
		t.Fatalf("write remove failed: %v", err)
	}

	if err := cache.Set(DetermineProtocolFromURL(keepURL), keepURL.Host, keepURL.Path, AccessEntry{URL: keepURL}); err != nil {
		t.Fatalf("Set(keep) error = %v", err)
	}

	if err := cache.deleteUnreferencedFilesByFilesystem(); err != nil {
		t.Fatalf("deleteUnreferencedFilesByFilesystem() error = %v", err)
	}

	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected keep file to remain, stat error = %v", err)
	}
	if _, err := os.Stat(removePath); !os.IsNotExist(err) {
		t.Fatalf("expected remove file to be deleted, stat error = %v", err)
	}
}

func TestGetFilesInCacheDirectorySkipsMetadataFiles(t *testing.T) {
	cache := newTestFSCache(t)
	dataFile := filepath.Join(cache.CachePath, "example.com", "pool", "main", "p", "pkg.deb")
	metaFile := dataFile + accessCacheMetaSuffix

	if err := os.MkdirAll(filepath.Dir(dataFile), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(dataFile, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write data failed: %v", err)
	}
	if err := os.WriteFile(metaFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write metadata failed: %v", err)
	}

	files, err := cache.getFilesInCacheDirectory()
	if err != nil {
		t.Fatalf("getFilesInCacheDirectory() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files len = %d, want 1", len(files))
	}
	if got := files[0]; got != filepath.Join("example.com", "pool", "main", "p", "pkg.deb") {
		t.Fatalf("file = %q, want pkg.deb entry", got)
	}
}

func TestDeleteUnreferencedFilesByMetadataReturnsNil(t *testing.T) {
	cache := newTestFSCache(t)
	u := mustParseURL(t, "https://example.com/pool/main/p/missing.deb")
	if err := cache.Set(DetermineProtocolFromURL(u), u.Host, u.Path, AccessEntry{URL: u, Size: 99}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := cache.deleteUnreferencedFilesByMetadata(); err != nil {
		t.Fatalf("deleteUnreferencedFilesByMetadata() error = %v", err)
	}

	if err := cache.DeleteUnreferencedFiles(); err != nil {
		t.Fatalf("DeleteUnreferencedFiles() error = %v", err)
	}
}
