package fscache

import (
	"os"
	"testing"
	"time"
)

func TestSetGetSHA256(t *testing.T) {
	cache := newTestFSCache(t)
	const (
		protocol = 0
		domain   = "example.com"
		path     = "/pool/main/p/pkg.deb"
	)

	if _, ok := cache.GetSHA256(protocol, domain, path); ok {
		t.Fatalf("expected missing SHA256 lookup to return ok=false")
	}

	if err := cache.SetSHA256(protocol, domain, path, "abc123"); err != nil {
		t.Fatalf("SetSHA256() returned error: %v", err)
	}

	sha, ok := cache.GetSHA256(protocol, domain, path)
	if !ok {
		t.Fatalf("expected SHA256 entry to exist")
	}
	if sha != "abc123" {
		t.Fatalf("SHA256 = %q, want %q", sha, "abc123")
	}

	entry, ok := cache.Get(protocol, domain, path)
	if !ok {
		t.Fatalf("expected entry to exist")
	}
	if entry.URL == nil || entry.URL.String() != "http://example.com/pool/main/p/pkg.deb" {
		t.Fatalf("entry.URL = %#v, want %q", entry.URL, "http://example.com/pool/main/p/pkg.deb")
	}
}

func TestHitAndUpdateLastChecked(t *testing.T) {
	cache := newTestFSCache(t)
	u := mustParseURL(t, "https://example.com/dists/stable/InRelease")
	protocol := DetermineProtocolFromURL(u)

	initial := AccessEntry{
		URL:          u,
		LastAccessed: time.Now().Add(-10 * time.Minute),
		LastChecked:  time.Now().Add(-10 * time.Minute),
	}
	if err := cache.Set(protocol, u.Host, u.Path, initial); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	before, ok := cache.Get(protocol, u.Host, u.Path)
	if !ok {
		t.Fatalf("expected entry to exist before update")
	}

	if err := cache.Hit(protocol, u.Host, u.Path); err != nil {
		t.Fatalf("Hit() returned error: %v", err)
	}
	if err := cache.UpdateLastChecked(protocol, u.Host, u.Path); err != nil {
		t.Fatalf("UpdateLastChecked() returned error: %v", err)
	}

	after, ok := cache.Get(protocol, u.Host, u.Path)
	if !ok {
		t.Fatalf("expected entry to exist after update")
	}
	if !after.LastAccessed.After(before.LastAccessed) {
		t.Fatalf("LastAccessed was not updated: before=%v after=%v", before.LastAccessed, after.LastAccessed)
	}
	if !after.LastChecked.After(before.LastChecked) {
		t.Fatalf("LastChecked was not updated: before=%v after=%v", before.LastChecked, after.LastChecked)
	}
}

func TestHitAndUpdateLastCheckedMissingEntry(t *testing.T) {
	cache := newTestFSCache(t)
	if err := cache.Hit(0, "example.com", "/missing"); err != nil {
		t.Fatalf("Hit() returned unexpected error: %v", err)
	}
	if err := cache.UpdateLastChecked(0, "example.com", "/missing"); err != nil {
		t.Fatalf("UpdateLastChecked() returned unexpected error: %v", err)
	}
}

func TestDeleteRemovesPersistedMetadata(t *testing.T) {
	cache := newTestFSCache(t)
	u := mustParseURL(t, "https://example.com/dists/stable/Release")
	protocol := DetermineProtocolFromURL(u)

	if err := cache.Set(protocol, u.Host, u.Path, AccessEntry{URL: u}); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}
	cache.flushAccessCache()

	metaPath := cache.accessCacheMetaPath(protocol, u.Host, u.Path)
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("expected metadata file to exist before delete, got err=%v", err)
	}

	cache.Delete(protocol, u.Host, u.Path)

	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Fatalf("expected metadata file to be removed, stat err=%v", err)
	}
	if _, ok := cache.Get(protocol, u.Host, u.Path); ok {
		t.Fatalf("expected entry to be deleted from cache")
	}
}

func TestMarkForDeletionSetsFlags(t *testing.T) {
	cache := newTestFSCache(t)
	u := mustParseURL(t, "https://example.com/dists/stable/Release")
	protocol := DetermineProtocolFromURL(u)

	if err := cache.Set(protocol, u.Host, u.Path, AccessEntry{URL: u}); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	cache.MarkForDeletion(protocol, u.Host, u.Path)

	record, ok := cache.getAccessCacheRecord(protocol, u.Host, u.Path)
	if !ok {
		t.Fatalf("expected record to exist")
	}
	if !record.markedForDeletion {
		t.Fatalf("expected record to be marked for deletion")
	}
	if record.markedAt.IsZero() {
		t.Fatalf("expected markedAt to be set")
	}
}

func TestAddURLIfNotExistsUsesFallbackForInvalidURL(t *testing.T) {
	cache := newTestFSCache(t)
	const (
		protocol = 1
		domain   = "example.com"
		path     = "/pool/main/p/pkg.deb"
	)

	if err := cache.AddURLIfNotExists(protocol, domain, path, "://bad"); err != nil {
		t.Fatalf("AddURLIfNotExists() returned error: %v", err)
	}

	entry, ok := cache.Get(protocol, domain, path)
	if !ok {
		t.Fatalf("expected entry to exist")
	}
	if entry.URL == nil || entry.URL.String() != "https://example.com/pool/main/p/pkg.deb" {
		t.Fatalf("entry.URL = %#v, want %q", entry.URL, "https://example.com/pool/main/p/pkg.deb")
	}

	if err := cache.AddURLIfNotExists(protocol, domain, path, "https://mirror.example.org/alt/pkg.deb"); err != nil {
		t.Fatalf("AddURLIfNotExists() returned error: %v", err)
	}
	entry, ok = cache.Get(protocol, domain, path)
	if !ok || entry.URL == nil || entry.URL.String() != "https://mirror.example.org/alt/pkg.deb" {
		t.Fatalf("entry.URL after update = %#v, want %q", entry.URL, "https://mirror.example.org/alt/pkg.deb")
	}
}
