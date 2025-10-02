package fscache

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/google/uuid"
)

var RefreshFiles = []string{
	"InRelease",
	"Release",
	"Release.gpg",
	"Packages",
	"Packages.gz",
}

var ConnectedFiles = map[string][]string{
	"InRelease": {
		"Release",
		"Release.gpg",
		"main/binary-amd64/Packages",
		"main/binary-amd64/Packages.gz",
		"main/binary-amd64/Packages.bz2",
		"main/binary-amd64/Packages.xz",
		"main/binary-i386/Packages",
		"main/binary-i386/Packages.gz",
		"main/binary-i386/Packages.bz2",
		"main/binary-i386/Packages.xz",
		"main/binary-arm64/Packages",
		"main/binary-arm64/Packages.gz",
		"main/binary-arm64/Packages.bz2",
		"main/binary-arm64/Packages.xz",
		"main/binary-armhf/Packages",
		"main/binary-armhf/Packages.gz",
		"main/binary-armhf/Packages.bz2",
		"main/binary-armhf/Packages.xz",
		"main/binary-all/Packages",
		"main/binary-all/Packages.gz",
		"main/binary-all/Packages.bz2",
		"main/binary-all/Packages.xz",
	},
	"Release":     {"Release.gpg", "InRelease"},
	"Release.gpg": {"Release", "InRelease"},
}

// evaluateRefresh checks if the file should be refreshed.
func (c *FSCache) evaluateRefresh(localFile *url.URL, lastAccess AccessEntry) bool {
	// From localFile, get the filename only without the path
	filename := filepath.Base(c.buildLocalPath(localFile))

	recheckTimeout := time.Hour * 24

	// Check if the file is in the list of files that should be refreshed more often
	if slices.Contains(RefreshFiles, filename) {
		recheckTimeout = time.Minute * 5
	}

	// Check if the file is older than the recheck timeout
	return time.Since(lastAccess.LastChecked) > recheckTimeout
}

