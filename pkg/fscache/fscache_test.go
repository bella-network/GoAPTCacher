package fscache

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestFSCache(t *testing.T) *FSCache {
	t.Helper()
	return NewFSCache(t.TempDir())
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("failed to parse url %q: %v", raw, err)
	}
	return u
}

func TestDetermineProtocol(t *testing.T) {
	tcs := []struct {
		name     string
		protocol string
		want     int
	}{
		{"HTTPS lower", "https", 1},
		{"HTTPS upper", "HTTPS", 1},
		{"HTTP", "http", 0},
		{"Unknown", "ftp", 0},
		{"Empty", "", 0},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetermineProtocol(tc.protocol); got != tc.want {
				t.Fatalf("DetermineProtocol(%q) = %d, want %d", tc.protocol, got, tc.want)
			}
		})
	}
}

func TestDetermineProtocolFromURL(t *testing.T) {
	assertProtocol := func(raw string, want int) {
		t.Helper()
		u := mustParseURL(t, raw)
		if got := DetermineProtocolFromURL(u); got != want {
			t.Fatalf("DetermineProtocolFromURL(%q) = %d, want %d", raw, got, want)
		}
	}

	assertProtocol("https://example.com/foo", 1)
	assertProtocol("http://example.com/foo", 0)
	assertProtocol("ftp://example.com/foo", 0)
}

func TestBuildLocalPath(t *testing.T) {
	cache := newTestFSCache(t)

	u := mustParseURL(t, "https://cdn.example.com/debian/Release")
	got := cache.buildLocalPath(u)
	want := filepath.Join(cache.CachePath, "cdn.example.com", "debian", "Release")
	if got != want {
		t.Fatalf("buildLocalPath() = %q, want %q", got, want)
	}
}

func TestBuildLocalPathPreventsTraversal(t *testing.T) {
	cache := newTestFSCache(t)

	u := mustParseURL(t, "https://cdn.example.com/../../../../tmp/pwn")
	got := cache.buildLocalPath(u)

	rel, err := filepath.Rel(cache.CachePath, got)
	if err != nil {
		t.Fatalf("filepath.Rel() error = %v", err)
	}
	if strings.HasPrefix(rel, "..") {
		t.Fatalf("buildLocalPath() escaped cache directory: %q", got)
	}

	want := filepath.Join(cache.CachePath, "cdn.example.com", "tmp", "pwn")
	if got != want {
		t.Fatalf("buildLocalPath() = %q, want %q", got, want)
	}
}

func TestBuildLocalPathNormalizesEncodedTraversalAndBackslashes(t *testing.T) {
	cache := newTestFSCache(t)

	u := mustParseURL(t, "https://cdn.example.com/%2e%2e/%2e%2e/dir%5c..%5ctest/file")
	got := cache.buildLocalPath(u)

	rel, err := filepath.Rel(cache.CachePath, got)
	if err != nil {
		t.Fatalf("filepath.Rel() error = %v", err)
	}
	if strings.HasPrefix(rel, "..") {
		t.Fatalf("buildLocalPath() escaped cache directory: %q", got)
	}

	want := filepath.Join(cache.CachePath, "cdn.example.com", "test", "file")
	if got != want {
		t.Fatalf("buildLocalPath() = %q, want %q", got, want)
	}
}

func TestBuildLocalPathWithCustomFunc(t *testing.T) {
	cache := newTestFSCache(t)
	cache.CustomCachePath = func(u *url.URL) string {
		return "custom-path"
	}

	got := cache.buildLocalPath(mustParseURL(t, "https://example.com/foo"))
	if got != "custom-path" {
		t.Fatalf("buildLocalPath() = %q, want %q", got, "custom-path")
	}
}

func TestValidateRequest(t *testing.T) {
	cache := newTestFSCache(t)

	t.Run("valid host", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/pkg.deb", nil)
		if err := cache.validateRequest(req); err != nil {
			t.Fatalf("validateRequest() error = %v", err)
		}
	})

	t.Run("set from request host", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/pkg.deb", nil)
		req.URL.Host = ""

		if err := cache.validateRequest(req); err != nil {
			t.Fatalf("validateRequest() error = %v", err)
		}
		if req.URL.Host != "example.com" {
			t.Fatalf("expected URL host to be populated, got %q", req.URL.Host)
		}
	})

	t.Run("invalid host", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/pkg.deb", nil)
		req.URL.Host = ""
		req.Host = "bad host"
		if err := cache.validateRequest(req); err == nil {
			t.Fatalf("expected error for invalid host")
		}
	})
}

func TestEvaluateRefreshDefaultInterval(t *testing.T) {
	cache := newTestFSCache(t)
	now := time.Now()
	entry := AccessEntry{
		LastChecked: now.Add(-23 * time.Hour),
	}
	if cache.evaluateRefresh(mustParseURL(t, "https://example.com/dists/stable/file.txt"), entry) {
		t.Fatalf("expected no refresh for recently checked file")
	}

	entry.LastChecked = now.Add(-25 * time.Hour)
	if !cache.evaluateRefresh(mustParseURL(t, "https://example.com/dists/stable/file.txt"), entry) {
		t.Fatalf("expected refresh when last check exceeded default interval")
	}
}

func TestEvaluateRefreshRefreshFilesShortInterval(t *testing.T) {
	cache := newTestFSCache(t)
	now := time.Now()
	entry := AccessEntry{
		LastChecked: now.Add(-3 * time.Minute),
	}
	if cache.evaluateRefresh(mustParseURL(t, "https://example.com/dists/stable/Release"), entry) {
		t.Fatalf("expected no refresh for Release checked recently")
	}

	entry.LastChecked = now.Add(-6 * time.Minute)
	if !cache.evaluateRefresh(mustParseURL(t, "https://example.com/dists/stable/Release"), entry) {
		t.Fatalf("expected refresh for Release beyond short interval")
	}
}

