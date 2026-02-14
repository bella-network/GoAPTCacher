package fscache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAccessCacheRecordFromFileDerivesFromMetaPath(t *testing.T) {
	cache := newTestFSCache(t)
	metaPath := filepath.Join(cache.CachePath, "example.com", "pool", "main", "p", "pkg.deb") + accessCacheMetaSuffix

	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(metaPath, []byte(`{"last_accessed":"2026-02-10T10:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("write metadata failed: %v", err)
	}

	record, ok := cache.loadAccessCacheRecordFromFile(metaPath)
	if !ok {
		t.Fatalf("loadAccessCacheRecordFromFile() ok = false, want true")
	}
	if record.domain != "example.com" {
		t.Fatalf("domain = %q, want %q", record.domain, "example.com")
	}
	if record.path != "/pool/main/p/pkg.deb" {
		t.Fatalf("path = %q, want %q", record.path, "/pool/main/p/pkg.deb")
	}
	if record.protocol != 0 {
		t.Fatalf("protocol = %d, want 0", record.protocol)
	}
	if record.entry.URL == nil || record.entry.URL.String() != "http://example.com/pool/main/p/pkg.deb" {
		t.Fatalf("entry.URL = %#v, want fallback URL", record.entry.URL)
	}
}

func TestLoadAccessCacheRecordFromFileUsesURLPayload(t *testing.T) {
	cache := newTestFSCache(t)
	metaPath := filepath.Join(cache.CachePath, "ignored.example", "path", "file") + accessCacheMetaSuffix

	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	payload := `{"url":"https://mirror.example.org/debian/Release","protocol":0,"domain":"wrong.example","path":"/wrong"}`
	if err := os.WriteFile(metaPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("write metadata failed: %v", err)
	}

	record, ok := cache.loadAccessCacheRecordFromFile(metaPath)
	if !ok {
		t.Fatalf("loadAccessCacheRecordFromFile() ok = false, want true")
	}
	if record.protocol != 1 {
		t.Fatalf("protocol = %d, want 1", record.protocol)
	}
	if record.domain != "mirror.example.org" {
		t.Fatalf("domain = %q, want %q", record.domain, "mirror.example.org")
	}
	if record.path != "/debian/Release" {
		t.Fatalf("path = %q, want %q", record.path, "/debian/Release")
	}
}

func TestGetURLBuildsHTTPAndHTTPS(t *testing.T) {
	cache := newTestFSCache(t)

	if got := cache.GetURL(0, "example.com", "/pool/pkg.deb"); got != "http://example.com/pool/pkg.deb" {
		t.Fatalf("GetURL(http) = %q", got)
	}
	if got := cache.GetURL(1, "example.com", "/pool/pkg.deb"); got != "https://example.com/pool/pkg.deb" {
		t.Fatalf("GetURL(https) = %q", got)
	}
}
