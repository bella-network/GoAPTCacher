package fscache

import (
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gitlab.com/bella.network/goaptcacher/lib/dbc"
)

// AccessEntry is an entry in the accessCache.
type AccessEntry struct {
	LastAccessed       time.Time `json:"last_accessed,omitempty"`
	LastChecked        time.Time `json:"last_checked,omitempty"`
	RemoteLastModified time.Time `json:"remote_last_modified,omitempty"`
	ETag               string    `json:"etag,omitempty"`
	URL                *url.URL  `json:"url,omitempty"`
	Size               int64     `json:"size,omitempty"`
	SHA256             string    `json:"sha256,omitempty"`
}

// Get returns the AccessEntry information for a given protocol, domain, and path.
func (fs *FSCache) Get(protocol int, domain, path string) (AccessEntry, bool) {
	row := fs.db.QueryRow(
		"SELECT ac.last_access, ac.last_check, f.modified, f.etag, f.size, f.sha256 FROM access_cache ac JOIN files f ON ac.file = f.id JOIN domains d ON f.domain = d.id WHERE d.protocol = ? AND d.domain = ? AND f.path = ?",
		protocol,
		domain,
		path,
	)

	var entry AccessEntry
	var lastAccessed, lastChecked, remoteLastModified string
	err := row.Scan(&lastAccessed, &lastChecked, &remoteLastModified, &entry.ETag, &entry.Size, &entry.SHA256)
	if err != nil {
		return AccessEntry{}, false
	}

	entry.LastAccessed, _ = time.Parse(time.RFC3339, lastAccessed)
	entry.LastChecked, _ = time.Parse(time.RFC3339, lastChecked)
	entry.RemoteLastModified, _ = time.Parse(time.RFC3339, remoteLastModified)
	if entry.RemoteLastModified.IsZero() {
		entry.RemoteLastModified = entry.LastAccessed
	}

	entry.URL, err = url.Parse(fs.GetURL(protocol, domain, path))
	if err != nil {
		return AccessEntry{}, false
	}

	return entry, true
}

// GetSHA256 returns the SHA256 hash for a given protocol, domain, and path of a
// file. This is used to retrieve the SHA256 hash of a file from the database.
func (fs *FSCache) GetSHA256(protocol int, domain, path string) (string, bool) {
	row := fs.db.QueryRow(
		"SELECT f.sha256 FROM files f JOIN domains d ON f.domain = d.id WHERE d.protocol = ? AND d.domain = ? AND f.path = ?",
		protocol,
		domain,
		path,
	)

	var sha256 string
	err := row.Scan(&sha256)
	if err != nil {
		return "", false
	}

	return sha256, true
}

// SetSHA256 sets the SHA256 hash for a given protocol, domain, and path of a
// file. This is used to store the SHA256 hash of a file in the database.
func (fs *FSCache) SetSHA256(protocol int, domain, path, sha256 string) error {
	_, err := fs.db.Exec(
		"UPDATE files SET sha256 = ? WHERE domain = (SELECT id FROM domains WHERE protocol = ? AND domain = ?) AND path = ?",
		sha256,
		protocol,
		domain,
		path,
	)

	return err
}

// Set sets the access information for a given key.
func (fs *FSCache) Set(protocol int, domain, path string, entry AccessEntry) error {
	// Select the Domain ID for the given protocol and domain
	var domainID int64
	if err := fs.db.QueryRow("SELECT `id` FROM `domains` WHERE `protocol` = ? AND `domain` = ?", protocol, domain).Scan(&domainID); err != nil {
		// Insert domain with protocol into the domains table as the previous
		// query failed, most likely because the domain does not exist yet.
		ins, err := fs.db.Exec("INSERT IGNORE INTO `domains` (`protocol`, `domain`) VALUES (?, ?)", protocol, domain)
		if err != nil {
			return err
		}
		// Get the last inserted ID, which is the domain ID
		domainID, err = ins.LastInsertId()
		if err != nil {
			return err
		}
	}

	// Insert the file if it does not exist
	if _, err := fs.db.Exec("INSERT INTO `files` (`domain`, `path`, `url`, `size`, `etag`, `modified`, `sha256`) VALUES (?, ?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE `url` = VALUES(`url`), `size` = VALUES(`size`), `etag` = VALUES(`etag`), `modified` = VALUES(`modified`), `sha256` = VALUES(`sha256`)",
		domainID,
		path,
		entry.URL.String(),
		entry.Size,
		entry.ETag,
		entry.RemoteLastModified.Format("2006-01-02 15:04:05"),
		entry.SHA256,
	); err != nil {
		return err
	}

	// Get ID of the file just inserted or updated
	var fileID int64
	if err := fs.db.QueryRow("SELECT `id` FROM `files` WHERE `domain` = ? AND `path` = ?", domainID, path).Scan(&fileID); err != nil {
		return err
	}

	// Insert or update the access cache entry
	if _, err := fs.db.Exec("INSERT INTO access_cache (file, last_access, last_check) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE last_access = VALUES(last_access), last_check = VALUES(last_check)",
		fileID,
		entry.LastAccessed.Format("2006-01-02 15:04:05"),
		entry.LastChecked.Format("2006-01-02 15:04:05"),
	); err != nil {
		return err
	}

	return nil
}