func TestGetFileByPath(t *testing.T) {
	cache := newTestFSCache(t)

	t.Run("relative path", func(t *testing.T) {
		u, ok := cache.GetFileByPath("https://example.com/repo/Release", "Packages.gz")
		if !ok {
			t.Fatalf("expected success resolving relative path")
		}
		if u.String() != "https://example.com/repo/Packages.gz" {
			t.Fatalf("unexpected resolved URL %q", u.String())
		}
	})

	t.Run("absolute URL", func(t *testing.T) {
		u, ok := cache.GetFileByPath("https://example.com/repo/Release", "https://mirror.example.org/other/Packages")
		if !ok {
			t.Fatalf("expected success resolving absolute URL")
		}
		if u.String() != "https://mirror.example.org/other/Packages" {
			t.Fatalf("unexpected resolved URL %q", u.String())
		}
	})

	t.Run("invalid absolute URL", func(t *testing.T) {
		if _, ok := cache.GetFileByPath("https://example.com/repo/Release", "http://%zz"); ok {
			t.Fatalf("expected failure resolving invalid absolute URL")
		}
	})
}

func TestCopyResponseHeaders(t *testing.T) {
	src := map[string][]string{
		"Content-Type":        {"application/octet-stream"},
		"Cache-Control":       {"public"},
		"Connection":          {"keep-alive"},
		"Transfer-Encoding":   {"chunked"},
		"X-Custom-Header":     {"value"},
		"Proxy-Authorization": {"secret"},
	}

	dst := make(map[string][]string)
	copyResponseHeaders(dst, src)

	if got := dst["Content-Type"]; len(got) != 1 || got[0] != "application/octet-stream" {
		t.Fatalf("Content-Type header not copied correctly: %v", got)
	}
	if got := dst["X-Custom-Header"]; len(got) != 1 || got[0] != "value" {
		t.Fatalf("X-Custom-Header header not copied correctly: %v", got)
	}

	forbidden := []string{"Connection", "Transfer-Encoding", "Proxy-Authorization"}
	for _, header := range forbidden {
		if _, ok := dst[header]; ok {
			t.Fatalf("hop-by-hop header %q should not be copied", header)
		}
	}
}

func TestGenerateSHA256Hash(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.txt")
	content := "goaptcacher"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got, err := GenerateSHA256Hash(file)
	if err != nil {
		t.Fatalf("GenerateSHA256Hash() error = %v", err)
	}

	want := "882fbc55635ef37ccc2cd6177507a8e01d3eba9ce960d9139e6270e0de8d0839"
	if !strings.EqualFold(got, want) {
		t.Fatalf("GenerateSHA256Hash() = %q, want %q", got, want)
	}
}

func TestEnsureDiskSpace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.dat")

	// Zero requirement should always succeed.
	if err := ensureDiskSpace(path, 0); err != nil {
		t.Fatalf("ensureDiskSpace() zero requirement returned error: %v", err)
	}

	// Small requirement should succeed on test environment.
	if err := ensureDiskSpace(path, 64); err != nil {
		t.Fatalf("ensureDiskSpace() small requirement returned error: %v", err)
	}
}

func TestPreallocateFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "prealloc-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()

	if err := preallocateFile(file, 1024); err != nil {
		t.Fatalf("preallocateFile() error = %v", err)
	}

	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat temp file: %v", err)
	}
	if info.Size() < 1024 {
		t.Fatalf("expected file size >= 1024, got %d", info.Size())
	}
}

func TestFileLocks(t *testing.T) {
	cache := newTestFSCache(t)
	const (
		protocol = 1
		domain   = "example.com"
		path     = "/foo"
	)

	cache.CreateFileLock(protocol, domain, path)
	if ok, _ := cache.HasFileLock(protocol, domain, path); !ok {
		t.Fatalf("expected read lock to be present")
	}
	cache.RemoveFileLock(protocol, domain, path)
	if ok, _ := cache.HasFileLock(protocol, domain, path); ok {
		t.Fatalf("expected read lock to be removed")
	}
}

func TestCreateExclusiveWriteLock(t *testing.T) {
	cache := newTestFSCache(t)
	const (
		protocol = 1
		domain   = "example.com"
		path     = "/bar"
	)

	if ok := cache.CreateExclusiveWriteLock(protocol, domain, path); !ok {
		t.Fatalf("expected exclusive write lock to be created")
	}

	if ok, _ := cache.HasWriteLock(protocol, domain, path); !ok {
		t.Fatalf("expected write lock to be recorded")
	}

	cache.DeleteWriteLock(protocol, domain, path)

	cache.CreateFileLock(protocol, domain, path)
	if ok := cache.CreateExclusiveWriteLock(protocol, domain, path); ok {
		t.Fatalf("expected exclusive write lock to fail while read lock exists")
	}
	cache.RemoveFileLock(protocol, domain, path)

	if err := cache.CreateWriteLock(protocol, domain, path); err != nil {
		t.Fatalf("CreateWriteLock failed: %v", err)
	}
	if ok := cache.CreateExclusiveWriteLock(protocol, domain, path); ok {
		t.Fatalf("expected exclusive write lock to fail while write lock exists")
	}
	cache.DeleteWriteLock(protocol, domain, path)
}

func TestGenerateUUID(t *testing.T) {
	cache := newTestFSCache(t)
	id := cache.GenerateUUID()
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("GenerateUUID returned invalid UUID: %v", err)
	}
}
