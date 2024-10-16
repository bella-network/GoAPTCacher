package cdb

import (
	"database/sql"
	"log"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// Open opens a connection to a SQLite database.
func Open(name string) (*sql.DB, error) {
	return sql.Open("sqlite3", name)
}

// OpenEmbed opens a connection to an embedded SQLite database.
func OpenEmbed(name string) (*sql.DB, error) {
	return sql.Open("sqlite3", "embed:"+name)
}

// OpenMemory opens a connection to an in-memory SQLite database.
func OpenMemory() (*sql.DB, error) {
	return sql.Open("sqlite3", ":memory:")
}

// OpenTemp opens a connection to a temporary SQLite database.
func OpenFile(name string) (*sql.DB, error) {
	return sql.Open("sqlite3", "file:"+name)
}

// Close closes the database connection.
func Close(db *sql.DB) error {
	return db.Close()
}

func PrepareAndMigrate(db *sql.DB) (bool, error) {
	// Check if tables are already created. This is done by checking if the
	// table "keyvalue" exists.
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='keyvalue'")
	if err != nil {
		return false, err
	}

	// If table does not exist, create it using initiateStructure function.
	if !rows.Next() {
		err = initiateStructure(db)
		if err != nil {
			return false, err
		}
	}

	rows.Close()

	// Tables already exist, start database migration.
	err = migrateDatabase(db)
	if err != nil {
		return false, err
	}

	return true, nil
}

func initiateStructure(db *sql.DB) error {
	// Create keyvalue table to store key-value pairs.
	if _, err := db.Exec(`CREATE TABLE keyvalue (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`); err != nil {
		return err
	}
	// Make key column unique.
	if _, err := db.Exec(`CREATE UNIQUE INDEX keyvalue_key ON keyvalue (key)`); err != nil {
		return err
	}

	// AccessCache table is used to keep an index when a file was downloaded, accessed, etc.
	if _, err := db.Exec(`CREATE TABLE access_cache (
		domain TEXT NOT NULL,
		path TEXT NOT NULL,
		last_accessed TEXT NOT NULL,
		last_checked TEXT NOT NULL,
		remote_last_modified TEXT NOT NULL,
		etag TEXT,
		url TEXT NOT NULL,
		PRIMARY KEY (domain, path),
		UNIQUE (url)
	)`); err != nil {
		return err
	}

	// FileLock table is used to keep an index when a file is being downloaded.
	if _, err := db.Exec(`CREATE TABLE file_lock (
		domain TEXT NOT NULL,
		path TEXT NOT NULL,
		uuid TEXT NOT NULL,
		lock_time INTEGER NOT NULL,
		UNIQUE (uuid)
	)`); err != nil {
		return err
	}
	// Create index for domain and path.
	if _, err := db.Exec(`CREATE INDEX file_lock_domain_path ON file_lock (domain, path)`); err != nil {
		return err
	}

	// WriteLock table is used to keep an index when a file is being written.
	// Files currently being written are locked ans will not be made available
	// for download from the cache.
	if _, err := db.Exec(`CREATE TABLE write_lock (
		domain TEXT NOT NULL,
		path TEXT NOT NULL,
		lock_time INTEGER NOT NULL,
		PRIMARY KEY (domain, path)
	)`); err != nil {
		return err
	}

	// MarkedFiles table is used to keep an index of files that are marked for
	// deletion. These files will be deleted when the cache is cleaned.
	if _, err := db.Exec(`CREATE TABLE marked_files (
		domain TEXT NOT NULL,
		path TEXT NOT NULL,
		mark_time INTEGER NOT NULL,
		PRIMARY KEY (domain, path)
	)`); err != nil {
		return err
	}

	// Insert current schema version into the database.
	if _, err := db.Exec(`INSERT INTO keyvalue (key, value) VALUES ('schema_version', '1')`); err != nil {
		return err
	}

	return nil
}

func migrateDatabase(db *sql.DB) error {
	// Get current schema version from the database.
	rows, err := db.Query("SELECT value FROM keyvalue WHERE key = 'schema_version'")
	if err != nil {
		return err
	}
	defer rows.Close()

	var version int
	if rows.Next() {
		err = rows.Scan(&version)
		if err != nil {
			return err
		}
	}

	log.Printf("[DB:MIGRATE] Current schema version: %d\n", version)

	// Migrate database to the latest schema version.
	// NOTING TO DO AS NO NEW SCHEMA VERSIONS ARE AVAILABLE.

	return nil
}
