package fscache

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
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

const (
	accessCacheMetaSuffix           = ".access.json"
	accessCacheFlushIntervalDefault = 30 * time.Second
)

type accessEntryJSON struct {
	Protocol           int       `json:"protocol"`
	Domain             string    `json:"domain"`
	Path               string    `json:"path"`
	URL                string    `json:"url,omitempty"`
	LastAccessed       time.Time `json:"last_accessed,omitempty"`
	LastChecked        time.Time `json:"last_checked,omitempty"`
	RemoteLastModified time.Time `json:"remote_last_modified,omitempty"`
	ETag               string    `json:"etag,omitempty"`
	Size               int64     `json:"size,omitempty"`
	SHA256             string    `json:"sha256,omitempty"`
	MarkedForDeletion  bool      `json:"marked_for_deletion,omitempty"`
	MarkedAt           time.Time `json:"marked_at,omitempty"`
}

type accessCacheRecord struct {
	entry             AccessEntry
	protocol          int
	domain            string
	path              string
	markedForDeletion bool
	markedAt          time.Time
	dirty             bool
}

func (fs *FSCache) accessCacheKey(protocol int, domain, path string) string {
	return strconv.Itoa(protocol) + "|" + domain + "|" + path
}

func protocolScheme(protocol int) string {
	if protocol == 1 {
		return "https"
	}
	return "http"
}

func (fs *FSCache) buildAccessURL(protocol int, domain, path string) *url.URL {
	return &url.URL{
		Scheme: protocolScheme(protocol),
		Host:   domain,
		Path:   path,
	}
}

func (fs *FSCache) accessCacheMetaPath(protocol int, domain, path string) string {
	localPath := fs.buildLocalPath(fs.buildAccessURL(protocol, domain, path))
	return localPath + accessCacheMetaSuffix
}

func (fs *FSCache) normalizeAccessEntry(protocol int, domain, path string, entry AccessEntry) AccessEntry {
	if entry.URL == nil {
		entry.URL = fs.buildAccessURL(protocol, domain, path)
	}
	if entry.RemoteLastModified.IsZero() && !entry.LastAccessed.IsZero() {
		entry.RemoteLastModified = entry.LastAccessed
	}
	return entry
}

func (fs *FSCache) startAccessCacheFlushLoop() {
	interval := fs.accessCacheFlushInterval
	if interval <= 0 {
		interval = accessCacheFlushIntervalDefault
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fs.flushAccessCache()
			case <-fs.accessCacheStop:
				return
			}
		}
	}()
}

func (fs *FSCache) flushAccessCache() {
	keys := make([]string, 0)
	fs.accessCacheMux.RLock()
	for key, record := range fs.accessCache {
		if record.dirty {
			keys = append(keys, key)
		}
	}
	fs.accessCacheMux.RUnlock()

	for _, key := range keys {
		fs.accessCacheMux.Lock()
		record, ok := fs.accessCache[key]
		if !ok || !record.dirty {
			fs.accessCacheMux.Unlock()
			continue
		}
		recordCopy := *record
		record.dirty = false
		fs.accessCacheMux.Unlock()

		if err := fs.writeAccessCacheRecord(&recordCopy); err != nil {
			log.Printf("[WARN:ACCESS] failed to write access cache record: %v", err)
			fs.accessCacheMux.Lock()
			if current, ok := fs.accessCache[key]; ok && !current.dirty {
				current.dirty = true
			}
			fs.accessCacheMux.Unlock()
		}
	}
}

func (fs *FSCache) writeAccessCacheRecord(record *accessCacheRecord) error {
	metaPath := fs.accessCacheMetaPath(record.protocol, record.domain, record.path)
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		return err
	}

	urlString := ""
	if record.entry.URL != nil {
		urlString = record.entry.URL.String()
	}
	if urlString == "" {
		urlString = fs.GetURL(record.protocol, record.domain, record.path)
	}

	payload := accessEntryJSON{
		Protocol:           record.protocol,
		Domain:             record.domain,
		Path:               record.path,
		URL:                urlString,
		LastAccessed:       record.entry.LastAccessed,
		LastChecked:        record.entry.LastChecked,
		RemoteLastModified: record.entry.RemoteLastModified,
		ETag:               record.entry.ETag,
		Size:               record.entry.Size,
		SHA256:             record.entry.SHA256,
		MarkedForDeletion:  record.markedForDeletion,
		MarkedAt:           record.markedAt,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	tmpPath := metaPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, metaPath)
}

func (fs *FSCache) loadAccessCacheRecord(protocol int, domain, path string) (*accessCacheRecord, bool) {
	metaPath := fs.accessCacheMetaPath(protocol, domain, path)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, false
	}

	var payload accessEntryJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		log.Printf("[WARN:ACCESS] invalid metadata %s: %v", metaPath, err)
		return nil, false
	}

	entry := AccessEntry{
		LastAccessed:       payload.LastAccessed,
		LastChecked:        payload.LastChecked,
		RemoteLastModified: payload.RemoteLastModified,
		ETag:               payload.ETag,
		Size:               payload.Size,
		SHA256:             payload.SHA256,
	}

	if payload.URL != "" {
		if parsed, err := url.Parse(payload.URL); err == nil {
			entry.URL = parsed
		}
	}

	if entry.URL == nil {
		entry.URL = fs.buildAccessURL(protocol, domain, path)
	}

	return &accessCacheRecord{
		entry:             entry,
		protocol:          protocol,
		domain:            domain,
		path:              path,
		markedForDeletion: payload.MarkedForDeletion,
		markedAt:          payload.MarkedAt,
	}, true
}

