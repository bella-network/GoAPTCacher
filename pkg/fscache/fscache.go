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
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/asaskevich/govalidator"
)

// FSCache is a cache for files downloaded from the internet.
type FSCache struct {
	client    *http.Client
	CachePath string

	CustomCachePath func(r *url.URL) string

	accessCache *accessCache
}

// NewFSCache creates a new FSCache with the given cache path.
func NewFSCache(cachePath string) *FSCache {
	cache, err := newAccessCache(cachePath + "/access_cache.db")
	if err != nil {
		log.Fatalf("Error creating access cache: %s\n", err)
	}

	return &FSCache{
		client: &http.Client{
			Timeout: time.Hour, // Timeout every extreme long requests
			Transport: &http.Transport{
				Proxy:               nil,
				MaxIdleConnsPerHost: 5,
			},
		},
		CachePath:   cachePath,
		accessCache: cache,
	}
}

// GetDatabaseConnection returns the database connection of the FSCache.
func (c *FSCache) GetDatabaseConnection() *sql.DB {
	return c.accessCache.GetDatabaseConnection()
}

// buildLocalPath builds the local path for the given request.
func (c *FSCache) buildLocalPath(rq *url.URL) string {
	if c.CustomCachePath != nil {
		return c.CustomCachePath(rq)
	}

	return c.CachePath + "/" + rq.Host + rq.Path
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
	//case http.MethodHead:
	//c.serveHEADRequest(r, w, localFile)
	//case http.MethodConnect:
	// TODO: Implement CONNECT method
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		log.Printf("Method not allowed: %s\n", r.Method)
	}
}
