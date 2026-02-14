package fscache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	w.Header().Set("X-Proxy-Server", fmt.Sprintf("GoAptCacher/%s", buildinfo.Version))

	// If a file from path /pool/ is requested, check at first if the file is
	// available on the local file system to be directly served. This speeds up
	// requests for Debian packages significantly. If some weird URL is used
	// which also contains /dists/, skip this optimization as this could freeze
	// updates permanently.
	localPath := c.buildLocalPath(r.URL)
	if _, err := os.Stat(localPath); strings.Contains(localPath, "/pool/") && !strings.Contains(localPath, "/dists/") && err == nil {
		// File exists, serve it directly to the client.
		c.serveLocalFile(w, r, localPath)

		// Perform background tasks for the cached file.
		go c.backgroundFileTasks(r.URL)
		return
	}

	// Check the access cache for the requested file to see if it is available,
	// which then allows a direct cache hit and serving the file directly.
	lastAccess, ok := c.Get(protocol, r.URL.Host, r.URL.Path)
	if ok {
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

		// Serve the file
		c.serveLocalFile(w, r, localPath)

		// Perform background tasks for the cached file.
		go c.backgroundFileTasks(r.URL)

		return
	}

	// Cache was missed, download the file from the internet and serve it to the client.
	c.serveGETRequestCacheMiss(r, w, 0)
}

// serveLocalFile serves a local file to the client.
func (c *FSCache) serveLocalFile(w http.ResponseWriter, r *http.Request, localPath string) {
	protocol := DetermineProtocolFromURL(r.URL)

	// Direct cache hit, serve the file directly to the client and return.
	c.CreateFileLock(protocol, r.URL.Host, r.URL.Path)
	// Remove the file lock
	defer c.RemoveFileLock(protocol, r.URL.Host, r.URL.Path)

	// Get file info
	info, err := os.Stat(localPath)
	if err != nil {
		http.Error(w, "Error accessing cached file", http.StatusInternalServerError)
		log.Printf("[ERROR:GET:STAT] %s - Error accessing cached file: %v\n", r.URL.String(), err)
		return
	}

	// Set headers
	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))

	// Serve the file
	http.ServeFile(w, r, localPath)

	// Log the cache hit
	log.Printf("[INFO:GET:HIT:%s] %s\n", r.RemoteAddr, r.URL.String())
	go c.TrackRequest(true, info.Size())
}

// backgroundFileTasks performs background tasks for a cached file, determines
// if upstream is checked for updates and updates local tracking info.
func (c *FSCache) backgroundFileTasks(request *url.URL) {
	// Perform background tasks to update access cache, hit count, URL list and refresh
	protocol := DetermineProtocolFromURL(request)

	lastAccess, ok := c.Get(protocol, request.Host, request.Path)
	if !ok {
		// File is not in access cache, nothing to do.
		return
	}

	// Check if the file should be rechecked
	if c.evaluateRefresh(request, lastAccess) {
		// File should be checked if a new version is available on the
		// internet for cache refresh.
		go c.cacheRefresh(request, lastAccess)
	}

	go c.Hit(protocol, request.Host, request.Path)
	go c.AddURLIfNotExists(protocol, request.Host, request.Path, request.String())
}

// serveGETRequestCacheMiss is the function to serve a GET request for a client if the cache was missed.
func (c *FSCache) serveGETRequestCacheMiss(r *http.Request, w http.ResponseWriter, retry uint64) {
	c.serveGETRequestCacheMissWithSleep(r, w, retry, time.Sleep)
}

func (c *FSCache) serveGETRequestCacheMissWithSleep(r *http.Request, w http.ResponseWriter, retry uint64, sleepFn func(time.Duration)) {
	if sleepFn == nil {
		sleepFn = time.Sleep
	}

	protocol := DetermineProtocolFromURL(r.URL)

	if c.retryLimitReached(r, w, retry) {
		return
	}

	if !c.acquireWriteLockOrRetry(protocol, r, w, retry, sleepFn) {
		return
	}
	defer c.DeleteWriteLock(protocol, r.URL.Host, r.URL.Path)

	if c.serveRecoveredCacheMiss(protocol, r, w) {
		return
	}

	c.fetchAndServeCacheMiss(protocol, r, w)
}

func (c *FSCache) retryLimitReached(r *http.Request, w http.ResponseWriter, retry uint64) bool {
	if retry <= 25 {
		return false
	}

	log.Printf("[ERROR:GET:RETRY:%d] %s%s - Too many retries, giving up\n", retry, r.URL.Host, r.URL.Path)
	http.Error(
		w,
		"File is currently being downloaded, please try again later",
		http.StatusInternalServerError,
	)
	return true
}

