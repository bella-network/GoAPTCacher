// FSCache is a package cache for files downloaded from the internet. It is able
// to check if the file was modified since the last download and only download
// it if necessary. It also caches the files on disk to avoid downloading them
// again. The cache is thread-safe and can be used by multiple goroutines
// concurrently.
// Additionally, the cacher can be used as streaming proxy to stream file
// contents to the client while downloading the file from the original source.
// At the same time it caches the file on disk.
package fscache

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/asaskevich/govalidator"
)

// FSCache is a cache for files downloaded from the internet.
type FSCache struct {
	client    *http.Client
	CachePath string

	CustomCachePath func(r *url.URL) string

	expirationInDays uint64

	memoryFileReadLockMux  sync.RWMutex
	memoryFileReadLock     map[string]time.Time
	memoryFileWriteLockMux sync.RWMutex
	memoryFileWriteLock    map[string]time.Time

	accessCacheMux           sync.RWMutex
	accessCache              map[string]*accessCacheRecord
	accessCacheFlushInterval time.Duration
	accessCacheStop          chan struct{}

	statsMux           sync.RWMutex
	statsByDate        map[string]*statsEntry
	statsFlushInterval time.Duration
	statsStop          chan struct{}
	statsDirty         bool
	statsRevision      uint64
}

// NewFSCache creates a new FSCache with the given cache path.
func NewFSCache(cachePath string) *FSCache {
	cache := &FSCache{
		client: &http.Client{
			Timeout: time.Hour, // Timeout every extreme long requests
			Transport: &http.Transport{
				Proxy:                 nil,             // No proxy by default
				MaxIdleConnsPerHost:   7,               // Maximum number of idle connections per host
				ResponseHeaderTimeout: time.Minute * 5, // Timeout for response headers
			},
		},
		CachePath:           cachePath,
		memoryFileReadLock:  make(map[string]time.Time),
		memoryFileWriteLock: make(map[string]time.Time),
		accessCache:         make(map[string]*accessCacheRecord),
		accessCacheStop:     make(chan struct{}),
		statsByDate:         make(map[string]*statsEntry),
		statsStop:           make(chan struct{}),
	}

	cache.accessCacheFlushInterval = accessCacheFlushIntervalDefault
	cache.startAccessCacheFlushLoop()
	cache.statsFlushInterval = statsFlushIntervalDefault
	if err := cache.loadStatsFromDisk(); err != nil {
		log.Printf("[WARN:STATS] failed to load persisted stats: %v", err)
	}
	cache.startStatsFlushLoop()

	return cache
}

// SetExpirationDays sets the expiration days for the cache, this will also
// start the expiration ticker in the background.
func (c *FSCache) SetExpirationDays(days uint64) {
	firstSet := c.expirationInDays == 0

	c.expirationInDays = days

	if firstSet {
		log.Printf("[INFO:EXPIRE] Activated file expiration\n")
		go c.expireUnusedFiles()
	}
}

// buildLocalPath builds the local path for the given request.
func (c *FSCache) buildLocalPath(rq *url.URL) string {
	if c.CustomCachePath != nil {
		return c.CustomCachePath(rq)
	}

	base := filepath.Clean(c.CachePath)

	host := rq.Hostname()
	if host == "" {
		host = rq.Host
	}
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.Trim(host, ".")
	if host == "" {
		host = "_invalid_host"
	}
	host = strings.ReplaceAll(host, "/", "_")
	host = strings.ReplaceAll(host, "\\", "_")

	normalizedPath := strings.ReplaceAll(rq.Path, "\\", "/")
	cleanPath := path.Clean("/" + normalizedPath)
	cleanPath = strings.TrimPrefix(cleanPath, "/")

	return filepath.Join(base, host, filepath.FromSlash(cleanPath))
}

// validateRequest validates the given request and returns an error if the
// request is invalid.
func (c *FSCache) validateRequest(r *http.Request) error {
	// Check if the used HTTP host is valid
	if r.URL.Host == "" {
		if r.Host == "" {
			return fmt.Errorf("invalid host")
		} else {
			r.URL.Host = r.Host
		}
	}

	// Check if the used HTTP Host is a valid domain
	if !govalidator.IsDNSName(r.URL.Host) {
		return fmt.Errorf("invalid host")
	}

	return nil
}

// ServeFromRequest serves a file from cache if available and not expired. If
// the file is not in the cache, it is downloaded from the internet.
func (c *FSCache) ServeFromRequest(r *http.Request, w http.ResponseWriter) {
	// Check if the request is valid
	if err := c.validateRequest(r); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		log.Printf("Invalid request: %s\n", err)
		return
	}

	switch r.Method {
	case http.MethodGet:
		c.serveGETRequest(r, w)
	case http.MethodHead:
		c.serveHEADRequest(r, w)
	//case http.MethodConnect:
	// TODO: Implement CONNECT method
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		log.Printf("Method not allowed: %s\n", r.Method)
	}
}

// DetermineProtocol determines the protocol based on the given string.
func DetermineProtocol(protocol string) int {
	// Normalize the protocol to lowercase to handle case insensitivity
	protocol = strings.ToLower(protocol)

	// Determine the protocol based on the scheme
	switch protocol {
	case "https":
		return 1 // HTTPS
	default: // HTTP or any other protocol
		return 0 // HTTP
	}
}

// DetermineProtocolFromURL determines the protocol from the given URL.
func DetermineProtocolFromURL(r *url.URL) int {
	return DetermineProtocol(r.Scheme)
}