// cacheRefresh refreshes the file if it has changed. If the file has changed, it
func (c *FSCache) cacheRefresh(localFile *url.URL, lastAccess AccessEntry) {
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
			connectedFile, ok := c.GetFileByPath(lastAccess.URL.String(), file)
			if !ok {
				continue
			}

			var protocol int
			if connectedFile.Scheme == "https" {
				protocol = 1 // HTTPS
			} else {
				protocol = 0 // HTTP
			}

			// Get the last access of the connected file
			connectedLastAccess, ok := c.Get(protocol, connectedFile.Host, connectedFile.Path)
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

// refreshFile checks if the file has changed and downloads the new file if
// necessary. The function returns true if the file has changed and false if the
// file has not changed. An error is returned if an error occurred during the
// download.
func (c *FSCache) refreshFile(generatedName string, localFile *url.URL, lastAccess AccessEntry) (bool, error) {
	// Perform a GET request to the server to check if the file has changed. If
	// possible, use the ETag or Last-Modified header. If the file has changed,
	// download the new file. A unchanged file should be signaled by the server
	// using a 304 Not Modified HTTP response.
	req, err := http.NewRequest("GET", lastAccess.URL.String(), nil)
	if err != nil {
		return false, err
	}

	// Add header to identify the client
	req.Header.Set("User-Agent", "GoAptCacher/1.0 (+https://gitlab.com/bella.network/goaptcacher)")
	req.Header.Set("X-ACTION", "refresh")

	// If ETag is available, add it to the request
	if lastAccess.ETag != "" {
		req.Header.Set("If-None-Match", lastAccess.ETag)
	}

	// If Last-Modified is available, add it to the request
	if !lastAccess.RemoteLastModified.IsZero() {
		req.Header.Set("If-Modified-Since", lastAccess.RemoteLastModified.UTC().Format(http.TimeFormat))
	}

	// This is specific to the GoAptCacher implementation, we add the SHA256 hash as header to the request
	if lastAccess.SHA256 != "" {
		req.Header.Set("X-SHA256", lastAccess.SHA256)
	}

	// Perform the request
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// Determine protocol from the URL scheme
	protocol := DetermineProtocolFromURL(lastAccess.URL)

	// First check if the HTTP status code is 304 Not Modified, if so update
	// only the last checked time.
	if resp.StatusCode == http.StatusNotModified {
		// Update the last checked time
		c.UpdateLastChecked(protocol, localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:304] %s%s has not changed\n", localFile.Host, localFile.Path)
		return false, nil
	}

	// If status code is 404 Not Found, mark the file for deletion.
	if resp.StatusCode == http.StatusNotFound {
		// Mark the file for deletion
		c.MarkForDeletion(protocol, localFile.Host, localFile.Path)
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
	// is older than the locally stored file, we assume the file has not
	// changed. Only perform this check if the Last-Modified header is
	// available.
	var lastModified time.Time
	if lastmod := resp.Header.Get("Last-Modified"); lastmod != "" && !lastAccess.RemoteLastModified.IsZero() {
		lastModified, err := time.Parse(http.TimeFormat, lastmod)
		if err == nil && lastModified.Before(lastAccess.RemoteLastModified) {
			// Update the last checked time
			c.UpdateLastChecked(protocol, localFile.Host, localFile.Path)
			log.Printf("[INFO:REFRESH:NOTMODIFIED:LAST-MODIFIED] %s%s has not changed\n", localFile.Host, localFile.Path)
			return false, nil
		}
	}

	// Check if the ETag has changed
	etag := resp.Header.Get("ETag")
	if etag != "" && etag == lastAccess.ETag {
		// Update the last checked time
		c.UpdateLastChecked(protocol, localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:NOTMODIFIED:ETAG] %s%s has not changed\n", localFile.Host, localFile.Path)
		return false, nil
	}

	// At this point, we know that the file has changed. We need to download the
	// new file and update the cache.
	requiredSize := resp.ContentLength
	if requiredSize > 0 {
		if err := ensureDiskSpace(generatedName, requiredSize); err != nil {
			log.Printf("[ERROR:REFRESH:DISK] %s %s\n", generatedName, err)
			return false, err
		}
	}

	// Generate a temporary filename to download the file to. This is
	// necessary to avoid overwriting the old file while it is still being
	// downloaded and to ensure that the file is only replaced once the download is
	// complete.
	tmpID, err := uuid.NewRandom()
	if err != nil {
		log.Printf("[ERROR:REFRESH:RANDOM] %s\n", err)
		return false, err
	}

	tempPath := generatedName + "-dl-" + tmpID.String()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	// Create the file
	file, err := os.Create(tempPath)
	if err != nil {
		log.Printf("[ERROR:REFRESH:CREATE] %s\n", err)
		return false, err
	}

	if err := preallocateFile(file, requiredSize); err != nil {
		log.Printf("[ERROR:REFRESH:PREALLOCATE] %s\n", err)
		file.Close()
		return false, err
	}

	// Write the file
	wrb, err := io.Copy(file, resp.Body)
	if err != nil {
		log.Printf("[ERROR:REFRESH:WRITE] %s\n", err)
		file.Close()

		return false, err
	}

	if err := file.Close(); err != nil {
		log.Printf("[ERROR:REFRESH:CLOSE] %s\n", err)
		return false, err
	}

	if resp.ContentLength > 0 && resp.ContentLength != wrb {
		err := fmt.Errorf("downloaded size mismatch: expected %d bytes, got %d", resp.ContentLength, wrb)
		log.Printf("[ERROR:REFRESH:LENGTH] %s\n", err)
		return false, err
	}

	newHash, err := GenerateSHA256Hash(tempPath)
	if err != nil {
		log.Printf("[ERROR:REFRESH:HASH] %s\n", err)
		return false, err
	}

	// Rename the file and overwrite the old file
	if err := os.Rename(tempPath, generatedName); err != nil {
		log.Printf("[ERROR:REFRESH:RENAME] %s\n", err)
		return false, err
	}
	cleanupTemp = false

	// Update the access cache with the new file
	c.UpdateFile(protocol, localFile.Host, localFile.Path, lastAccess.URL.String(), lastModified, etag, wrb)
	if err := c.SetSHA256(protocol, localFile.Host, localFile.Path, newHash); err != nil {
		log.Printf("[ERROR:REFRESH:SHA256] %s\n", err)
	}
	go c.TrackRequest(false, wrb)

	log.Printf("[INFO:REFRESH:200] %s%s has changed, downloaded %d bytes\n", localFile.Host, localFile.Path, wrb)

	return true, nil
}
