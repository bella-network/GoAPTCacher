package fscache

import (
	"log"
	"net/url"
	"time"
)

// expireUnusedFiles deletes files that have not been accessed for a long time (configurable).
func (c *FSCache) expireUnusedFiles() {
	time.Sleep(time.Second * 5)

	for {
		log.Printf("[INFO:EXPIRE] Starting file expiration\n")

		// Get all files that have not been accessed for a long time
		files, err := c.GetUnusedFiles(c.expirationInDays)
		if err != nil {
			log.Printf("[ERROR:EXPIRE] %s\n", err)
			return
		}

		// Delete all files that have not been accessed for a long time
		for _, file := range files {
			c.DeleteFile(&file)
		}

		// Sleep for a day
		time.Sleep(time.Hour * 24)
	}
}

// GetUnusedFiles returns all files that have not been accessed for a given period of time.
func (c *FSCache) GetUnusedFiles(days uint64) ([]url.URL, error) {
	if days == 0 {
		return nil, nil
	}

	rows, err := c.accessCache.db.Query("SELECT domain, path, url, size FROM access_cache WHERE last_accessed < ?", time.Now().Add(-time.Hour*24*time.Duration(days)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := []url.URL{}
	sizeTotal := uint64(0)
	for rows.Next() {
		var domain, path, rurl string
		var size uint64
		err = rows.Scan(&domain, &path, &rurl, &size)
		if err != nil {
			return nil, err
		}

		parsedURL, err := url.Parse(rurl)
		if err != nil {
			return nil, err
		}

		files = append(files, *parsedURL)
		sizeTotal += uint64(size)
	}

	log.Printf("[INFO:EXPIRE] Found %d files that have not been accessed for %d days. Total size: %d bytes\n", len(files), days, sizeTotal)

	return files, nil
}
