package fscache

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// serveGETRequest is the basic function to serve a GET request for a client.
func (c *FSCache) serveGETRequest(r *http.Request, w http.ResponseWriter) {
	lastAccess, ok := c.accessCache.Get(r.URL.Host, r.URL.Path)
	if ok {
		// Update last hit time for the file
		c.accessCache.Hit(r.URL.Host, r.URL.Path)
		c.accessCache.AddURLIfNotExists(r.URL.Host, r.URL.Path, r.URL.String())

		// Set header that describes the cache hit
		r.Header.Add("X-Cache", "HIT")

		// Check if the file should be rechecked
		if c.evaluateRefresh(r.URL, lastAccess) {
			// File should be checked if a new version is available on the
			// internet for cache refresh.
			go c.cacheRefesh(r.URL, lastAccess)
		}

		// Add the Last-Modified header to the response
		if !lastAccess.RemoteLastModified.IsZero() {
			w.Header().Set("Last-Modified", lastAccess.RemoteLastModified.Format(time.RFC1123))
		}

		// Add the ETag header to the response if available
		if lastAccess.ETag != "" {
			w.Header().Set("ETag", lastAccess.ETag)
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
			if lastAccess.RemoteLastModified.Before(parsedTime) {
				w.WriteHeader(http.StatusNotModified)
				log.Printf("[INFO:GET:NOTMODIFIED:%s] %s%s\n", r.RemoteAddr, r.URL.Host, r.URL.Path)
				go c.accessCache.TrackRequest(true, 0)
				return
			}
		}

		// Direct cache hit, serve the file directly to the client and return.
		c.accessCache.CreateFileLock(r.URL.Host, r.URL.Path)
		// Remove the file lock
		defer c.accessCache.RemoveFileLock(r.URL.Host, r.URL.Path)

		// Serve the file to the client
		http.ServeFile(w, r, c.buildLocalPath(r.URL))

		log.Printf("[INFO:GET:HIT:%s] %s\n", r.RemoteAddr, r.URL.String())
		go c.accessCache.TrackRequest(true, lastAccess.Size)

		return
	}

	// Cache was missed, download the file from the internet and serve it to the client.
	c.serveGETRequestCacheMiss(r, w, 0)
}

// serveGETRequestCacheMiss is the function to serve a GET request for a client if the cache was missed.
func (c *FSCache) serveGETRequestCacheMiss(r *http.Request, w http.ResponseWriter, retry uint64) {
	// If retry count is too high, return an error to the client.
	if retry > 25 {
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
	created := c.accessCache.CreateExclusiveWriteLock(r.URL.Host, r.URL.Path)
	if !created {
		time.Sleep(time.Second)
		c.serveGETRequestCacheMiss(r, w, retry+1)
		return
	}
	defer c.accessCache.DeleteWriteLock(r.URL.Host, r.URL.Path)

	// File might be downloaded by another request, check again.
	_, ok := c.accessCache.Get(r.URL.Host, r.URL.Path)
	if ok {
		// Retry the request.
		c.serveGETRequest(r, w)
		return
	}

	// Check if the file exists in the cache directory.
	if fileInfo, err := os.Stat(c.buildLocalPath(r.URL)); err == nil {
		// File exists, but is not in the access cache. This can happen if the
		// cache was deleted or the file was added manually.
		c.accessCache.Set(r.URL.Host, r.URL.Path, AccessEntry{
			RemoteLastModified: fileInfo.ModTime(),
			LastAccessed:       time.Now(),
			URL:                r.URL.String(),
			Size:               fileInfo.Size(),
		})

		// Set header that describes the cache hit
		r.Header.Add("X-Cache", "ROUNDTRIP")

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

			req.Header.Add(key, value)
		}
	}

	// Add a header to indicate that the request is coming from the cache
	req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	req.Header.Set("X-Proxy-Server", "goaptcacher")

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
		return
	}

	// Set header that describes the cache hit
	r.Header.Add("X-Cache", "MISS")

	// Set Last-Modified header if available
	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		w.Header().Set("Last-Modified", lastModified)
	}

	// Set ETag header if available
	if eTag := resp.Header.Get("ETag"); eTag != "" {
		w.Header().Set("ETag", eTag)
	}

	// Body needs to be read 2 times, once to stream to the client and once to write to the cache
	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, &buf)
	_, err = io.Copy(w, tee)
	if err != nil {
		http.Error(w, "Error streaming file", http.StatusInternalServerError)
		log.Printf("Error streaming file: %v\n", err)
		return
	}

	// Create the cache directory if it does not exist
	err = os.MkdirAll(filepath.Dir(c.buildLocalPath(r.URL)), 0755)
	if err != nil {
		log.Printf("Error creating cache directory: %v\n", err)
		return
	}

	// Write the file to the cache
	file, err := os.Create(c.buildLocalPath(r.URL))
	if err != nil {
		log.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	// Write the file to the cache
	bw, err := io.Copy(file, &buf)
	if err != nil {
		log.Printf("Error writing file: %v\n", err)
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

	// Update the access cache with the new file
	c.accessCache.Set(r.URL.Host, r.URL.Path, AccessEntry{
		RemoteLastModified: lastModifiedTime,
		LastAccessed:       time.Now(),
		LastChecked:        time.Now(),
		ETag:               resp.Header.Get("ETag"),
		URL:                r.URL.String(),
		Size:               bw,
	})

	log.Printf("[INFO:DL:CREATED] %s%s - Wrote %d bytes\n", r.URL.Host, r.URL.Path, bw)
	go c.accessCache.TrackRequest(false, bw)
}
