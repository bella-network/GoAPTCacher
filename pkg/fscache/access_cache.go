package fscache

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"gitlab.com/bella.network/goaptcacher/lib/cdb"
)

// AccessEntry is an entry in the accessCache.
type AccessEntry struct {
	LastAccessed       time.Time `json:"last_accessed,omitempty"`
	LastChecked        time.Time `json:"last_checked,omitempty"`
	RemoteLastModified time.Time `json:"remote_last_modified,omitempty"`
	ETag               string    `json:"etag,omitempty"`
	URL                string    `json:"url,omitempty"`
	Size               int64     `json:"size,omitempty"`
}

// accessCache is a cache for file access information.
type accessCache struct {
	db *sql.DB
}

// newAccessCache creates a new accessCache.
func newAccessCache(file string) (*accessCache, error) {
	conn, err := cdb.OpenFile(file)
	if err != nil {
		return nil, err
	}

	// Check if the tables exist
	_, err = cdb.PrepareAndMigrate(conn)
	if err != nil {
		return nil, err
	}

	return &accessCache{
		db: conn,
	}, nil
}

// GetDatabaseConnection returns the database connection of the accessCache.
func (ac *accessCache) GetDatabaseConnection() *sql.DB {
	return ac.db
}

// Set sets the access information for a given domain and path.
func (ac *accessCache) Get(domain, path string) (AccessEntry, bool) {
	row := ac.db.QueryRow(
		"SELECT last_accessed, last_checked, remote_last_modified, etag, size, url FROM access_cache WHERE domain = ? AND path = ?",
		domain,
		path,
	)

	var entry AccessEntry
	err := row.Scan(&entry.LastAccessed, &entry.LastChecked, &entry.RemoteLastModified, &entry.ETag, &entry.Size, &entry.URL)
	if err != nil {
		return AccessEntry{}, false
	}

	return entry, true
}

// Set sets the access information for a given key.
func (ac *accessCache) Set(domain, path string, entry AccessEntry) error {
	_, err := ac.db.Exec(
		"INSERT INTO access_cache (domain, path, last_accessed, last_checked, remote_last_modified, etag, size, url) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		domain,
		path,
		entry.LastAccessed,
		entry.LastChecked,
		entry.RemoteLastModified,
		entry.ETag,
		entry.Size,
		entry.URL,
	)

	return err
}

// Delete deletes the access information for a given key.
func (ac *accessCache) Delete(domain, path string) {
	_, _ = ac.db.Exec("DELETE FROM access_cache WHERE domain = ? AND path = ?", domain, path)
}

// Hit updates the access information for a given key. Most usually called if
// the ressource was accessed.
func (ac *accessCache) Hit(domain, key string) error {
	_, err := ac.db.Exec("UPDATE access_cache SET last_accessed = ? WHERE domain = ? AND path = ?", time.Now(), domain, key)
	if err != nil {
		return err
	}

	return nil
}

// UpdateLastChecked updates the last checked time for the given key.
func (ac *accessCache) UpdateLastChecked(domain, path string) error {
	_, err := ac.db.Exec("UPDATE access_cache SET last_checked = ? WHERE domain = ? AND path = ?", time.Now(), domain, path)
	if err != nil {
		return err
	}

	return nil
}

// GetURL returns the URL of the given key.
func (ac *accessCache) GetURL(domain, path string) string {
	row := ac.db.QueryRow("SELECT url FROM access_cache WHERE domain = ? AND path = ?", domain, path)

	var url string
	_ = row.Scan(&url)

	return url
}

// UpdateFile updates the file for the given key.
func (ac *accessCache) UpdateFile(domain, path, url string, lastModified time.Time, etag string, size int64) {
	_, _ = ac.db.Exec(
		"UPDATE access_cache SET last_accessed = ?, last_checked = ?, remote_last_modified = ?, etag = ?, size = ?, url = ? WHERE domain = ? AND path = ?",
		time.Now(),
		time.Now(),
		lastModified,
		etag,
		size,
		url,
		domain,
		path,
	)
}

// AddURLIfNotExists adds the URL to the given key if the url isn't already
// stored with the entry.
func (ac *accessCache) AddURLIfNotExists(domain, path, url string) {
	_, _ = ac.db.Exec("INSERT OR IGNORE INTO access_cache (domain, path, url) VALUES (?, ?, ?)", domain, path, url)
}

// GenerateUUID generates a new UUID.
func (ac *accessCache) GenerateUUID() string {
	id := uuid.New()
	return id.String()
}

// CreateFileLock creates a lock for the given domain and path.
func (ac *accessCache) CreateFileLock(domain, path string) (string, error) {
	uuid := ac.GenerateUUID()

	_, err := ac.db.Exec("INSERT INTO file_lock (domain, path, uuid, lock_time) VALUES (?, ?, ?, ?)", domain, path, uuid, time.Now().Unix())
	if err != nil {
		return "", err
	}

	return uuid, nil
}

// RemoveFileLock deletes the lock for the given UUID.
func (ac *accessCache) RemoveFileLock(uuid string) {
	_, _ = ac.db.Exec("DELETE FROM file_lock WHERE uuid = ?", uuid)
}

// HasFileLock checks if the given domain and path has a lock.
func (ac *accessCache) HasFileLock(domain, path string) bool {
	row := ac.db.QueryRow("SELECT uuid FROM file_lock WHERE domain = ? AND path = ?", domain, path)

	var uuid string
	err := row.Scan(&uuid)

	return err == nil
}

// CreateWriteLock creates a write lock for the given domain and path.
func (ac *accessCache) CreateWriteLock(domain, path string) error {
	_, err := ac.db.Exec("INSERT INTO write_lock (domain, path, lock_time) VALUES (?, ?, ?)", domain, path, time.Now().Unix())
	if err != nil {
		return err
	}

	return nil
}

// DeleteWriteLock deletes the write lock for the given domain and path.
func (ac *accessCache) DeleteWriteLock(domain, path string) {
	_, _ = ac.db.Exec("DELETE FROM write_lock WHERE domain = ? AND path = ?", domain, path)
}

// RemoveWriteLockByUUID deletes the write lock for the given UUID.
func (ac *accessCache) RemoveWriteLockByUUID(uuid string) {
	_, _ = ac.db.Exec("DELETE FROM write_lock WHERE uuid = ?", uuid)
}

// HasWriteLock checks if the given domain and path has a write lock.
func (ac *accessCache) HasWriteLock(domain, path string) bool {
	row := ac.db.QueryRow("SELECT lock_time FROM write_lock WHERE domain = ? AND path = ?", domain, path)

	var lockTime int64
	err := row.Scan(&lockTime)

	return err == nil
}

// CreateExclusiveWriteLock locks the write lock for the given domain if it is
// not already locked for writing and there are currently no read locks.
func (ac *accessCache) CreateExclusiveWriteLock(domain, path string) bool {
	if ac.HasWriteLock(domain, path) {
		return false
	}

	if ac.HasFileLock(domain, path) {
		return false
	}

	err := ac.CreateWriteLock(domain, path)

	return err == nil
}

// MarkForDeletion marks the given domain and path for deletion.
func (ac *accessCache) MarkForDeletion(domain, path string) {
	_, _ = ac.db.Exec("INSERT INTO marked_files (domain, path, mark_time) VALUES (?, ?, ?)", domain, path, time.Now().Unix())
}

// TrackRequest tracks a request in the database.
func (ac *accessCache) TrackRequest(cacheHit bool, transferred int64) error {
	return cdb.TrackRequest(ac.db, cacheHit, transferred)
}
