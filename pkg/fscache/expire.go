package fscache

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
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

// DeleteUnreferencedFiles deletes all files that are not referenced in the database.
func (c *FSCache) DeleteUnreferencedFiles() error {
	err := c.deleteUnreferencedFilesByDatabase()
	if err != nil {
		return err
	}

	return nil
}

// deleteUnreferencedFilesByDatabase deletes all files that are found in the
// database but not on the filesystem.
func (c *FSCache) deleteUnreferencedFilesByDatabase() error {
	rows, err := c.accessCache.db.Query("SELECT domain, path, url, size FROM access_cache")
	if err != nil {
		return err
	}
	defer rows.Close()

	files := []url.URL{}
	sizeTotal := uint64(0)
	for rows.Next() {
		var domain, path, rurl string
		var size uint64
		err = rows.Scan(&domain, &path, &rurl, &size)
		if err != nil {
			return err
		}

		parsedURL, err := url.Parse(rurl)
		if err != nil {
			continue
		}

		if _, err := os.Stat(c.CachePath + "/" + domain + "/" + path); os.IsNotExist(err) {
			files = append(files, *parsedURL)
			sizeTotal += uint64(size)
		}
	}

	log.Printf("[INFO:EXPIRE] Found %d files that are not on the filesystem but in the database. Total size: %d bytes\n", len(files), sizeTotal)

	return nil
}

// deleteUnreferencedFilesByFilesystem deletes all files that are found on the
// filesystem but not in the database.
func (c *FSCache) deleteUnreferencedFilesByFilesystem() error {
	// Get all files in the cache directory
	files, err := c.getFilesInCacheDirectory()
	if err != nil {
		return err
	}

	// Get all files in the database
	rows, err := c.accessCache.db.Query("SELECT domain, path, url, size FROM access_cache")
	if err != nil {
		return err
	}
	defer rows.Close()

	// Create a map of all files in the database
	filesInDatabase := map[string]bool{}
	for rows.Next() {
		var domain, path, rurl string
		err = rows.Scan(&domain, &path, &rurl)
		if err != nil {
			return err
		}

		filesInDatabase[domain+"/"+path] = true
	}

	// Delete all files that are not in the database
	for _, file := range files {
		if _, ok := filesInDatabase[file]; !ok {
			err := os.Remove(c.CachePath + "/" + file)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// getFilesInCacheDirectory returns all files in the cache directory.
func (c *FSCache) getFilesInCacheDirectory() ([]string, error) {
	files := []string{}

	err := filepath.Walk(c.CachePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}
