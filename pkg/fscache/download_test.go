package fscache

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type failingReadCloser struct{}

func (f failingReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (f failingReadCloser) Close() error {
	return nil
}

func TestDownloadFileSimpleSuccess(t *testing.T) {
	cache := newTestFSCache(t)
	expectedTime := time.Date(2026, 2, 11, 14, 9, 49, 0, time.UTC)

	cache.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.Header.Get("User-Agent"), "GoAptCacher/") {
			t.Fatalf("unexpected user agent: %q", r.Header.Get("User-Agent"))
		}

		headers := http.Header{}
		headers.Set("Last-Modified", expectedTime.Format(http.TimeFormat))
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        headers,
			Body:          io.NopCloser(strings.NewReader("payload")),
			ContentLength: 7,
			Request:       r,
		}, nil
	})}

	localPath := filepath.Join(t.TempDir(), "downloaded.bin")
	if err := cache.downloadFileSimple("https://example.com/pkg.deb", localPath); err != nil {
		t.Fatalf("downloadFileSimple() error = %v", err)
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if got := string(data); got != "payload" {
		t.Fatalf("file content = %q, want %q", got, "payload")
	}

	info, err := os.Stat(localPath)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.ModTime().UTC().Equal(expectedTime) {
		t.Fatalf("mod time = %v, want %v", info.ModTime().UTC(), expectedTime)
	}
}

func TestDownloadFileSimpleInvalidURL(t *testing.T) {
	cache := newTestFSCache(t)
	localPath := filepath.Join(t.TempDir(), "downloaded.bin")

	if err := cache.downloadFileSimple("://bad-url", localPath); err == nil {
		t.Fatalf("downloadFileSimple() error = nil, want non-nil")
	}
}

func TestDownloadFileSimpleClientError(t *testing.T) {
	cache := newTestFSCache(t)
	cache.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}

	localPath := filepath.Join(t.TempDir(), "downloaded.bin")
	if err := cache.downloadFileSimple("https://example.com/pkg.deb", localPath); err == nil {
		t.Fatalf("downloadFileSimple() error = nil, want non-nil")
	}
}

func TestDownloadFileSimpleCreateFileError(t *testing.T) {
	cache := newTestFSCache(t)
	cache.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{},
			Body:          io.NopCloser(strings.NewReader("payload")),
			ContentLength: 7,
			Request:       r,
		}, nil
	})}

	localPath := filepath.Join(t.TempDir(), "existing-dir")
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	if err := cache.downloadFileSimple("https://example.com/pkg.deb", localPath); err == nil {
		t.Fatalf("downloadFileSimple() error = nil, want non-nil")
	}
}

func TestDownloadFileSimpleCopyErrorRemovesPartialFile(t *testing.T) {
	cache := newTestFSCache(t)
	cache.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{},
			Body:          failingReadCloser{},
			ContentLength: -1,
			Request:       r,
		}, nil
	})}

	localPath := filepath.Join(t.TempDir(), "downloaded.bin")
	if err := cache.downloadFileSimple("https://example.com/pkg.deb", localPath); err == nil {
		t.Fatalf("downloadFileSimple() error = nil, want non-nil")
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected partial file to be removed, stat err = %v", err)
	}
}

func TestDownloadFileSimpleContentLengthMismatchRemovesPartialFile(t *testing.T) {
	cache := newTestFSCache(t)
	cache.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{},
			Body:          io.NopCloser(strings.NewReader("abc")),
			ContentLength: 5,
			Request:       r,
		}, nil
	})}

	localPath := filepath.Join(t.TempDir(), "downloaded.bin")
	err := cache.downloadFileSimple("https://example.com/pkg.deb", localPath)
	if err == nil {
		t.Fatalf("downloadFileSimple() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "download incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, stat err = %v", err)
	}
}
