package fscache

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

var RefreshFiles = []string{
	"InRelease",
	"Release",
	"Release.gpg",
}

var ConnectedFiles = map[string][]string{
	"InRelease":   {"Release", "Release.gpg"},
	"Release":     {"Release.gpg", "InRelease"},
	"Release.gpg": {"Release", "InRelease"},
}

func (c *FSCache) evaluateRefresh(localFile *url.URL, lastAccess AccessEntry) bool {
	// From localFile, get the filename only without the path
	filename := filepath.Base(c.buildLocalPath(localFile))

	recheckTimeout := time.Hour * 24

	// Check if the file is in the list of files that should be refreshed more often
	for _, file := range RefreshFiles {
		if file == filename {
			recheckTimeout = time.Minute * 5
			break
		}
	}

	// Check if the file is older than the recheck timeout
	return time.Since(lastAccess.LastChecked) > recheckTimeout
}

func (c *FSCache) cacheRefesh(localFile *url.URL, lastAccess AccessEntry) {
	generatedName := c.buildLocalPath(localFile)
	// From localFile, get the filename only without the path
	filename := filepath.Base(generatedName)
	// get dir where the file is stored
	dir := filepath.Dir(generatedName)

	// Get the connected files
	connectedFiles, ok := ConnectedFiles[filename]
	if !ok {
		connectedFiles = []string{}
	} else {
		// As the current file is a connected file, the connected files are in a relative path
		for i, file := range connectedFiles {
			connectedFiles[i] = filepath.Join(dir, file)

			// A connected file could include ".." in the path, so we need to resolve the path
			connectedFiles[i], _ = filepath.Abs(connectedFiles[i])
		}
	}

	// Perform a GET request to the server to check if the file has changed. If
	// possible, use the ETag or Last-Modified header. If the file has changed,
	// download the new file. A unchanged file should be signaled by the server
	// using a 304 Not Modified HTTP response.
	req, err := http.NewRequest("GET", lastAccess.URL, nil)
	if err != nil {
		return
	}

	// Add header to identify the client
	req.Header.Add("User-Agent", "GoAptCacher")

	// If ETag is available, add it to the request
	if lastAccess.ETag != "" {
		req.Header.Add("If-None-Match", lastAccess.ETag)
	}

	// If Last-Modified is available, add it to the request
	if !lastAccess.RemoteLastModified.IsZero() {
		req.Header.Add("If-Modified-Since", lastAccess.RemoteLastModified.Format(http.TimeFormat))
	}

	// Perform the request
	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// First check if the HTTP status code is 304 Not Modified, if so update
	// only the last checked time.
	if resp.StatusCode == http.StatusNotModified {
		// Update the last checked time
		c.accessCache.UpdateLastChecked(localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:304] %s%s has not changed\n", localFile.Host, localFile.Path)
		return
	}

	// If status code is 404 Not Found, mark the file for deletion.
	if resp.StatusCode == http.StatusNotFound {
		// Mark the file for deletion
		c.accessCache.MarkForDeletion(localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:404] %s%s not found, marked for deletion\n", localFile.Host, localFile.Path)
		return
	}

	// If the status code is not 200 OK, log warning and return
	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARN:REFRESH:CODE] %s%s returned status code %d\n", localFile.Host, localFile.Path, resp.StatusCode)
		return
	}

	// Check if the file has changed. For this, we compare the Last-Modified
	// header and the ETag header with the last known values. If Last-Modified
	// is older than the locally stored file, we assume the file has not changed.
	lastModified, err := time.Parse(http.TimeFormat, resp.Header.Get("Last-Modified"))
	if err != nil {
		lastModified = time.Time{}
	}
	if lastModified.Before(lastAccess.RemoteLastModified) {
		// Update the last checked time
		c.accessCache.UpdateLastChecked(localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:NOTMODIFIED:LAST-MODIFIED] %s%s has not changed\n", localFile.Host, localFile.Path)
		return
	}

	// Check if the ETag has changed
	etag := resp.Header.Get("ETag")
	if etag != "" && etag == lastAccess.ETag {
		// Update the last checked time
		c.accessCache.UpdateLastChecked(localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:NOTMODIFIED:ETAG] %s%s has not changed\n", localFile.Host, localFile.Path)
		return
	}

	// At this point, we know that the file has changed. We need to download the
	// new file and update the cache.

	// Create the file
	file, err := os.Create(generatedName + "-dl")
	if err != nil {
		log.Printf("[ERROR:REFRESH:CREATE] %s\n", err)
		return
	}

	// Write the file
	wrb, err := io.Copy(file, resp.Body)
	if err != nil {
		log.Printf("[ERROR:REFRESH:WRITE] %s\n", err)
		file.Close()

		return
	}

	err = file.Close()
	if err != nil {
		log.Printf("[ERROR:REFRESH:CLOSE] %s\n", err)
		return
	}

	// Rename the file and overwrite the old file
	err = os.Rename(generatedName+"-dl", generatedName)
	if err != nil {
		log.Printf("[ERROR:REFRESH:RENAME] %s\n", err)
		return
	}

	// Update the access cache with the new file
	c.accessCache.UpdateFile(localFile.Host, localFile.Path, lastAccess.URL, lastModified, etag)

	log.Printf("[INFO:REFRESH:200] %s%s has changed, downloaded %d bytes\n", localFile.Host, localFile.Path, wrb)
}
