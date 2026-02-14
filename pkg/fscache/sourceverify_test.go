package fscache

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifySourcesMarksDebMissingFromPackagesIndex(t *testing.T) {
	const (
		releasePath      = "/debian/dists/stable/InRelease"
		packagesPath     = "/debian/dists/stable/main/binary-amd64/Packages"
		missingDebPath   = "/debian/pool/main/m/missing/missing_1.0_amd64.deb"
		packagesFilename = "pool/main/h/hello/hello_1.0_amd64.deb"
	)

	releaseBody := "SHA256:\n 1111111111111111111111111111111111111111111111111111111111111111 123 main/binary-amd64/Packages\n"
	packagesBody := "Package: hello\nFilename: " + packagesFilename + "\nSHA256: abcdef\n\n"

	cache := newTestFSCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case releasePath:
			_, _ = w.Write([]byte(releaseBody))
		case packagesPath:
			_, _ = w.Write([]byte(packagesBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cache.client = server.Client()

	releaseURL := mustParseURL(t, server.URL+releasePath)
	missingDebURL := mustParseURL(t, server.URL+missingDebPath)
	protocol := DetermineProtocolFromURL(releaseURL)

	if err := cache.Set(protocol, releaseURL.Host, releaseURL.Path, AccessEntry{URL: releaseURL}); err != nil {
		t.Fatalf("failed to seed release entry: %v", err)
	}
	if err := cache.Set(protocol, missingDebURL.Host, missingDebURL.Path, AccessEntry{URL: missingDebURL}); err != nil {
		t.Fatalf("failed to seed deb entry: %v", err)
	}

	if err := cache.verifySources(); err != nil {
		t.Fatalf("verifySources() returned error: %v", err)
	}

	record, ok := cache.getAccessCacheRecord(protocol, missingDebURL.Host, missingDebURL.Path)
	if !ok {
		t.Fatalf("expected deb access cache record to exist")
	}
	if !record.markedForDeletion {
		t.Fatalf("expected missing deb to be marked for deletion")
	}
}

func TestVerifySourcesMarksDebChecksumMismatch(t *testing.T) {
	const (
		releasePath  = "/debian/dists/stable/InRelease"
		packagesPath = "/debian/dists/stable/main/binary-amd64/Packages"
		debPath      = "/debian/pool/main/h/hello/hello_1.0_amd64.deb"
	)

	expectedHash := checksumHex("expected content")
	releaseBody := "SHA256:\n 1111111111111111111111111111111111111111111111111111111111111111 123 main/binary-amd64/Packages\n"
	packagesBody := "Package: hello\nFilename: pool/main/h/hello/hello_1.0_amd64.deb\nSHA256: " + expectedHash + "\n\n"

	cache := newTestFSCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case releasePath:
			_, _ = w.Write([]byte(releaseBody))
		case packagesPath:
			_, _ = w.Write([]byte(packagesBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cache.client = server.Client()

	releaseURL := mustParseURL(t, server.URL+releasePath)
	debURL := mustParseURL(t, server.URL+debPath)
	protocol := DetermineProtocolFromURL(releaseURL)

	if err := cache.Set(protocol, releaseURL.Host, releaseURL.Path, AccessEntry{URL: releaseURL}); err != nil {
		t.Fatalf("failed to seed release entry: %v", err)
	}
	if err := cache.Set(protocol, debURL.Host, debURL.Path, AccessEntry{URL: debURL}); err != nil {
		t.Fatalf("failed to seed deb entry: %v", err)
	}

	localDebPath := cache.buildLocalPath(debURL)
	if err := os.MkdirAll(filepath.Dir(localDebPath), 0o755); err != nil {
		t.Fatalf("failed to create deb parent directory: %v", err)
	}
	if err := os.WriteFile(localDebPath, []byte("actual content"), 0o644); err != nil {
		t.Fatalf("failed to write local deb file: %v", err)
	}

	if err := cache.verifySources(); err != nil {
		t.Fatalf("verifySources() returned error: %v", err)
	}

	record, ok := cache.getAccessCacheRecord(protocol, debURL.Host, debURL.Path)
	if !ok {
		t.Fatalf("expected deb access cache record to exist")
	}
	if !record.markedForDeletion {
		t.Fatalf("expected deb with checksum mismatch to be marked for deletion")
	}
}

func TestVerifySourcesKeepsDebWhenChecksumMatches(t *testing.T) {
	const (
		releasePath  = "/debian/dists/stable/InRelease"
		packagesPath = "/debian/dists/stable/main/binary-amd64/Packages"
		debPath      = "/debian/pool/main/h/hello/hello_1.0_amd64.deb"
	)

	localContent := "matching content"
	expectedHash := checksumHex(localContent)
	releaseBody := "SHA256:\n 1111111111111111111111111111111111111111111111111111111111111111 123 main/binary-amd64/Packages\n"
	packagesBody := strings.Join([]string{
		"Package: hello",
		"Filename: pool/main/h/hello/hello_1.0_amd64.deb",
		"SHA256: " + expectedHash,
		"",
	}, "\n")

	cache := newTestFSCache(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case releasePath:
			_, _ = w.Write([]byte(releaseBody))
		case packagesPath:
			_, _ = w.Write([]byte(packagesBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cache.client = server.Client()

	releaseURL := mustParseURL(t, server.URL+releasePath)
	debURL := mustParseURL(t, server.URL+debPath)
	protocol := DetermineProtocolFromURL(releaseURL)

	if err := cache.Set(protocol, releaseURL.Host, releaseURL.Path, AccessEntry{URL: releaseURL}); err != nil {
		t.Fatalf("failed to seed release entry: %v", err)
	}
	if err := cache.Set(protocol, debURL.Host, debURL.Path, AccessEntry{URL: debURL}); err != nil {
		t.Fatalf("failed to seed deb entry: %v", err)
	}

	localDebPath := cache.buildLocalPath(debURL)
	if err := os.MkdirAll(filepath.Dir(localDebPath), 0o755); err != nil {
		t.Fatalf("failed to create deb parent directory: %v", err)
	}
	if err := os.WriteFile(localDebPath, []byte(localContent), 0o644); err != nil {
		t.Fatalf("failed to write local deb file: %v", err)
	}

	if err := cache.verifySources(); err != nil {
		t.Fatalf("verifySources() returned error: %v", err)
	}

	record, ok := cache.getAccessCacheRecord(protocol, debURL.Host, debURL.Path)
	if !ok {
		t.Fatalf("expected deb access cache record to exist")
	}
	if record.markedForDeletion {
		t.Fatalf("expected deb with matching checksum to stay active")
	}
}

func checksumHex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
