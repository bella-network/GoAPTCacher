package fscache

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServeFromRequestInvalidHost(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/pkg.deb", nil)
	req.URL.Host = ""
	req.Host = ""
	rr := httptest.NewRecorder()

	cache.ServeFromRequest(req, rr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestServeFromRequestMethodNotAllowed(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/pkg.deb", nil)
	rr := httptest.NewRecorder()

	cache.ServeFromRequest(req, rr)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestServeFromRequestHEADUsesCacheHit(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodHead, "https://example.com/pool/main/p/pkg.deb", nil)
	localPath := cache.buildLocalPath(req.URL)

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("cached"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	rr := httptest.NewRecorder()
	cache.ServeFromRequest(req, rr)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("X-Cache"); got != "HIT" {
		t.Fatalf("X-Cache = %q, want HIT", got)
	}
}

func TestSetExpirationDaysUpdatesConfiguration(t *testing.T) {
	cache := newTestFSCache(t)

	cache.SetExpirationDays(3)
	if cache.expirationInDays != 3 {
		t.Fatalf("expirationInDays = %d, want 3", cache.expirationInDays)
	}

	cache.SetExpirationDays(7)
	if cache.expirationInDays != 7 {
		t.Fatalf("expirationInDays = %d, want 7", cache.expirationInDays)
	}
}