func (c *FSCache) acquireWriteLockOrRetry(
	protocol int,
	r *http.Request,
	w http.ResponseWriter,
	retry uint64,
	sleepFn func(time.Duration),
) bool {
	created := c.CreateExclusiveWriteLock(protocol, r.URL.Host, r.URL.Path)
	if created {
		return true
	}

	sleepFn(time.Second)
	c.serveGETRequestCacheMissWithSleep(r, w, retry+1, sleepFn)
	return false
}

func (c *FSCache) serveRecoveredCacheMiss(protocol int, r *http.Request, w http.ResponseWriter) bool {
	if _, ok := c.Get(protocol, r.URL.Host, r.URL.Path); ok {
		c.serveGETRequest(r, w)
		return true
	}

	localPath := c.buildLocalPath(r.URL)
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return false
	}

	hash, err := GenerateSHA256Hash(localPath)
	if err != nil {
		log.Printf("Error generating SHA256 hash: %v\n", err)
		http.Error(w, "Error generating file hash", http.StatusInternalServerError)
		return true
	}

	err = c.Set(protocol, r.URL.Host, r.URL.Path, AccessEntry{
		RemoteLastModified: fileInfo.ModTime(),
		LastAccessed:       time.Now(),
		URL:                r.URL,
		Size:               fileInfo.Size(),
		SHA256:             hash,
	})
	if err != nil {
		log.Printf("Error updating access cache: %v\n", err)
		http.Error(w, "Error updating cache metadata", http.StatusInternalServerError)
		return true
	}

	w.Header().Add("X-Cache", "ROUNDTRIP")
	c.serveGETRequest(r, w)
	return true
}