func (fs *FSCache) loadAccessCacheRecordFromFile(metaPath string) (*accessCacheRecord, bool) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, false
	}

	var payload accessEntryJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		log.Printf("[WARN:ACCESS] invalid metadata %s: %v", metaPath, err)
		return nil, false
	}

	entry := AccessEntry{
		LastAccessed:       payload.LastAccessed,
		LastChecked:        payload.LastChecked,
		RemoteLastModified: payload.RemoteLastModified,
		ETag:               payload.ETag,
		Size:               payload.Size,
		SHA256:             payload.SHA256,
	}

	protocol := payload.Protocol
	domain := payload.Domain
	path := payload.Path

	if payload.URL != "" {
		if parsed, err := url.Parse(payload.URL); err == nil {
			entry.URL = parsed
			if parsed.Host != "" {
				domain = parsed.Host
			}
			if parsed.Path != "" {
				path = parsed.Path
			}
			if parsed.Scheme != "" {
				protocol = DetermineProtocol(parsed.Scheme)
			}
		}
	}

	if domain == "" || path == "" {
		rel, err := filepath.Rel(fs.CachePath, metaPath)
		if err == nil && strings.HasSuffix(rel, accessCacheMetaSuffix) {
			rel = strings.TrimSuffix(rel, accessCacheMetaSuffix)
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			if len(parts) > 0 && domain == "" {
				domain = parts[0]
			}
			if len(parts) == 2 && path == "" {
				path = "/" + filepath.ToSlash(parts[1])
			}
		}
	}

	if domain == "" || path == "" {
		return nil, false
	}

	if entry.URL == nil {
		entry.URL = fs.buildAccessURL(protocol, domain, path)
	}

	return &accessCacheRecord{
		entry:             entry,
		protocol:          protocol,
		domain:            domain,
		path:              path,
		markedForDeletion: payload.MarkedForDeletion,
		markedAt:          payload.MarkedAt,
	}, true
}

func (fs *FSCache) snapshotAccessCache() map[string]accessCacheRecord {
	fs.accessCacheMux.RLock()
	defer fs.accessCacheMux.RUnlock()

	snapshot := make(map[string]accessCacheRecord, len(fs.accessCache))
	for key, record := range fs.accessCache {
		snapshot[key] = accessCacheRecord{
			entry:             record.entry,
			protocol:          record.protocol,
			domain:            record.domain,
			path:              record.path,
			markedForDeletion: record.markedForDeletion,
			markedAt:          record.markedAt,
		}
	}

	return snapshot
}

func (fs *FSCache) loadAccessCacheRecordsFromDisk() (map[string]accessCacheRecord, error) {
	entries := map[string]accessCacheRecord{}
	if _, err := os.Stat(fs.CachePath); err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}

	err := filepath.WalkDir(fs.CachePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, accessCacheMetaSuffix) {
			return nil
		}

		record, ok := fs.loadAccessCacheRecordFromFile(path)
		if !ok {
			return nil
		}

		key := fs.accessCacheKey(record.protocol, record.domain, record.path)
		entries[key] = *record
		return nil
	})
	if err != nil {
		return nil, err
	}

	return entries, nil
}

func (fs *FSCache) collectAccessCacheRecords() ([]accessCacheRecord, error) {
	diskEntries, err := fs.loadAccessCacheRecordsFromDisk()
	if err != nil {
		return nil, err
	}

	memoryEntries := fs.snapshotAccessCache()
	for key, record := range memoryEntries {
		diskEntries[key] = record
	}

	result := make([]accessCacheRecord, 0, len(diskEntries))
	for _, record := range diskEntries {
		result = append(result, record)
	}

	return result, nil
}

func (fs *FSCache) getAccessCacheRecord(protocol int, domain, path string) (*accessCacheRecord, bool) {
	key := fs.accessCacheKey(protocol, domain, path)
	fs.accessCacheMux.RLock()
	record, ok := fs.accessCache[key]
	fs.accessCacheMux.RUnlock()
	if ok {
		return record, true
	}

	record, ok = fs.loadAccessCacheRecord(protocol, domain, path)
	if !ok {
		return nil, false
	}

	fs.accessCacheMux.Lock()
	if existing, ok := fs.accessCache[key]; ok {
		fs.accessCacheMux.Unlock()
		return existing, true
	}
	fs.accessCache[key] = record
	fs.accessCacheMux.Unlock()

	return record, true
}

