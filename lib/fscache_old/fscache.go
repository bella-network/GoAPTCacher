package fscache_old

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FSCache struct {
	Path       string
	MaximumTTL time.Duration

	// local write cache for parallel requests
	writeQueue map[string]time.Time
	client     *http.Client
	rw         sync.RWMutex

	// redownload cache
	recentCache map[string]RecentFileCache
	rdcRw       sync.RWMutex
}

func NewFSCache(path string, maximumTTL time.Duration) *FSCache {
	return &FSCache{
		Path:       path,
		MaximumTTL: maximumTTL,
		writeQueue: make(map[string]time.Time),
		client: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				Proxy:               nil,
				MaxIdleConnsPerHost: 5,
			},
		},
	}
}

// Print all writeQueue entries
func (c *FSCache) PrintWriteQueue() {
	c.rw.RLock()
	defer c.rw.RUnlock()
	for k, v := range c.writeQueue {
		log.Printf("Key: %s, Value: %v\n", k, v)
	}
}

// ServeFromRequest checks the received request and serves the file from the cache if it exists and is not expired.
func (c *FSCache) ServeFromRequest(r *http.Request, w http.ResponseWriter) {
	// Parse the request and get the file path
	filePath := c.Path + r.URL.Host + r.URL.Path

	// Check if the file exists in the cache
	if fi, err := os.Stat(filePath); err == nil {
		// Add header that describes the cache hit
		w.Header().Add("X-Cache", "HIT")

		// Check if If-Modified-Since header is present and return 304 Not Modified if the file has not been modified
		if r.Header.Get("If-Modified-Since") != "" {
			// Parse the If-Modified-Since header from "Sun, 29 Sep 2024 14:34:29 GMT" to a time.Time object
			ifModifiedSince, err := time.Parse(time.RFC1123, r.Header.Get("If-Modified-Since"))
			if err != nil {
				http.Error(w, "Error parsing If-Modified-Since header", http.StatusInternalServerError)
				log.Printf("Error parsing If-Modified-Since header: %v\n", err)
				return
			}

			// Check if the file has been modified since the If-Modified-Since header
			if fi.ModTime().Before(ifModifiedSince) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		// Serve the file from the cache
		http.ServeFile(w, r, filePath)
		return
	}

	// Add header that describes the cache miss
	w.Header().Add("X-Cache", "MISS")

	// Serve the file from the original source
	c.FetchAndStream(r, w, filePath)
}

// FetchAndStream fetches the file from the original source and streams it to the client.
func (c *FSCache) FetchAndStream(r *http.Request, w http.ResponseWriter, filePath string) {
	c.rw.RLock()
	// check if current requested file is already being fetched
	if _, ok := c.writeQueue[filePath]; ok {
		c.rw.RUnlock()
		log.Printf("File %s is already being fetched\n", filePath)
		// wait for the file to be fetched
		time.Sleep(100 * time.Millisecond)
		c.FetchAndStream(r, w, filePath)
		return
	}
	c.rw.RUnlock()

	// check if the file exists in the cache
	if _, err := os.Stat(filePath); err == nil {
		log.Printf("File %s exists already in cache\n", filePath)
		c.ServeFromRequest(r, w)
		return
	}

	// Fetch the file from the original source
	c.rw.Lock()
	c.writeQueue[filePath] = time.Now()
	c.rw.Unlock()

	log.Printf("Fetching file %s\n", filePath)

	// Fetch the file from the original source
	req, err := http.NewRequest(r.Method, r.URL.String(), nil)
	if err != nil {
		http.Error(w, "Error creating request", http.StatusInternalServerError)
		return
	}

	// Copy headers from the original request to the new request
	for key, values := range r.Header {
		for _, value := range values {
			// Skip E-Tag and If-Modified-Since headers as this would return a 304 Not Modified response
			if key == "If-Modified-Since" || key == "If-None-Match" {
				continue
			}

			req.Header.Add(key, value)
		}
	}

	// Add a header to indicate that the request is coming from the cache
	req.Header.Add("X-Forwarded-For", r.RemoteAddr)
	req.Header.Add("X-Proxy-Server", "goaptcacher")

	// Send the request to the original source
	resp, err := c.client.Do(req)
	if err != nil {
		http.Error(w, "Error fetching file", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Check if the response status code is OK
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Error fetching file", http.StatusNotFound)
		log.Printf("Error fetching file %s: %v\n", filePath, resp.Status)
		return
	}

	// Create the cache directory if it does not exist
	err = os.MkdirAll(filepath.Dir(filePath), 0755)
	if err != nil {
		log.Printf("Error creating cache directory: %v\n", err)
		return
	}

	// Write the file to the cache
	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	// Use io.MultiWriter to write to both the file and the response writer
	multiWriter := io.MultiWriter(w, file)

	// Write the file to the cache
	bw, err := io.Copy(multiWriter, resp.Body)
	if err != nil {
		log.Printf("Error writing file: %v\n", err)
		return
	}

	// Remove the file from the write queue
	c.rw.Lock()
	delete(c.writeQueue, filePath)
	c.rw.Unlock()

	log.Printf("Wrote %d bytes to %s\n", bw, filePath)
}
