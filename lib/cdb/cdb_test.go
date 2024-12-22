package cdb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "testdb_open.cdb")
	defer os.Remove(dbPath)

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// write something to the database to check if it was created
	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("Database file was not created")
	}
}

func TestOpenEmbed(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "testdb_embed.cdb")
	defer os.Remove(dbPath)

	db, err := OpenEmbed(dbPath)
	if err != nil {
		t.Fatalf("Failed to open embedded database: %v", err)
	}
	defer db.Close()

	// write something to the database to check if it was created
	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("Embedded database file was not created")
	}
}

func TestOpenMemory(t *testing.T) {
	db, err := OpenMemory()
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	// write something to the database to check if it was created
	_, err = db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// No file to check for in-memory database
}

func TestPrepareAndMigrate(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "testdb_prepare.cdb")
	defer os.Remove(dbPath)

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	created, err := PrepareAndMigrate(db)
	if err != nil {
		t.Fatalf("Failed to prepare and migrate database: %v", err)
	}

	if !created {
		t.Fatalf("Expected database to be created and migrated")
	}

	// Check if tables exist
	tables := []string{"keyvalue", "access_cache", "file_lock", "write_lock", "marked_files", "stats"}
	for _, table := range tables {
		rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
		if err != nil {
			t.Fatalf("Failed to query table %s: %v", table, err)
		}
		if !rows.Next() {
			t.Fatalf("Expected table %s to exist", table)
		}
		rows.Close()
	}
}

func TestInitiateStructure(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "testdb_initiate.cdb")
	defer os.Remove(dbPath)

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	err = initiateStructure(db)
	if err != nil {
		t.Fatalf("Failed to initiate structure: %v", err)
	}

	// Check if tables exist
	tables := []string{"keyvalue", "access_cache", "file_lock", "write_lock", "marked_files", "stats"}
	for _, table := range tables {
		rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
		if err != nil {
			t.Fatalf("Failed to query table %s: %v", table, err)
		}
		if !rows.Next() {
			t.Fatalf("Expected table %s to exist", table)
		}
		rows.Close()
	}
}

func TestMigrateDatabase(t *testing.T) {
	dbPath := filepath.Join(os.TempDir(), "testdb_migrate.cdb")
	defer os.Remove(dbPath)

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	err = initiateStructure(db)
	if err != nil {
		t.Fatalf("Failed to initiate structure: %v", err)
	}

	err = migrateDatabase(db)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// Check if schema version is set
	rows, err := db.Query("SELECT value FROM keyvalue WHERE key = 'schema_version'")
	if err != nil {
		t.Fatalf("Failed to query schema version: %v", err)
	}
	defer rows.Close()

	var version string
	if rows.Next() {
		err = rows.Scan(&version)
		if err != nil {
			t.Fatalf("Failed to scan schema version: %v", err)
		}
	}

	if version != "1" {
		t.Fatalf("Expected schema version 1, got %s", version)
	}
}