// Delete deletes the access information for a given key.
func (fs *FSCache) Delete(protocol int, domain, path string) {
	_, _ = fs.db.Exec("DELETE FROM access_cache WHERE file = (SELECT id FROM files WHERE domain = (SELECT id FROM domains WHERE protocol = ? AND domain = ?) AND path = ?)", protocol, domain, path)
}

// Hit updates the access information for a given key. Most usually called if
// the ressource was accessed.
func (fs *FSCache) Hit(protocol int, domain, key string) error {
	_, err := fs.db.Exec("UPDATE access_cache SET last_access = ? WHERE file = (SELECT id FROM files WHERE domain = (SELECT id FROM domains WHERE protocol = ? AND domain = ?) AND path = ?)", time.Now(), protocol, domain, key)
	if err != nil {
		return err
	}

	return nil
}

// UpdateLastChecked updates the last checked time for the given key.
func (fs *FSCache) UpdateLastChecked(protocol int, domain, path string) error {
	_, err := fs.db.Exec("UPDATE access_cache SET last_check = ? WHERE file = (SELECT id FROM files WHERE domain = (SELECT id FROM domains WHERE protocol = ? AND domain = ?) AND path = ?)", time.Now(), protocol, domain, path)
	if err != nil {
		return err
	}

	return nil
}

// GetURL returns the URL of the given key.
func (fs *FSCache) GetURL(protocol int, domain, path string) string {
	// Build the URL from the protocol, domain, and path
	var fileURL *url.URL
	if protocol == 1 { // HTTPS
		fileURL = &url.URL{
			Scheme: "https",
			Host:   domain,
			Path:   path,
		}
	} else { // Assuming everything else is HTTP
		fileURL = &url.URL{
			Scheme: "http",
			Host:   domain,
			Path:   path,
		}
	}

	return fileURL.String()
}

// UpdateFile updates the file for the given key.
func (fs *FSCache) UpdateFile(protocol int, domain, path, url string, lastModified time.Time, etag string, size int64) {
	_, _ = fs.db.Exec(
		"UPDATE access_cache SET last_access = ?, last_check = ?, last_modified = ? WHERE file = (SELECT id FROM files WHERE domain = (SELECT id FROM domains WHERE protocol = ? AND domain = ?) AND path = ?)",
		time.Now(),
		time.Now(),
		lastModified,
		protocol,
		domain,
		path,
	)
}

// AddURLIfNotExists adds the URL to the given key if the url isn't already
// stored with the entry.
func (fs *FSCache) AddURLIfNotExists(protocol int, domain, path, url string) error {
	// Add domain with protocol to the domains table if it does not exist
	_, _ = fs.db.Exec("INSERT IGNORE INTO `domains` (`protocol`, `domain`) VALUES (?, ?)", protocol, domain)

	// Select the Domain ID for the given protocol and domain
	var domainID int
	if err := fs.db.QueryRow("SELECT `id` FROM `domains` WHERE `protocol` = ? AND `domain` = ?", protocol, domain).Scan(&domainID); err != nil {
		return err
	}

	// Insert the file if it does not exist
	_, _ = fs.db.Exec("INSERT INTO `files` (`domain`, `path`, `url`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `url` = VALUES(`url`)", domainID, path, url)

	return nil
}

// GenerateUUID generates a new UUID.
func (fs *FSCache) GenerateUUID() string {
	id := uuid.New()
	return id.String()
}

// CreateFileLock creates a lock for the given domain and path.
func (fs *FSCache) CreateFileLock(protocol int, domain, path string) {
	fs.memoryFileReadLockWrite(protocol, domain, path)
}

// RemoveFileLock deletes the lock for the given UUID.
func (fs *FSCache) RemoveFileLock(protocol int, domain, path string) {
	fs.memoryFileReadLockDelete(protocol, domain, path)
}

// HasFileLock checks if the given domain and path has a lock.
func (fs *FSCache) HasFileLock(protocol int, domain, path string) (bool, time.Time) {
	return fs.memoryFileReadLockRead(protocol, domain, path)
}

