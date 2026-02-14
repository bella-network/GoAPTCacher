package fscache

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeHEADRequestWithDepsHit(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodHead, "https://example.com/pool/main/p/pkg.deb", nil)
	localFile := cache.buildLocalPath(req.URL)

	if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	payload := []byte("cached-data")
	if err := os.WriteFile(localFile, payload, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	rr := httptest.NewRecorder()
	downloadCalled := false
	cache.serveHEADRequestWithDeps(req, rr, os.Stat, func(_, _ string) error {
		downloadCalled = true
		return nil
	})

	if downloadCalled {
		t.Fatalf("download should not be called on cache hit")
	}
	if got := rr.Header().Get("X-Cache"); got != "HIT" {
		t.Fatalf("X-Cache = %q, want HIT", got)
	}
	if got := rr.Header().Get("Content-Length"); got != "11" {
		t.Fatalf("Content-Length = %q, want 11", got)
	}
}

func TestServeHEADRequestWithDepsMissDownloadSuccess(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodHead, "https://example.com/dists/stable/Release", nil)
	localFile := cache.buildLocalPath(req.URL)

	rr := httptest.NewRecorder()
	cache.serveHEADRequestWithDeps(req, rr, os.Stat, func(_, path string) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte("release"), 0o644)
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("X-Cache"); got != "MISS" {
		t.Fatalf("X-Cache = %q, want MISS", got)
	}
	if got := rr.Header().Get("Content-Length"); got != "7" {
		t.Fatalf("Content-Length = %q, want 7", got)
	}

	if _, err := os.Stat(localFile); err != nil {
		t.Fatalf("expected downloaded file to exist: %v", err)
	}
}

func TestServeHEADRequestWithDepsDownloadError(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodHead, "https://example.com/dists/stable/Release", nil)
	rr := httptest.NewRecorder()

	cache.serveHEADRequestWithDeps(req, rr, func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}, func(_, _ string) error {
		return errors.New("download failed")
	})

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rr.Body.String(), "Error downloading file") {
		t.Fatalf("unexpected response body %q", rr.Body.String())
	}
}

func TestServeHEADRequestWithDepsReadErrorAfterDownload(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodHead, "https://example.com/dists/stable/Release", nil)
	rr := httptest.NewRecorder()

	calls := 0
	cache.serveHEADRequestWithDeps(req, rr, func(string) (os.FileInfo, error) {
		calls++
		if calls == 1 {
			return nil, os.ErrNotExist
		}
		return nil, errors.New("stat failed")
	}, func(_, _ string) error {
		return nil
	})

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rr.Body.String(), "Error reading file") {
		t.Fatalf("unexpected response body %q", rr.Body.String())
	}
}

func TestServeHEADRequestWrapperUsesDefaultDeps(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodHead, "https://example.com/pool/main/p/pkg.deb", nil)
	localFile := cache.buildLocalPath(req.URL)

	if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(localFile, []byte("cached-data"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	rr := httptest.NewRecorder()
	cache.serveHEADRequest(req, rr)

	if got := rr.Header().Get("X-Cache"); got != "HIT" {
		t.Fatalf("X-Cache = %q, want HIT", got)
	}
}
