package cdb

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t testing.TB) (*sql.DB, func()) {
	dbPath := filepath.Join(os.TempDir(), "testdb_queues.cdb")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	err = initiateStructure(db)
	if err != nil {
		t.Fatalf("Failed to initiate structure: %v", err)
	}

	return db, func() {
		db.Close()
		os.Remove(dbPath)
	}
}

func TestTrackRequestCacheHit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := TrackRequest(db, true, 1024)
	if err != nil {
		t.Fatalf("Failed to track request: %v", err)
	}

	date := time.Now().Format("2006-01-02")
	row := db.QueryRow("SELECT requests, hits, misses, traffic_down, traffic_up FROM stats WHERE date = ?", date)

	var requests, hits, misses, trafficDown, trafficUp int64
	err = row.Scan(&requests, &hits, &misses, &trafficDown, &trafficUp)
	if err != nil {
		t.Fatalf("Failed to scan stats: %v", err)
	}

	if requests != 1 || hits != 1 || misses != 0 || trafficDown != 0 || trafficUp != 1024 {
		t.Fatalf("Unexpected stats values: requests=%d, hits=%d, misses=%d, traffic_down=%d, traffic_up=%d",
			requests, hits, misses, trafficDown, trafficUp)
	}
}

func TestTrackRequestCacheMiss(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := TrackRequest(db, false, 2048)
	if err != nil {
		t.Fatalf("Failed to track request: %v", err)
	}

	date := time.Now().Format("2006-01-02")
	row := db.QueryRow("SELECT requests, hits, misses, traffic_down, traffic_up FROM stats WHERE date = ?", date)

	var requests, hits, misses, trafficDown, trafficUp int64
	err = row.Scan(&requests, &hits, &misses, &trafficDown, &trafficUp)
	if err != nil {
		t.Fatalf("Failed to scan stats: %v", err)
	}

	if requests != 1 || hits != 0 || misses != 1 || trafficDown != 2048 || trafficUp != 2048 {
		t.Fatalf("Unexpected stats values: requests=%d, hits=%d, misses=%d, traffic_down=%d, traffic_up=%d",
			requests, hits, misses, trafficDown, trafficUp)
	}
}

func BenchmarkTrackRequestCacheHit(b *testing.B) {
	db, cleanup := setupTestDB(b)
	defer cleanup()

	for i := 0; i < b.N; i++ {
		err := TrackRequest(db, true, 1024)
		if err != nil {
			b.Fatalf("Failed to track request: %v", err)
		}
	}
}

func BenchmarkTrackRequestCacheMiss(b *testing.B) {
	db, cleanup := setupTestDB(b)
	defer cleanup()

	for i := 0; i < b.N; i++ {
		err := TrackRequest(db, false, 2048)
		if err != nil {
			b.Fatalf("Failed to track request: %v", err)
		}
	}
}
