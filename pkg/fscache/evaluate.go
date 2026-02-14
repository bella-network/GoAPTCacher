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
	"strings"
	"time"

	"github.com/google/uuid"
	"gitlab.com/bella.network/goaptcacher/pkg/buildinfo"
)

// RefreshFiles is a list of files that should be refreshed more often as these
// contain the main indexes of a repository and are changed every time packages
// are added or removed.
var RefreshFiles = []string{
	"InRelease",
	"Release",
	"Release.gpg",
	"Packages",
	"Packages.gz",
	"Packages.bz2",
	"Packages.xz",
	"Sources",
	"Sources.gz",
	"Index",
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

	// By default a 24 hour recheck timeout is used
	recheckTimeout := time.Hour * 24

	// If the file is within pool/**, these files are usually static and do not
	// need to be refreshed often.
	if strings.Contains(localFile.Path, "/pool/") {
		recheckTimeout = time.Hour * 168 // 7 days
	}

	// Files served using the "by-hash" URL usually do not change and can be
	// cached for longer periods, same as pool files.
	if strings.Contains(localFile.Path, "/by-hash/") {
		recheckTimeout = time.Hour * 168 // 7 days
	}

	// Check if the file is in the RefreshFiles list which should be kept as fresh
	// as possible.
	if slices.Contains(RefreshFiles, filename) {
		recheckTimeout = time.Minute * 5
	}

	// Check if the file is older than the recheck timeout
	return time.Since(lastAccess.LastChecked) > recheckTimeout
}

// cacheRefresh refreshes the file if it has changed. If the file has changed, it
// will be downloaded again.
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
	// Build a conditional GET so unchanged files can be detected cheaply by the origin.
	req, err := buildRefreshRequest(lastAccess)
	if err != nil {
		return false, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// Use cached URL protocol to address the same entry that triggered this refresh.
	protocol := DetermineProtocolFromURL(lastAccess.URL)

	if c.handleRefreshStatus(resp.StatusCode, protocol, localFile) {
		return false, nil
	}

	lastModified, etag, unchanged := c.evaluateNotModified(resp, localFile, protocol, lastAccess)
	if unchanged {
		return false, nil
	}

	// Download into a temporary file and replace atomically once complete.
	wrb, newHash, err := downloadResponseToFile(resp, generatedName)
	if err != nil {
		return false, err
	}

	// Update the access cache with the new file
	c.UpdateFile(protocol, localFile.Host, localFile.Path, lastAccess.URL.String(), lastModified, etag, wrb)
	if err := c.SetSHA256(protocol, localFile.Host, localFile.Path, newHash); err != nil {
		log.Printf("[ERROR:REFRESH:SHA256] %s\n", err)
	}
	c.trackRequestAsync(false, wrb)

	log.Printf("[INFO:REFRESH:200] %s%s has changed, downloaded %d bytes\n", localFile.Host, localFile.Path, wrb)

	return true, nil
}

// buildRefreshRequest creates the conditional GET request used for cache refreshes.
func buildRefreshRequest(lastAccess AccessEntry) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, lastAccess.URL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", fmt.Sprintf("GoAptCacher/%s (+https://gitlab.com/bella.network/goaptcacher)", buildinfo.Version))
	req.Header.Set("X-ACTION", "refresh")

	if lastAccess.ETag != "" {
		req.Header.Set("If-None-Match", lastAccess.ETag)
	}
	if !lastAccess.RemoteLastModified.IsZero() {
		req.Header.Set("If-Modified-Since", lastAccess.RemoteLastModified.UTC().Format(http.TimeFormat))
	}
	// X-SHA256 is a GoAptCacher specific validator used by some origins.
	if lastAccess.SHA256 != "" {
		req.Header.Set("X-SHA256", lastAccess.SHA256)
	}

	return req, nil
}

// handleRefreshStatus handles status codes that do not require a download.
func (c *FSCache) handleRefreshStatus(statusCode, protocol int, localFile *url.URL) bool {
	switch statusCode {
	case http.StatusOK:
		return false
	case http.StatusNotModified:
		if err := c.UpdateLastChecked(protocol, localFile.Host, localFile.Path); err != nil {
			log.Printf("[ERROR:REFRESH:304] %s%s failed to update last checked: %v\n", localFile.Host, localFile.Path, err)
		}
		log.Printf("[INFO:REFRESH:304] %s%s has not changed\n", localFile.Host, localFile.Path)
	case http.StatusNotFound:
		c.MarkForDeletion(protocol, localFile.Host, localFile.Path)
		log.Printf("[INFO:REFRESH:404] %s%s not found, marked for deletion\n", localFile.Host, localFile.Path)
	default:
		log.Printf("[WARN:REFRESH:CODE] %s%s returned status code %d\n", localFile.Host, localFile.Path, statusCode)
	}

	return true
}

