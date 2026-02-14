package fscache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrackAndSnapshotIncludesTunnelTraffic(t *testing.T) {
	cache := newTestFSCache(t)

	if err := cache.TrackRequest(true, 10); err != nil {
		t.Fatalf("TrackRequest(hit) error = %v", err)
	}
	if err := cache.TrackRequest(false, 20); err != nil {
		t.Fatalf("TrackRequest(miss) error = %v", err)
	}
	if err := cache.TrackTunnelRequest(30); err != nil {
		t.Fatalf("TrackTunnelRequest() error = %v", err)
	}

	snapshot := cache.GetStatsSnapshot(1)
	if snapshot.Totals.Requests != 3 {
		t.Fatalf("Requests = %d, want 3", snapshot.Totals.Requests)
	}
	if snapshot.Totals.Hits != 1 {
		t.Fatalf("Hits = %d, want 1", snapshot.Totals.Hits)
	}
	if snapshot.Totals.Misses != 1 {
		t.Fatalf("Misses = %d, want 1", snapshot.Totals.Misses)
	}
	if snapshot.Totals.Tunnel != 1 {
		t.Fatalf("Tunnel = %d, want 1", snapshot.Totals.Tunnel)
	}
	if snapshot.Totals.TrafficDown != 50 {
		t.Fatalf("TrafficDown = %d, want 50", snapshot.Totals.TrafficDown)
	}
	if snapshot.Totals.TrafficUp != 60 {
		t.Fatalf("TrafficUp = %d, want 60", snapshot.Totals.TrafficUp)
	}
	if snapshot.Totals.TunnelTransfer != 30 {
		t.Fatalf("TunnelTransfer = %d, want 30", snapshot.Totals.TunnelTransfer)
	}
	if len(snapshot.Daily) != 1 {
		t.Fatalf("Daily len = %d, want 1", len(snapshot.Daily))
	}
}

func TestFlushAndLoadStatsFromDisk(t *testing.T) {
	cache := newTestFSCache(t)
	if err := cache.TrackRequest(false, 12); err != nil {
		t.Fatalf("TrackRequest() error = %v", err)
	}

	if err := cache.flushStatsToDisk(); err != nil {
		t.Fatalf("flushStatsToDisk() error = %v", err)
	}
	if _, err := os.Stat(cache.statsFilePath()); err != nil {
		t.Fatalf("expected stats file to exist: %v", err)
	}

	loaded := &FSCache{CachePath: cache.CachePath, statsByDate: make(map[string]*statsEntry)}
	if err := loaded.loadStatsFromDisk(); err != nil {
		t.Fatalf("loadStatsFromDisk() error = %v", err)
	}

	snapshot := loaded.GetStatsSnapshot(10)
	if snapshot.Totals.Requests != 1 {
		t.Fatalf("loaded Requests = %d, want 1", snapshot.Totals.Requests)
	}
	if snapshot.Totals.Misses != 1 {
		t.Fatalf("loaded Misses = %d, want 1", snapshot.Totals.Misses)
	}
	if snapshot.Totals.TrafficDown != 12 {
		t.Fatalf("loaded TrafficDown = %d, want 12", snapshot.Totals.TrafficDown)
	}
	if snapshot.Totals.TrafficUp != 12 {
		t.Fatalf("loaded TrafficUp = %d, want 12", snapshot.Totals.TrafficUp)
	}
}

func TestFlushStatsToDiskWithoutChangesDoesNothing(t *testing.T) {
	cache := &FSCache{CachePath: t.TempDir(), statsByDate: make(map[string]*statsEntry)}

	if err := cache.flushStatsToDisk(); err != nil {
		t.Fatalf("flushStatsToDisk() error = %v", err)
	}
	if _, err := os.Stat(cache.statsFilePath()); !os.IsNotExist(err) {
		t.Fatalf("expected no stats file, stat error = %v", err)
	}
}

func TestGetCacheUsageDeduplicatesSameLocalFile(t *testing.T) {
	cache := newTestFSCache(t)
	httpsURL := mustParseURL(t, "https://example.com/pool/main/p/pkg.deb")
	httpURL := mustParseURL(t, "http://example.com/pool/main/p/pkg.deb")
	localPath := cache.buildLocalPath(httpsURL)

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if err := cache.Set(DetermineProtocolFromURL(httpsURL), httpsURL.Host, httpsURL.Path, AccessEntry{URL: httpsURL}); err != nil {
		t.Fatalf("Set(https) error = %v", err)
	}
	if err := cache.Set(DetermineProtocolFromURL(httpURL), httpURL.Host, httpURL.Path, AccessEntry{URL: httpURL}); err != nil {
		t.Fatalf("Set(http) error = %v", err)
	}

	files, size, err := cache.GetCacheUsage()
	if err != nil {
		t.Fatalf("GetCacheUsage() error = %v", err)
	}
	if files != 1 {
		t.Fatalf("files = %d, want 1", files)
	}
	if size != uint64(len("payload")) {
		t.Fatalf("size = %d, want %d", size, len("payload"))
	}
}