// CreateWriteLock creates a in-memory write lock for the given protocol,
// domain, and path. This is used to prevent concurrent write access to the same
// file.
func (fs *FSCache) CreateWriteLock(protocol int, domain, path string) error {
	// Check if the write lock already exists
	if ok, _ := fs.HasWriteLock(protocol, domain, path); ok {
		return fmt.Errorf("write lock already exists")
	}

	fs.memoryFileWriteLockMux.Lock()
	defer fs.memoryFileWriteLockMux.Unlock()

	fs.memoryFileWriteLock[strconv.Itoa(protocol)+domain+path] = time.Now()
	return nil
}

// DeleteWriteLock deletes the write lock for the given protocol, domain and
// path. This is used to remove the write lock after the file has been
// successfully written to the disk.
func (fs *FSCache) DeleteWriteLock(protocol int, domain, path string) {
	// Remove the write lock from the in-memory map
	fs.memoryFileWriteLockMux.Lock()
	defer fs.memoryFileWriteLockMux.Unlock()

	delete(fs.memoryFileWriteLock, strconv.Itoa(protocol)+domain+path)
}

// HasWriteLock checks if the given protocol, domain and path has a write lock.
func (fs *FSCache) HasWriteLock(protocol int, domain, path string) (bool, time.Time) {
	fs.memoryFileWriteLockMux.RLock()
	defer fs.memoryFileWriteLockMux.RUnlock()

	lockTime, ok := fs.memoryFileWriteLock[strconv.Itoa(protocol)+domain+path]
	if !ok {
		return false, time.Time{}
	}

	return true, lockTime
}

// CreateExclusiveWriteLock locks the write lock for the given domain if it is
// not already locked for writing and there are currently no read locks.
func (fs *FSCache) CreateExclusiveWriteLock(protocol int, domain, path string) bool {
	if ok, _ := fs.HasWriteLock(protocol, domain, path); ok {
		return false
	}

	if ok, _ := fs.HasFileLock(protocol, domain, path); ok {
		return false
	}

	err := fs.CreateWriteLock(protocol, domain, path)

	return err == nil
}

// MarkForDeletion marks the given domain and path for deletion.
func (fs *FSCache) MarkForDeletion(protocol int, domain, path string) {
	_, err := fs.db.Exec("INSERT INTO files_delete (file, time) VALUES ((SELECT id FROM files WHERE domain = (SELECT id FROM domains WHERE protocol = ? AND domain = ?) AND path = ?), NOW())", protocol, domain, path)
	if err != nil {
		log.Printf("[ERROR] Failed to mark file for deletion: %v", err)
	}
}

// TrackRequest tracks a request in the database.
func (fs *FSCache) TrackRequest(cacheHit bool, transferred int64) error {
	return dbc.TrackRequest(fs.db, cacheHit, transferred)
}

// GetFileByPath returns the file by the given path. OriginURL is the URL of the
// file that was previously accessed as path might be a relative path.
func (fs *FSCache) GetFileByPath(originURL, path string) (*url.URL, bool) {
	// Try to parse originURL
	origin, err := url.Parse(originURL)
	if err != nil {
		origin = &url.URL{}
	}

	// Try to resolve path based on origin. If path is an absolute URL, it will
	// be returned as is. This is done by checking if the path starts with a
	// protocol like http:// or https://.
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		// Parse the URL
		url, err := url.Parse(path)
		if err != nil {
			return nil, false
		}

		return url, true
	}

	// Resolve the path based on the origin URL
	return origin.ResolveReference(&url.URL{Path: path}), true
}

// memoryFileReadLockRead reads the memoryFileReadLock if a given file is locked.
func (fs *FSCache) memoryFileReadLockRead(protocol int, domain, path string) (bool, time.Time) {
	fs.memoryFileReadLockMux.RLock()
	defer fs.memoryFileReadLockMux.RUnlock()

	lockTime, ok := fs.memoryFileReadLock[strconv.Itoa(protocol)+domain+path]
	return ok, lockTime
}

// memoryFileReadLockWrite writes the memoryFileReadLock if a given file is locked.
func (fs *FSCache) memoryFileReadLockWrite(protocol int, domain, path string) {
	fs.memoryFileReadLockMux.Lock()
	defer fs.memoryFileReadLockMux.Unlock()

	fs.memoryFileReadLock[strconv.Itoa(protocol)+domain+path] = time.Now()
}

// memoryFileReadLockDelete deletes the memoryFileReadLock if a given file is locked.
func (fs *FSCache) memoryFileReadLockDelete(protocol int, domain, path string) {
	fs.memoryFileReadLockMux.Lock()
	defer fs.memoryFileReadLockMux.Unlock()

	delete(fs.memoryFileReadLock, strconv.Itoa(protocol)+domain+path)
}
