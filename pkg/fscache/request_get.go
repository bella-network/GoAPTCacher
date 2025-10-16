package fscache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"gitlab.com/bella.network/goaptcacher/pkg/buildinfo"
)

// hopByHopHeaders lists headers that must not be forwarded to the client when
// proxying a request. These are defined by RFC 9110 section 7.6.1.
var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Proxy-Connection":    {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

// serveGETRequest is the basic function to serve a GET request for a client.
func (c *FSCache) serveGETRequest(r *http.Request, w http.ResponseWriter) {
	protocol := DetermineProtocolFromURL(r.URL)

	// Set basic headers for the response
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Cache-Control", "public, max-age=900")
	w.Header().Set("Server", fmt.Sprintf("GoAptCacher/%s (+https://gitlab.com/bella.network/goaptcacher)", buildinfo.Version))

	// Check the access cache for the requested file
	lastAccess, ok := c.Get(protocol, r.URL.Host, r.URL.Path)
	if ok {
		localPath := c.buildLocalPath(r.URL)
		if info, err := os.Stat(localPath); err != nil || (lastAccess.Size > 0 && info.Size() != lastAccess.Size) {
			if err != nil {
				if !os.IsNotExist(err) {
					log.Printf("[WARN:GET:STALE] %s%s stat failed: %v\n", r.URL.Host, r.URL.Path, err)
				}
			} else {
				log.Printf("[WARN:GET:STALE] %s%s size mismatch: expected %d bytes, got %d\n", r.URL.Host, r.URL.Path, lastAccess.Size, info.Size())
			}
			c.Delete(protocol, r.URL.Host, r.URL.Path)
			_ = os.Remove(localPath)
			c.serveGETRequestCacheMiss(r, w, 0)
			return
		}

		// Remove the port from remote address if it is present
		remoteAddr := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			remoteAddr = host
		}

		// Update last hit time for the file
		c.Hit(protocol, r.URL.Host, r.URL.Path)
		c.AddURLIfNotExists(protocol, r.URL.Host, r.URL.Path, r.URL.String())

		// Set header that describes the cache hit
		w.Header().Set("X-Cache", "HIT")

		// Check if the file should be rechecked
		if c.evaluateRefresh(r.URL, lastAccess) {
			// File should be checked if a new version is available on the
			// internet for cache refresh.
			go c.cacheRefresh(r.URL, lastAccess)
		}

		// Add the Last-Modified header to the response
		if !lastAccess.RemoteLastModified.IsZero() && lastAccess.RemoteLastModified.Year() > 2000 {
			// Force the Last-Modified header to be in RFC1123 and GMT format as
			// this is the format used by HTTP.
			w.Header().Set("Last-Modified", lastAccess.RemoteLastModified.UTC().Format(time.RFC1123))
		}

		// Add the ETag header to the response if available
		if lastAccess.ETag != "" {
			w.Header().Set("ETag", lastAccess.ETag)
		}

		// Add the SHA256 header to the response if available
		if lastAccess.SHA256 != "" {
			w.Header().Set("X-SHA256", lastAccess.SHA256)
		}

		// Client may has delivered the header "If-Modified-Since". If the file has not been modified since the
		// given time, we can return a 304 Not Modified response.
		if ifModifiedSince := r.Header.Get("If-Modified-Since"); ifModifiedSince != "" {
			parsedTime, err := time.Parse(time.RFC1123, ifModifiedSince)
			if err != nil {
				http.Error(w, "Error parsing If-Modified-Since header", http.StatusInternalServerError)
				return
			}

			// Check if the file has been modified since the If-Modified-Since header
			if !lastAccess.RemoteLastModified.IsZero() && lastAccess.RemoteLastModified.Before(parsedTime) && lastAccess.RemoteLastModified.Year() > 2000 {
				w.WriteHeader(http.StatusNotModified)
				log.Printf("[INFO:GET:NOTMODIFIED:%s] %s%s\n", remoteAddr, r.URL.Host, r.URL.Path)
				go c.TrackRequest(true, 0)
				return
			}
		}

		// Direct cache hit, serve the file directly to the client and return.
		c.CreateFileLock(protocol, r.URL.Host, r.URL.Path)
		// Remove the file lock
		defer c.RemoveFileLock(protocol, r.URL.Host, r.URL.Path)

		// Serve the file to the client
		http.ServeFile(w, r, c.buildLocalPath(r.URL))

		// Log the cache hit
		log.Printf("[INFO:GET:HIT:%s] %s\n", remoteAddr, r.URL.String())
		go c.TrackRequest(true, lastAccess.Size)

		return
	}

	// Cache was missed, download the file from the internet and serve it to the client.
	c.serveGETRequestCacheMiss(r, w, 0)
}

