package fscache

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNormalizeAccessEntryKeepsUnknownRemoteLastModified(t *testing.T) {
	cache := newTestFSCache(t)
	now := time.Now()

	normalized := cache.normalizeAccessEntry(0, "example.com", "/dists/stable/InRelease", AccessEntry{
		LastAccessed: now,
	})

	if normalized.URL == nil {
		t.Fatalf("expected URL to be set")
	}
	if normalized.URL.String() != "http://example.com/dists/stable/InRelease" {
		t.Fatalf("unexpected URL %q", normalized.URL.String())
	}
	if !normalized.RemoteLastModified.IsZero() {
		t.Fatalf("expected unknown RemoteLastModified to stay zero, got %v", normalized.RemoteLastModified)
	}
}

func TestRefreshFileStoresLastModifiedWithoutPreviousRemoteTime(t *testing.T) {
	const (
		responseBody = "new inrelease content"
		newETag      = "\"new-etag\""
	)
	const lastModifiedRaw = "Wed, 11 Feb 2026 14:09:49 GMT"

	expectedLastModified, err := time.Parse(http.TimeFormat, lastModifiedRaw)
	if err != nil {
		t.Fatalf("failed to parse test Last-Modified header: %v", err)
	}

	cache := newTestFSCache(t)
	cache.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/debian/dists/trixie-updates/InRelease" {
				t.Fatalf("unexpected request path: %s", r.URL.Path)
			}
			headers := http.Header{}
			headers.Set("Last-Modified", lastModifiedRaw)
			headers.Set("ETag", newETag)
			return &http.Response{
				StatusCode:    http.StatusOK,
				Header:        headers,
				Body:          io.NopCloser(strings.NewReader(responseBody)),
				ContentLength: int64(len(responseBody)),
				Request:       r,
			}, nil
		}),
	}

	localFile := mustParseURL(t, "http://mirror.example/debian/dists/trixie-updates/InRelease")
	generatedName := cache.buildLocalPath(localFile)

	if err := os.MkdirAll(filepath.Dir(generatedName), 0o755); err != nil {
		t.Fatalf("failed to create cache directory: %v", err)
	}
	if err := os.WriteFile(generatedName, []byte("old content"), 0o644); err != nil {
		t.Fatalf("failed to write old cache file: %v", err)
	}

	protocol := DetermineProtocolFromURL(localFile)
	previousEntry := AccessEntry{
		LastAccessed: time.Now().Add(-time.Hour),
		LastChecked:  time.Now().Add(-10 * time.Minute),
		ETag:         "\"old-etag\"",
		URL:          localFile,
		Size:         int64(len("old content")),
		SHA256:       "old-sha",
	}
	if err := cache.Set(protocol, localFile.Host, localFile.Path, previousEntry); err != nil {
		t.Fatalf("failed to seed access cache entry: %v", err)
	}

	refreshed, err := cache.refreshFile(generatedName, localFile, previousEntry)
	if err != nil {
		t.Fatalf("refreshFile returned error: %v", err)
	}
	if !refreshed {
		t.Fatalf("expected refreshFile to detect a changed file")
	}

	gotEntry, ok := cache.Get(protocol, localFile.Host, localFile.Path)
	if !ok {
		t.Fatalf("expected updated access entry in cache")
	}
	if gotEntry.RemoteLastModified.IsZero() {
		t.Fatalf("expected RemoteLastModified to be stored from response")
	}
	if !gotEntry.RemoteLastModified.Equal(expectedLastModified) {
		t.Fatalf("unexpected RemoteLastModified: got %v want %v", gotEntry.RemoteLastModified, expectedLastModified)
	}
	if gotEntry.LastChecked.IsZero() {
		t.Fatalf("expected LastChecked to be updated on successful refresh")
	}
	if gotEntry.ETag != newETag {
		t.Fatalf("unexpected ETag: got %q want %q", gotEntry.ETag, newETag)
	}
	if gotEntry.Size != int64(len(responseBody)) {
		t.Fatalf("unexpected size: got %d want %d", gotEntry.Size, len(responseBody))
	}

	data, err := os.ReadFile(generatedName)
	if err != nil {
		t.Fatalf("failed reading refreshed file: %v", err)
	}
	if string(data) != responseBody {
		t.Fatalf("unexpected refreshed file contents: got %q want %q", string(data), responseBody)
	}
}
