package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCachedRepositoryFromInReleasePath(t *testing.T) {
	path := filepath.Join("/var/cache/goaptcacher", "mirror.example.org", "ubuntu", "dists", "noble", "InRelease")

	got, ok := cachedRepositoryFromInReleasePath(path)
	if !ok {
		t.Fatalf("cachedRepositoryFromInReleasePath(%q) reported ok=false", path)
	}

	want := cachedRepository{
		rootPath: filepath.Join("/var/cache/goaptcacher", "mirror.example.org", "ubuntu"),
		distrib:  "noble",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cachedRepositoryFromInReleasePath(%q) = %#v, want %#v", path, got, want)
	}
}

func TestDiscoverCachedRepositories(t *testing.T) {
	cacheDir := t.TempDir()

	valid := filepath.Join(cacheDir, "mirror.example.org", "debian", "dists", "stable", "InRelease")
	if err := os.MkdirAll(filepath.Dir(valid), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned error: %v", filepath.Dir(valid), err)
	}
	if err := os.WriteFile(valid, []byte("SHA256:\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", valid, err)
	}

	ignored := filepath.Join(cacheDir, "mirror.example.org", "debian", "dists", "stable", "Release")
	if err := os.WriteFile(ignored, []byte("not used"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", ignored, err)
	}

	repositories, err := discoverCachedRepositories(cacheDir)
	if err != nil {
		t.Fatalf("discoverCachedRepositories() returned error: %v", err)
	}

	want := []cachedRepository{
		{
			rootPath: filepath.Join(cacheDir, "mirror.example.org", "debian"),
			distrib:  "stable",
		},
	}
	if !reflect.DeepEqual(repositories, want) {
		t.Fatalf("discoverCachedRepositories() = %#v, want %#v", repositories, want)
	}
}