// serveGETRequestCacheMiss is the function to serve a GET request for a client if the cache was missed.
func (c *FSCache) serveGETRequestCacheMiss(r *http.Request, w http.ResponseWriter, retry uint64) {
	protocol := DetermineProtocolFromURL(r.URL)

	// If retry count is too high, return an error to the client.
	if retry > 25 {
		log.Printf("[ERROR:GET:RETRY:%d] %s%s - Too many retries, giving up\n", retry, r.URL.Host, r.URL.Path)
		http.Error(
			w,
			"File is currently being downloaded, please try again later",
			http.StatusInternalServerError,
		)
		return
	}

	// File is requested to be direcrly downloaded. As parallel downloads are
	// possible of the same file, block all other requests for the same file
	// until the download is finished.
	created := c.CreateExclusiveWriteLock(protocol, r.URL.Host, r.URL.Path)
	if !created {
		time.Sleep(time.Second)
		c.serveGETRequestCacheMiss(r, w, retry+1)
		return
	}
	defer c.DeleteWriteLock(protocol, r.URL.Host, r.URL.Path)

	// File might be downloaded by another request, check again.
	_, ok := c.Get(protocol, r.URL.Host, r.URL.Path)
	if ok {
		// Retry the request.
		c.serveGETRequest(r, w)
		return
	}

	// Check if the file exists in the cache directory.
	if fileInfo, err := os.Stat(c.buildLocalPath(r.URL)); err == nil {
		// File exists, but is not in the access cache. This can happen if the
		// cache was deleted or the file was added manually.

		// Generate SHA256 hash of the file
		hash, err := GenerateSHA256Hash(c.buildLocalPath(r.URL))
		if err != nil {
			log.Printf("Error generating SHA256 hash: %v\n", err)
			http.Error(w, "Error generating file hash", http.StatusInternalServerError)
			return
		}

		c.Set(protocol, r.URL.Host, r.URL.Path, AccessEntry{
			RemoteLastModified: fileInfo.ModTime(),
			LastAccessed:       time.Now(),
			URL:                r.URL,
			Size:               fileInfo.Size(),
			SHA256:             hash,
		})

		// Set header that describes the cache hit
		w.Header().Add("X-Cache", "ROUNDTRIP")

		// Retry the request.
		c.serveGETRequest(r, w)
		return
	}

	// Fetch the file from the original source
	req, err := http.NewRequest("GET", r.URL.String(), nil)
	if err != nil {
		http.Error(w, "Error creating request", http.StatusInternalServerError)
		return
	}

	// Copy headers from the original request to the new request
	for key, values := range r.Header {
		for _, value := range values {
			// Skip E-Tag and If-Modified-Since headers as this would return a 304 Not Modified response
			if key == "If-Modified-Since" || key == "If-None-Match" || key == "E-Tag" {
				continue
			}
			// Skip hop-by-hop headers as these are handled by the proxy
			if _, skip := hopByHopHeaders[http.CanonicalHeaderKey(key)]; skip {
				continue
			}

			req.Header.Add(key, value)
		}
	}

	// Add a header to indicate that the request is coming from the cache
	req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	req.Header.Set(
		"X-Proxy-Server",
		fmt.Sprintf("GoAptCacher/%s (+https://gitlab.com/bella.network/goaptcacher)", buildinfo.Version),
	)

	// Send the request to the original source
	resp, err := c.client.Do(req)
	if err != nil {
		http.Error(w, "Error fetching file", http.StatusInternalServerError)
		log.Printf("[ERROR:GET:FETCH] %s%s - Error fetching file: %v\n", r.URL.Host, r.URL.Path, err)
		return
	}
	defer resp.Body.Close()

	// Check if the response status code is OK
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Error fetching file", http.StatusNotFound)
		log.Printf("[ERROR:GET:STATUS:%d] %s%s - Error fetching file: received status code %d\n", resp.StatusCode, r.URL.Host, r.URL.Path, resp.StatusCode)
		return
	}

	targetPath := c.buildLocalPath(r.URL)

	// Create the cache directory if it does not exist
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		log.Printf("Error creating cache directory: %v\n", err)
		http.Error(w, "Error creating cache directory", http.StatusInternalServerError)
		return
	}

	// Ensure that there is enough disk space to store the file If
	// Content-Length is not set, we cannot reserve space and just try to
	// download the file. This might lead to a failed download if the disk is
	// full.
	requiredSize := resp.ContentLength
	if requiredSize > 0 {
		if err := ensureDiskSpace(targetPath, requiredSize); err != nil {
			log.Printf("[ERROR:GET:DISK] Error reserving disk space for %s%s: %v\n", r.URL.Host, r.URL.Path, err)
			http.Error(w, "Insufficient storage on cache server", http.StatusInsufficientStorage)
			return
		}
	}

	// Forward headers from the upstream response before streaming the body.
	copyResponseHeaders(w.Header(), resp.Header)

	// Set header that describes the cache hit
	w.Header().Set("X-Cache", "MISS")

	// Set Last-Modified header if available
	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		if parsed, err := time.Parse(time.RFC1123, lastModified); err == nil {
			w.Header().Set("Last-Modified", parsed.UTC().Format(time.RFC1123))
		} else {
			log.Printf("Invalid Last-Modified header: %s (%v)", lastModified, err)
		}
	}

	// Set ETag header if available
	if eTag := resp.Header.Get("ETag"); eTag != "" {
		w.Header().Set("ETag", eTag)
	}

	// Create a UUID for the file to prevent conflicts with other downloads
	randomName := uuid.New().String()
	tempPath := targetPath + "." + randomName + ".partial"
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	// Write the file to the cache asynchronously so the response to the
	// client is not limited by disk throughput. A small buffer is used to
	// prevent unbounded memory usage while still decoupling the disk write.
	file, err := os.Create(tempPath)
	if err != nil {
		log.Printf("Error creating file: %v\n", err)
		return
	}

	// Preallocate the file to the required size to prevent fragmentation and
	// ensure that writes are atomic. This is only done if Content-Length is
	// set.
	if requiredSize > 0 {
		if err := preallocateFile(file, requiredSize); err != nil {
			log.Printf("Error preallocating file: %v\n", err)
			file.Close()
			http.Error(w, "Error reserving storage", http.StatusInternalServerError)
			return
		}
	}

	// Write the response status now that headers are populated and storage is reserved.
	w.WriteHeader(resp.StatusCode)

	// Create an async file writer that writes to the file with a buffer size of
	// 32KB This allows the file to be written asynchronously while still being
	// able to stream the response to the client.
	asyncWriter := newAsyncFileWriter(file, 32)
	multiWriter := io.MultiWriter(w, asyncWriter)

	// Stream data to the client and asynchronously to disk.
	bw, err := io.Copy(multiWriter, resp.Body)
	if errClose := asyncWriter.Close(); err == nil && errClose != nil {
		err = errClose
	}
	if err != nil {
		log.Printf("Error writing file: %v\n", err)
		return
	}

	// Check if the number of bytes written matches the Content-Length header
	if resp.ContentLength > 0 && resp.ContentLength != bw {
		log.Printf("Error writing file: expected %d bytes, got %d\n", resp.ContentLength, bw)
		return
	}

	// Check if Last-Modified header is set and can be parsed as a time
	lastModifiedTime := time.Now()
	lastModified := resp.Header.Get("Last-Modified")
	if lastModified != "" {
		// Parse the Last-Modified header of Mon, 30 Sep 2024 22:10:24 GMT format
		lastModifiedTime, err = time.Parse(time.RFC1123, lastModified)
		if err != nil {
			log.Printf("Error parsing Last-Modified header: %v\n", err)
		}
	}

	// Generate SHA256 hash of the downloaded file
	hash, err := GenerateSHA256Hash(tempPath)
	if err != nil {
		log.Printf("Error generating SHA256 hash: %v\n", err)
		http.Error(w, "Error generating file hash", http.StatusInternalServerError)
		return
	}

	// Rename the file to its final name
	err = os.Rename(tempPath, targetPath)
	if err != nil {
		log.Printf("Error renaming file: %v\n", err)
		http.Error(w, "Error renaming file", http.StatusInternalServerError)
		return
	}
	cleanupTemp = false

	// Update the access cache with the new file
	if err := c.Set(protocol, r.URL.Host, r.URL.Path, AccessEntry{
		RemoteLastModified: lastModifiedTime,
		LastAccessed:       time.Now(),
		LastChecked:        time.Now(),
		ETag:               resp.Header.Get("ETag"),
		URL:                r.URL,
		Size:               bw,
		SHA256:             hash,
	}); err != nil {
		log.Printf("Error updating access cache: %v\n", err)
	}

	log.Printf("[INFO:DL:CREATED] %s%s - Wrote %d bytes\n", r.URL.Host, r.URL.Path, bw)
	go c.TrackRequest(false, bw)
}

// copyResponseHeaders copies response headers from src to dst while stripping
// hop-by-hop headers that must be handled locally by this proxy.
func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if _, skip := hopByHopHeaders[http.CanonicalHeaderKey(key)]; skip {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// GenerateSHA256Hash generates a SHA256 hash of the file at the given path.
func GenerateSHA256Hash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
