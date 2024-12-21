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
	"Packages",
	"Packages.gz",
}

var ConnectedFiles = map[string][]string{
	"InRelease":   {"Release", "Release.gpg", "main/binary-amd64/Packages", "main/binary-amd64/Packages.gz", "main/binary-i386/Packages", "main/binary-i386/Packages.gz", "main/binary-arm64/Packages", "main/binary-arm64/Packages.gz", "main/binary-armhf/Packages", "main/binary-armhf/Packages.gz", "main/binary-all/Packages", "main/binary-all/Packages.gz"},
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

	// Get the connected files
	connectedFiles, ok := ConnectedFiles[filename]
	if !ok {
		connectedFiles = []string{}
	}

	// Refresh the current file
	refreshed, err := c.refreshFile(generatedName, localFile, lastAccess)
	if err != nil {
		log.Printf("[ERROR:REFRESH] %s\n", err)
		return
	}

	// If the file was refreshed, we need to refresh the connected files
	if refreshed {
		// Parse protocol and domain from
		for _, file := range connectedFiles {
			// Get the URL of the connected file
			connectedFile, ok := c.accessCache.GetFileByPath(lastAccess.URL, file)
			if !ok {
				continue
			}

			// Get the last access of the connected file
			connectedLastAccess, ok := c.accessCache.Get(connectedFile.Host, connectedFile.Path)
			if !ok {
				// If the file is not in the cache or the last access is not
				// available, we can't refresh the file as it might not exist in
				// the target repository.
				continue
			}

			// Refresh the connected file
			_, err := c.refreshFile(file, connectedFile, connectedLastAccess)
			if err != nil {
				log.Printf("[ERROR:REFRESH] %s\n", err)
			}
		}
	}
}

func (c *FSCache) refreshFile(generatedName string, localFile *url.URL, lastAccess AccessEntry) (bool, error) {
	// Perform a GET request to the server to check if the file has changed. If
	// possible, use the ETag or Last-Modified header. If the file has changed,
	// download the new file. A unchanged file should be signaled by the server
	// using a 304 Not Modified HTTP response.
	req, err := http.NewRequest("GET", lastAccess.URL, nil)
	if err != nil {
		return false, err
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
		return false, err
	}
	defer resp.Body.Close()

	// First check if the HTTP status code is 304 Not Modified, if so update
	// only the last checked time.
	if resp.StatusCode == http.StatusNotModified {
		// Update the last checked time
		c.accessCache.UpdateLastChecked(localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:304] %s%s has not changed\n", localFile.Host, localFile.Path)
		return false, nil
	}

	// If status code is 404 Not Found, mark the file for deletion.
	if resp.StatusCode == http.StatusNotFound {
		// Mark the file for deletion
		c.accessCache.MarkForDeletion(localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:404] %s%s not found, marked for deletion\n", localFile.Host, localFile.Path)
		return false, nil
	}

	// If the status code is not 200 OK, log warning and return
	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARN:REFRESH:CODE] %s%s returned status code %d\n", localFile.Host, localFile.Path, resp.StatusCode)
		return false, nil
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
		return false, nil
	}

	// Check if the ETag has changed
	etag := resp.Header.Get("ETag")
	if etag != "" && etag == lastAccess.ETag {
		// Update the last checked time
		c.accessCache.UpdateLastChecked(localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:NOTMODIFIED:ETAG] %s%s has not changed\n", localFile.Host, localFile.Path)
		return false, nil
	}

	// At this point, we know that the file has changed. We need to download the
	// new file and update the cache.

	// Create the file
	file, err := os.Create(generatedName + "-dl")
	if err != nil {
		log.Printf("[ERROR:REFRESH:CREATE] %s\n", err)
		return false, err
	}

	// Write the file
	wrb, err := io.Copy(file, resp.Body)
	if err != nil {
		log.Printf("[ERROR:REFRESH:WRITE] %s\n", err)
		file.Close()

		return false, err
	}

	err = file.Close()
	if err != nil {
		log.Printf("[ERROR:REFRESH:CLOSE] %s\n", err)
		return false, err
	}

	// Rename the file and overwrite the old file
	err = os.Rename(generatedName+"-dl", generatedName)
	if err != nil {
		log.Printf("[ERROR:REFRESH:RENAME] %s\n", err)
		return false, err
	}

	// Update the access cache with the new file
	c.accessCache.UpdateFile(localFile.Host, localFile.Path, lastAccess.URL, lastModified, etag, wrb)

	log.Printf("[INFO:REFRESH:200] %s%s has changed, downloaded %d bytes\n", localFile.Host, localFile.Path, wrb)

	return true, nil
}
