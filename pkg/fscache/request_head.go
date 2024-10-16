package fscache

import (
	"fmt"
	"net/http"
	"os"
)

// serveHEADRequest serves a HEAD request for a file from cache if available and not expired. If
// the file is not in the cache, it is downloaded from the internet.
func (c *FSCache) serveHEADRequest(r *http.Request, w http.ResponseWriter) {

	// Define the local path for the file
	localFile := fmt.Sprintf(
		"%s/%s/%s",
		c.CachePath,
		r.URL.Host,
		r.URL.Path,
	)

	// Check if the file exists in the cache
	if fi, err := os.Stat(localFile); err == nil {
		// Add header that describes the cache hit
		w.Header().Add("X-Cache", "HIT")
		w.Header().Add("Content-Length", fmt.Sprintf("%d", fi.Size()))
		w.Header().Add("Content-Type", "application/octet-stream")
		return
	}

	// If the file is not in the cache, download it
	err := c.downloadFileSimple(r.URL.String(), localFile)
	if err != nil {
		http.Error(w, "Error downloading file", http.StatusInternalServerError)
		return
	}

	// Serve the file from the cache
	fi, err := os.Stat(localFile)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	// Add header that describes the cache miss
	w.Header().Add("X-Cache", "MISS")
	w.Header().Add("Content-Length", fmt.Sprintf("%d", fi.Size()))
	w.Header().Add("Content-Type", "application/octet-stream")
}
