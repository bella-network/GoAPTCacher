package fscache

import (
	"fmt"
	"net/http"
	"os"
)

// serveHEADRequest serves a HEAD request for a file from cache if available and not expired. If
// the file is not in the cache, it is downloaded from the internet.
func (c *FSCache) serveHEADRequest(r *http.Request, w http.ResponseWriter) {
	c.serveHEADRequestWithDeps(r, w, os.Stat, c.downloadFileSimple)
}

func (c *FSCache) serveHEADRequestWithDeps(
	r *http.Request,
	w http.ResponseWriter,
	statFile func(string) (os.FileInfo, error),
	downloadFile func(string, string) error,
) {
	if statFile == nil {
		statFile = os.Stat
	}
	if downloadFile == nil {
		downloadFile = c.downloadFileSimple
	}

	// Define the local path for the file
	localFile := c.buildLocalPath(r.URL)

	// Check if the file exists in the cache
	if fi, err := statFile(localFile); err == nil {
		// Add header that describes the cache hit
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
		return
	}

	// If the file is not in the cache, download it
	err := downloadFile(r.URL.String(), localFile)
	if err != nil {
		http.Error(w, "Error downloading file", http.StatusInternalServerError)
		return
	}

	// Serve the file from the cache
	fi, err := statFile(localFile)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	// Add header that describes the cache miss
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
}