// evaluateNotModified evaluates validators and returns true if content is unchanged.
func (c *FSCache) evaluateNotModified(resp *http.Response, localFile *url.URL, protocol int, lastAccess AccessEntry) (time.Time, string, bool) {
	lastModified, unchangedByDate := c.resolveRemoteLastModified(resp.Header.Get("Last-Modified"), localFile, protocol, lastAccess)
	if unchangedByDate {
		return lastModified, "", true
	}

	etag := resp.Header.Get("ETag")
	if c.isUnchangedByETag(etag, protocol, localFile, lastAccess.ETag) {
		return lastModified, etag, true
	}

	return lastModified, etag, false
}

// resolveRemoteLastModified parses Last-Modified and compares it with known metadata.
func (c *FSCache) resolveRemoteLastModified(lastmod string, localFile *url.URL, protocol int, lastAccess AccessEntry) (time.Time, bool) {
	lastModified := lastAccess.RemoteLastModified
	if lastmod == "" {
		return lastModified, false
	}

	parsedLastModified, parseErr := time.Parse(http.TimeFormat, lastmod)
	if parseErr != nil {
		log.Printf("[WARN:REFRESH:LAST-MODIFIED] %s%s invalid Last-Modified header %q: %v\n", localFile.Host, localFile.Path, lastmod, parseErr)
		return lastModified, false
	}

	lastModified = parsedLastModified
	if !lastAccess.RemoteLastModified.IsZero() && lastModified.Before(lastAccess.RemoteLastModified) {
		if err := c.UpdateLastChecked(protocol, localFile.Host, localFile.Path); err != nil {
			log.Printf("[ERROR:REFRESH:NOTMODIFIED:LAST-MODIFIED] %s%s failed to update last checked: %v\n", localFile.Host, localFile.Path, err)
		}
		log.Printf("[INFO:REFRESH:NOTMODIFIED:LAST-MODIFIED] %s%s has not changed\n", localFile.Host, localFile.Path)
		return lastModified, true
	}

	return lastModified, false
}

// isUnchangedByETag returns true if the origin reports the same entity tag.
func (c *FSCache) isUnchangedByETag(etag string, protocol int, localFile *url.URL, previousETag string) bool {
	if etag == "" || etag != previousETag {
		return false
	}

	if err := c.UpdateLastChecked(protocol, localFile.Host, localFile.Path); err != nil {
		log.Printf("[ERROR:REFRESH:NOTMODIFIED:ETAG] %s%s failed to update last checked: %v\n", localFile.Host, localFile.Path, err)
	}
	log.Printf("[INFO:REFRESH:NOTMODIFIED:ETAG] %s%s has not changed\n", localFile.Host, localFile.Path)
	return true
}

// downloadResponseToFile stores the response body in a temp file and atomically swaps it in.
func downloadResponseToFile(resp *http.Response, generatedName string) (int64, string, error) {
	requiredSize := resp.ContentLength
	if requiredSize > 0 {
		if err := ensureDiskSpace(generatedName, requiredSize); err != nil {
			log.Printf("[ERROR:REFRESH:DISK] %s %s\n", generatedName, err)
			return 0, "", err
		}
	}

	tmpID, err := uuid.NewRandom()
	if err != nil {
		log.Printf("[ERROR:REFRESH:RANDOM] %s\n", err)
		return 0, "", err
	}

	tempPath := generatedName + "-dl-" + tmpID.String()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	file, err := os.Create(tempPath)
	if err != nil {
		log.Printf("[ERROR:REFRESH:CREATE] %s\n", err)
		return 0, "", err
	}

	if err := preallocateFile(file, requiredSize); err != nil {
		log.Printf("[ERROR:REFRESH:PREALLOCATE] %s\n", err)
		file.Close()
		return 0, "", err
	}

	wrb, err := io.Copy(file, resp.Body)
	if err != nil {
		log.Printf("[ERROR:REFRESH:WRITE] %s\n", err)
		file.Close()
		return 0, "", err
	}

	if err := file.Close(); err != nil {
		log.Printf("[ERROR:REFRESH:CLOSE] %s\n", err)
		return 0, "", err
	}

	if resp.ContentLength > 0 && resp.ContentLength != wrb {
		err := fmt.Errorf("downloaded size mismatch: expected %d bytes, got %d", resp.ContentLength, wrb)
		log.Printf("[ERROR:REFRESH:LENGTH] %s\n", err)
		return 0, "", err
	}

	newHash, err := GenerateSHA256Hash(tempPath)
	if err != nil {
		log.Printf("[ERROR:REFRESH:HASH] %s\n", err)
		return 0, "", err
	}

	if err := os.Rename(tempPath, generatedName); err != nil {
		log.Printf("[ERROR:REFRESH:RENAME] %s\n", err)
		return 0, "", err
	}
	cleanupTemp = false

	return wrb, newHash, nil
}
