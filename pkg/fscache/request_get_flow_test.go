package fscache

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServeLocalFileSuccess(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/pool/main/p/pkg.deb", nil)
	localPath := cache.buildLocalPath(req.URL)

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(localPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rr := httptest.NewRecorder()
	cache.serveLocalFile(rr, req, localPath)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("X-Cache"); got != "HIT" {
		t.Fatalf("X-Cache = %q, want HIT", got)
	}
	if got := rr.Body.String(); got != "payload" {
		t.Fatalf("body = %q, want %q", got, "payload")
	}
	if hasLock, _ := cache.HasFileLock(DetermineProtocolFromURL(req.URL), req.URL.Host, req.URL.Path); hasLock {
		t.Fatalf("expected file lock to be released")
	}
}

func TestServeLocalFileMissingFile(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/pool/main/p/missing.deb", nil)
	rr := httptest.NewRecorder()

	cache.serveLocalFile(rr, req, cache.buildLocalPath(req.URL))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rr.Body.String(), "Error accessing cached file") {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}

func TestBackgroundFileTasksNoEntry(t *testing.T) {
	cache := newTestFSCache(t)
	cache.backgroundFileTasks(mustParseURL(t, "https://example.com/pool/main/p/pkg.deb"))
}

func TestBackgroundFileTasksUpdatesAccessData(t *testing.T) {
	cache := newTestFSCache(t)
	reqURL := mustParseURL(t, "https://example.com/pool/main/p/pkg.deb")
	protocol := DetermineProtocolFromURL(reqURL)
	oldURL := mustParseURL(t, "https://mirror.example.org/other/pkg.deb")
	before := time.Now().Add(-time.Hour)

	if err := cache.Set(protocol, reqURL.Host, reqURL.Path, AccessEntry{
		URL:          oldURL,
		LastAccessed: before,
		LastChecked:  time.Now(),
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	cache.backgroundFileTasks(reqURL)

	deadline := time.Now().Add(2 * time.Second)
	updated := false
	for time.Now().Before(deadline) {
		entry, ok := cache.Get(protocol, reqURL.Host, reqURL.Path)
		if ok && entry.LastAccessed.After(before) && entry.URL != nil && entry.URL.String() == reqURL.String() {
			updated = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !updated {
		t.Fatalf("background tasks did not update LastAccessed and URL in time")
	}
}

func TestServeGETRequestPoolDirectHit(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/pool/main/p/pkg.deb", nil)
	localPath := cache.buildLocalPath(req.URL)

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(localPath, []byte("pkg-data"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rr := httptest.NewRecorder()
	cache.serveGETRequest(req, rr)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "pkg-data" {
		t.Fatalf("body = %q, want %q", rr.Body.String(), "pkg-data")
	}
	if got := rr.Header().Get("X-Cache"); got != "HIT" {
		t.Fatalf("X-Cache = %q, want HIT", got)
	}
}

func TestServeGETRequestStaleEntryTriggersMissAndCleanup(t *testing.T) {
	cache := newTestFSCache(t)
	cache.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream down")
	})}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/dists/stable/Release", nil)
	localPath := cache.buildLocalPath(req.URL)
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(localPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	protocol := DetermineProtocolFromURL(req.URL)
	if err := cache.Set(protocol, req.URL.Host, req.URL.Path, AccessEntry{
		URL:  req.URL,
		Size: 99,
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	rr := httptest.NewRecorder()
	cache.serveGETRequest(req, rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed, stat err = %v", err)
	}
	if _, ok := cache.Get(protocol, req.URL.Host, req.URL.Path); ok {
		t.Fatalf("expected stale metadata to be removed")
	}
}

func TestServeGETRequestMissFetchError(t *testing.T) {
	cache := newTestFSCache(t)
	cache.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("fetch failed")
	})}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/dists/stable/InRelease", nil)
	rr := httptest.NewRecorder()
	cache.serveGETRequest(req, rr)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestServeGETRequestCacheMissRetryLimit(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/dists/stable/InRelease", nil)
	rr := httptest.NewRecorder()

	cache.serveGETRequestCacheMiss(req, rr, 26)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rr.Body.String(), "currently being downloaded") {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}

func TestServeGETRequestCacheMissLockContentionPath(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/dists/stable/InRelease", nil)
	protocol := DetermineProtocolFromURL(req.URL)

	if err := cache.CreateWriteLock(protocol, req.URL.Host, req.URL.Path); err != nil {
		t.Fatalf("CreateWriteLock() error = %v", err)
	}
	defer cache.DeleteWriteLock(protocol, req.URL.Host, req.URL.Path)

	rr := httptest.NewRecorder()
	cache.serveGETRequestCacheMissWithSleep(req, rr, 25, func(time.Duration) {})

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestServeGETRequestCacheMissUsesExistingFileRoundtrip(t *testing.T) {
	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/dists/stable/Release", nil)
	localPath := cache.buildLocalPath(req.URL)
	content := []byte("release-data")

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(localPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rr := httptest.NewRecorder()
	cache.serveGETRequestCacheMiss(req, rr, 0)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != string(content) {
		t.Fatalf("body = %q, want %q", rr.Body.String(), string(content))
	}

	entry, ok := cache.Get(DetermineProtocolFromURL(req.URL), req.URL.Host, req.URL.Path)
	if !ok {
		t.Fatalf("expected metadata entry to be created")
	}
	if entry.Size != int64(len(content)) {
		t.Fatalf("entry size = %d, want %d", entry.Size, len(content))
	}
	if entry.SHA256 == "" {
		t.Fatalf("expected SHA256 to be populated")
	}
}

func TestServeGETRequestCacheMissDownloadAndCache(t *testing.T) {
	const payload = "upstream-payload"
	lastModified := time.Now().UTC().Truncate(time.Second)

	var gotCustomHeader string
	var gotIfModifiedSince string
	var gotIfNoneMatch string
	var gotForwardedFor string
	var gotProxyServer string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustomHeader = r.Header.Get("X-Custom-Req")
		gotIfModifiedSince = r.Header.Get("If-Modified-Since")
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		gotProxyServer = r.Header.Get("X-Proxy-Server")

		w.Header().Set("ETag", "\"upstream-etag\"")
		w.Header().Set("Last-Modified", lastModified.Format(time.RFC1123))
		w.Header().Set("X-Upstream", "ok")
		w.Header().Set("Connection", "close")
		_, _ = io.WriteString(w, payload)
	}))
	defer upstream.Close()

	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodGet, upstream.URL+"/debian/dists/stable/InRelease", nil)
	req.Header.Set("If-Modified-Since", time.Now().Add(-time.Hour).Format(time.RFC1123))
	req.Header.Set("If-None-Match", "\"etag\"")
	req.Header.Set("E-Tag", "\"legacy\"")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("X-Custom-Req", "keep-me")

	rr := httptest.NewRecorder()
	cache.serveGETRequestCacheMiss(req, rr, 0)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != payload {
		t.Fatalf("body = %q, want %q", rr.Body.String(), payload)
	}
	if got := rr.Header().Get("X-Cache"); got != "MISS" {
		t.Fatalf("X-Cache = %q, want MISS", got)
	}
	if got := rr.Header().Get("ETag"); got != "\"upstream-etag\"" {
		t.Fatalf("ETag = %q, want %q", got, "\"upstream-etag\"")
	}
	if got := rr.Header().Get("X-Upstream"); got != "ok" {
		t.Fatalf("X-Upstream = %q, want ok", got)
	}
	if got := rr.Header().Get("Connection"); got != "" {
		t.Fatalf("expected Connection header to be stripped, got %q", got)
	}

	if gotCustomHeader != "keep-me" {
		t.Fatalf("upstream X-Custom-Req = %q, want keep-me", gotCustomHeader)
	}
	if gotIfModifiedSince != "" {
		t.Fatalf("expected If-Modified-Since to be stripped, got %q", gotIfModifiedSince)
	}
	if gotIfNoneMatch != "" {
		t.Fatalf("expected If-None-Match to be stripped, got %q", gotIfNoneMatch)
	}
	if gotForwardedFor == "" {
		t.Fatalf("expected X-Forwarded-For to be set")
	}
	if !strings.Contains(gotProxyServer, "GoAptCacher/") {
		t.Fatalf("unexpected X-Proxy-Server header: %q", gotProxyServer)
	}

	targetPath := cache.buildLocalPath(req.URL)
	cached, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile(cached) error = %v", err)
	}
	if string(cached) != payload {
		t.Fatalf("cached body = %q, want %q", string(cached), payload)
	}

	entry, ok := cache.Get(DetermineProtocolFromURL(req.URL), req.URL.Host, req.URL.Path)
	if !ok {
		t.Fatalf("expected metadata entry after download")
	}
	if entry.ETag != "\"upstream-etag\"" {
		t.Fatalf("entry ETag = %q, want %q", entry.ETag, "\"upstream-etag\"")
	}
	if entry.Size != int64(len(payload)) {
		t.Fatalf("entry size = %d, want %d", entry.Size, len(payload))
	}
}

func TestServeGETRequestCacheMissUpstreamStatusError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "not found")
	}))
	defer upstream.Close()

	cache := newTestFSCache(t)
	req := httptest.NewRequest(http.MethodGet, upstream.URL+"/missing", nil)
	rr := httptest.NewRecorder()

	cache.serveGETRequestCacheMiss(req, rr, 0)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
	if !strings.Contains(rr.Body.String(), "Error fetching file") {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}