func (fs *FSCache) setAccessCacheRecord(protocol int, domain, path string, update func(record *accessCacheRecord) bool) {
	key := fs.accessCacheKey(protocol, domain, path)
	fs.accessCacheMux.Lock()
	record, ok := fs.accessCache[key]
	if !ok {
		record = &accessCacheRecord{protocol: protocol, domain: domain, path: path}
		fs.accessCache[key] = record
	}
	if update(record) {
		record.dirty = true
	}
	fs.accessCacheMux.Unlock()
}

// Get returns the AccessEntry information for a given protocol, domain, and path.
func (fs *FSCache) Get(protocol int, domain, path string) (AccessEntry, bool) {
	record, ok := fs.getAccessCacheRecord(protocol, domain, path)
	if !ok {
		return AccessEntry{}, false
	}

	entry := fs.normalizeAccessEntry(protocol, domain, path, record.entry)
	return entry, true
}

// GetSHA256 returns the SHA256 hash for a given protocol, domain, and path of a
// file. This is used to retrieve the SHA256 hash of a file from the cache metadata.
func (fs *FSCache) GetSHA256(protocol int, domain, path string) (string, bool) {
	record, ok := fs.getAccessCacheRecord(protocol, domain, path)
	if !ok {
		return "", false
	}

	return record.entry.SHA256, true
}

// SetSHA256 sets the SHA256 hash for a given protocol, domain, and path of a
// file. This is used to store the SHA256 hash of a file in the cache metadata.
func (fs *FSCache) SetSHA256(protocol int, domain, path, sha256 string) error {
	fs.setAccessCacheRecord(protocol, domain, path, func(record *accessCacheRecord) bool {
		changed := record.entry.SHA256 != sha256
		record.entry.SHA256 = sha256
		if record.entry.URL == nil {
			record.entry.URL = fs.buildAccessURL(protocol, domain, path)
			changed = true
		}
		return changed
	})
	return nil
}

// Set sets the access information for a given key.
func (fs *FSCache) Set(protocol int, domain, path string, entry AccessEntry) error {
	if entry.URL == nil {
		entry.URL = fs.buildAccessURL(protocol, domain, path)
	}
	fs.setAccessCacheRecord(protocol, domain, path, func(record *accessCacheRecord) bool {
		record.entry = entry
		record.markedForDeletion = false
		record.markedAt = time.Time{}
		return true
	})
	return nil
}

// Delete deletes the access information for a given key.
func (fs *FSCache) Delete(protocol int, domain, path string) {
	key := fs.accessCacheKey(protocol, domain, path)
	fs.accessCacheMux.Lock()
	delete(fs.accessCache, key)
	fs.accessCacheMux.Unlock()

	metaPath := fs.accessCacheMetaPath(protocol, domain, path)
	_ = os.Remove(metaPath)
}

// Hit updates the access information for a given key. Most usually called if
// the ressource was accessed.
func (fs *FSCache) Hit(protocol int, domain, key string) error {
	record, ok := fs.getAccessCacheRecord(protocol, domain, key)
	if !ok {
		return nil
	}

	fs.accessCacheMux.Lock()
	record.entry.LastAccessed = time.Now()
	record.dirty = true
	fs.accessCacheMux.Unlock()

	return nil
}

// UpdateLastChecked updates the last checked time for the given key.
func (fs *FSCache) UpdateLastChecked(protocol int, domain, path string) error {
	record, ok := fs.getAccessCacheRecord(protocol, domain, path)
	if !ok {
		return nil
	}

	fs.accessCacheMux.Lock()
	record.entry.LastChecked = time.Now()
	record.dirty = true
	fs.accessCacheMux.Unlock()

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
func (fs *FSCache) UpdateFile(protocol int, domain, path, urlString string, lastModified time.Time, etag string, size int64) {
	parsedURL, err := url.Parse(urlString)
	if err != nil {
		parsedURL = fs.buildAccessURL(protocol, domain, path)
	}

	fs.setAccessCacheRecord(protocol, domain, path, func(record *accessCacheRecord) bool {
		record.entry.URL = parsedURL
		record.entry.RemoteLastModified = lastModified
		record.entry.ETag = etag
		record.entry.Size = size
		record.markedForDeletion = false
		record.markedAt = time.Time{}
		return true
	})
}

// AddURLIfNotExists adds the URL to the given key if the url isn't already
// stored with the entry.
func (fs *FSCache) AddURLIfNotExists(protocol int, domain, path, urlString string) error {
	parsedURL, err := url.Parse(urlString)
	if err != nil {
		parsedURL = fs.buildAccessURL(protocol, domain, path)
	}

	fs.setAccessCacheRecord(protocol, domain, path, func(record *accessCacheRecord) bool {
		if record.entry.URL == nil || record.entry.URL.String() != parsedURL.String() {
			record.entry.URL = parsedURL
			return true
		}
		return false
	})
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
	record, ok := fs.getAccessCacheRecord(protocol, domain, path)
	if !ok {
		return
	}

	fs.accessCacheMux.Lock()
	record.markedForDeletion = true
	record.markedAt = time.Now()
	record.dirty = true
	fs.accessCacheMux.Unlock()
}

// TrackRequest tracks a request in the database.
func (fs *FSCache) TrackRequest(cacheHit bool, transferred int64) error {
	if fs.db == nil {
		return nil
	}
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
