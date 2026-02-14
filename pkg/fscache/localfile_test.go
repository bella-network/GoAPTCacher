package fscache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteFileDeletesCachedFileAndMetadata(t *testing.T) {
	cache := newTestFSCache(t)
	u := mustParseURL(t, "https://example.com/repo/pkg.deb")
	protocol := DetermineProtocolFromURL(u)

	localPath := cache.buildLocalPath(u)
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("failed creating parent directory: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("failed writing cached file: %v", err)
	}
	if err := cache.Set(protocol, u.Host, u.Path, AccessEntry{URL: u}); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	if err := cache.DeleteFile(u); err != nil {
		t.Fatalf("DeleteFile() returned error: %v", err)
	}

	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected cached file to be removed, stat err=%v", err)
	}
	if _, ok := cache.Get(protocol, u.Host, u.Path); ok {
		t.Fatalf("expected metadata entry to be removed")
	}
}

func TestDeleteFileMissingFileStillRemovesMetadata(t *testing.T) {
	cache := newTestFSCache(t)
	u := mustParseURL(t, "https://example.com/repo/missing.deb")
	protocol := DetermineProtocolFromURL(u)

	if err := cache.Set(protocol, u.Host, u.Path, AccessEntry{URL: u}); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	if err := cache.DeleteFile(u); err != nil {
		t.Fatalf("DeleteFile() returned error for missing file: %v", err)
	}
	if _, ok := cache.Get(protocol, u.Host, u.Path); ok {
		t.Fatalf("expected metadata entry to be removed even when file is missing")
	}
}

func TestDeleteFileReturnsUnexpectedRemoveError(t *testing.T) {
	cache := newTestFSCache(t)
	u := mustParseURL(t, "https://example.com/repo")
	protocol := DetermineProtocolFromURL(u)

	localPath := cache.buildLocalPath(u)
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		t.Fatalf("failed creating directory at file path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localPath, "child"), []byte("x"), 0o644); err != nil {
		t.Fatalf("failed creating nested file: %v", err)
	}
	if err := cache.Set(protocol, u.Host, u.Path, AccessEntry{URL: u}); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	if err := cache.DeleteFile(u); err == nil {
		t.Fatalf("expected DeleteFile() to return an error for non-empty directory")
	}
	if _, ok := cache.Get(protocol, u.Host, u.Path); !ok {
		t.Fatalf("expected metadata entry to remain when delete fails")
	}
}
