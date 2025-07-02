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
		}

		// Delete all files that have not been accessed for a long time
		for _, file := range files {
			err := c.DeleteFile(&file)
			if err != nil {
				log.Printf("[ERROR:EXPIRE] %s\n", err)
			}
		}

		log.Printf("[INFO:EXPIRE] File expiration finished\n")

		// Sleep for a day
		time.Sleep(time.Hour * 12)
	}
}

// GetUnusedFiles returns all files that have not been accessed for a given period of time.
func (c *FSCache) GetUnusedFiles(days uint64) ([]url.URL, error) {
	if days == 0 {
		return nil, nil
	}

	rows, err := c.db.Query("SELECT d.protocol, d.domain, f.path, f.url, f.size FROM access_cache a JOIN files f ON a.file = f.id JOIN domains d ON f.domain = d.id WHERE a.last_access < NOW() - INTERVAL ? DAY", days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := []url.URL{}
	sizeTotal := uint64(0)
	for rows.Next() {
		var domain, path, rurl string
		var size uint64
		var protocol int
		err = rows.Scan(&protocol, &domain, &path, &rurl, &size)
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
	rows, err := c.db.Query("SELECT ...")
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
	rows, err := c.db.Query("SELECT d.protocol, d.domain, f.path, f.url FROM files f JOIN domains d ON f.domain = d.id")
	if err != nil {
		return err
	}
	defer rows.Close()

	// Create a map of all files in the database
	filesInDatabase := map[string]bool{}
	for rows.Next() {
		var domain, path, rurl string
		var protocol int
		err = rows.Scan(&protocol, &domain, &path, &rurl)
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

		rel, err := filepath.Rel(c.CachePath, path)
		if err != nil {
			return err
		}

		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}