func (c *FSCache) fetchAndServeCacheMiss(protocol int, r *http.Request, w http.ResponseWriter) {
	req, err := c.newCacheMissUpstreamRequest(r)
	if err != nil {
		http.Error(w, "Error creating request", http.StatusInternalServerError)
		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		http.Error(w, "Error fetching file", http.StatusInternalServerError)
		log.Printf("[ERROR:GET:FETCH] %s%s - Error fetching file: %v\n", r.URL.Host, r.URL.Path, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Error fetching file", http.StatusNotFound)
		log.Printf("[ERROR:GET:STATUS:%d] %s%s - Error fetching file: received status code %d\n", resp.StatusCode, r.URL.Host, r.URL.Path, resp.StatusCode)
		return
	}

	c.streamCacheMissResponse(protocol, r, w, resp)
}

func (c *FSCache) newCacheMissUpstreamRequest(r *http.Request) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, r.URL.String(), nil)
	if err != nil {
		return nil, err
	}

	for key, values := range r.Header {
		if skipRequestHeaderForCacheMiss(key) {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	req.Header.Set(
		"X-Proxy-Server",
		fmt.Sprintf("GoAptCacher/%s (+https://gitlab.com/bella.network/goaptcacher)", buildinfo.Version),
	)
	return req, nil
}

func skipRequestHeaderForCacheMiss(key string) bool {
	if key == "If-Modified-Since" || key == "If-None-Match" || key == "E-Tag" {
		return true
	}

	_, skip := hopByHopHeaders[http.CanonicalHeaderKey(key)]
	return skip
}

func (c *FSCache) streamCacheMissResponse(protocol int, r *http.Request, w http.ResponseWriter, resp *http.Response) {
	targetPath := c.buildLocalPath(r.URL)
	requiredSize, ok := c.prepareCacheMissTarget(targetPath, r, w, resp)
	if !ok {
		return
	}

	tempPath := buildTempCachePath(targetPath)
	defer func() {
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
	}()

	file, ok := c.createCacheMissTempFile(tempPath, requiredSize, w)
	if !ok {
		return
	}

	bw, hash, ok := streamResponseToClientAndCache(w, resp, file)
	if !ok {
		return
	}

	if resp.ContentLength > 0 && resp.ContentLength != bw {
		log.Printf("Error writing file: expected %d bytes, got %d\n", resp.ContentLength, bw)
		return
	}

	lastModifiedTime := parseLastModifiedForMetadata(resp.Header.Get("Last-Modified"))
	if !c.finalizeCacheMissFile(tempPath, targetPath, lastModifiedTime, w) {
		return
	}
	tempPath = ""

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

func (c *FSCache) prepareCacheMissTarget(
	targetPath string,
	r *http.Request,
	w http.ResponseWriter,
	resp *http.Response,
) (int64, bool) {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		log.Printf("Error creating cache directory: %v\n", err)
		http.Error(w, "Error creating cache directory", http.StatusInternalServerError)
		return 0, false
	}

	requiredSize := resp.ContentLength
	if requiredSize > 0 {
		if err := ensureDiskSpace(targetPath, requiredSize); err != nil {
			log.Printf("[ERROR:GET:DISK] Error reserving disk space for %s%s: %v\n", r.URL.Host, r.URL.Path, err)
			http.Error(w, "Insufficient storage on cache server", http.StatusInsufficientStorage)
			return 0, false
		}
	}

	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("X-Cache", "MISS")
	setConditionalCacheMissHeaders(w, resp)
	return requiredSize, true
}

func setConditionalCacheMissHeaders(w http.ResponseWriter, resp *http.Response) {
	lastModified := resp.Header.Get("Last-Modified")
	if lastModified != "" {
		parsed, err := time.Parse(time.RFC1123, lastModified)
		if err != nil {
			log.Printf("Invalid Last-Modified header: %s (%v)", lastModified, err)
		} else {
			w.Header().Set("Last-Modified", parsed.UTC().Format(time.RFC1123))
		}
	}

	if eTag := resp.Header.Get("ETag"); eTag != "" {
		w.Header().Set("ETag", eTag)
	}
}

func buildTempCachePath(targetPath string) string {
	randomName := uuid.New().String()
	return targetPath + "." + randomName + ".partial"
}

func (c *FSCache) createCacheMissTempFile(tempPath string, requiredSize int64, w http.ResponseWriter) (*os.File, bool) {
	file, err := os.Create(tempPath)
	if err != nil {
		log.Printf("Error creating file: %v\n", err)
		return nil, false
	}

	if requiredSize > 0 {
		if err := preallocateFile(file, requiredSize); err != nil {
			log.Printf("Error preallocating file: %v\n", err)
			_ = file.Close()
			http.Error(w, "Error reserving storage", http.StatusInternalServerError)
			return nil, false
		}
	}

	return file, true
}

func streamResponseToClientAndCache(w http.ResponseWriter, resp *http.Response, file *os.File) (int64, string, bool) {
	w.WriteHeader(resp.StatusCode)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	clientWriter := responseWriterWithFlush(w)
	hasher := sha256.New()
	cacheDropper := newCacheDropWriter(file, cacheDropThreshold, cacheDropChunk)
	multiWriter := io.MultiWriter(clientWriter, cacheDropper, hasher)
	copyBuf := make([]byte, 32*1024)
	reader := readerOnly{r: resp.Body}

	bw, err := io.CopyBuffer(multiWriter, reader, copyBuf)
	if err != nil {
		log.Printf("Error writing file: %v\n", err)
		return 0, "", false
	}
	cacheDropper.DropCache()

	if err := file.Close(); err != nil {
		log.Printf("Error closing file: %v\n", err)
		return 0, "", false
	}

	return bw, hex.EncodeToString(hasher.Sum(nil)), true
}

func responseWriterWithFlush(w http.ResponseWriter) io.Writer {
	if flusher, ok := w.(http.Flusher); ok {
		return flushWriter{w: w, flusher: flusher}
	}
	return w
}

func parseLastModifiedForMetadata(lastModified string) time.Time {
	lastModifiedTime := time.Now()
	if lastModified == "" {
		return lastModifiedTime
	}

	parsed, err := time.Parse(time.RFC1123, lastModified)
	if err != nil {
		log.Printf("Error parsing Last-Modified header: %v\n", err)
		return lastModifiedTime
	}

	return parsed
}

func (c *FSCache) finalizeCacheMissFile(
	tempPath string,
	targetPath string,
	lastModifiedTime time.Time,
	w http.ResponseWriter,
) bool {
	if err := os.Rename(tempPath, targetPath); err != nil {
		log.Printf("Error renaming file: %v\n", err)
		http.Error(w, "Error renaming file", http.StatusInternalServerError)
		return false
	}

	if !lastModifiedTime.IsZero() && lastModifiedTime.Year() > 2000 {
		if err := os.Chtimes(targetPath, time.Now(), lastModifiedTime); err != nil {
			log.Printf("Error setting file times: %v\n", err)
		}
	}

	return true
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

// readerOnly hides optional interfaces (like io.WriterTo) to keep io.CopyBuffer
// using the provided buffer size.
type readerOnly struct {
	r io.Reader
}

func (ro readerOnly) Read(p []byte) (int, error) {
	return ro.r.Read(p)
}

// flushWriter forces periodic flushes so clients receive streamed data.
type flushWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.flusher.Flush()
	return n, err
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
