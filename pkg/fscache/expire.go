package fscache

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	files := []url.URL{}
	sizeTotal := uint64(0)
	entries, err := c.collectAccessCacheRecords()
	if err != nil {
		return nil, err
	}

	daysInt, err := strconv.Atoi(strconv.FormatUint(days, 10))
	if err != nil {
		daysInt = int(^uint(0) >> 1)
	}

	cutoff := time.Now().AddDate(0, 0, -daysInt)
	for _, record := range entries {
		entry := c.normalizeAccessEntry(record.protocol, record.domain, record.path, record.entry)
		if entry.LastAccessed.IsZero() {
			continue
		}
		if entry.LastAccessed.Before(cutoff) {
			if entry.URL != nil {
				files = append(files, *entry.URL)
			}
			if entry.Size > 0 {
				sizeTotal += uint64(entry.Size)
			}
		}
	}

	log.Printf("[INFO:EXPIRE] Found %d files that have not been accessed for %d days. Total size: %d bytes\n", len(files), days, sizeTotal)

	return files, nil
}

// DeleteUnreferencedFiles deletes all files that are not referenced in the cache metadata.
func (c *FSCache) DeleteUnreferencedFiles() error {
	err := c.deleteUnreferencedFilesByMetadata()
	if err != nil {
		return err
	}

	return nil
}

// deleteUnreferencedFilesByMetadata deletes all files that are found in the
// cache metadata but not on the filesystem.
func (c *FSCache) deleteUnreferencedFilesByMetadata() error {
	files := []url.URL{}
	sizeTotal := uint64(0)
	entries, err := c.collectAccessCacheRecords()
	if err != nil {
		return err
	}

	for _, record := range entries {
		entry := c.normalizeAccessEntry(record.protocol, record.domain, record.path, record.entry)
		localPath := c.buildLocalPath(entry.URL)
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			if entry.URL != nil {
				files = append(files, *entry.URL)
			}
			if entry.Size > 0 {
				sizeTotal += uint64(entry.Size)
			}
		}
	}

	log.Printf("[INFO:EXPIRE] Found %d files that are not on the filesystem but in the cache metadata. Total size: %d bytes\n", len(files), sizeTotal)

	return nil
}

// deleteUnreferencedFilesByFilesystem deletes all files that are found on the
// filesystem but not in the cache metadata.
func (c *FSCache) deleteUnreferencedFilesByFilesystem() error {
	// Get all files in the cache directory
	files, err := c.getFilesInCacheDirectory()
	if err != nil {
		return err
	}

	// Create a map of all files found in cache metadata.
	filesInMetadata := map[string]bool{}
	entries, err := c.collectAccessCacheRecords()
	if err != nil {
		return err
	}

	for _, record := range entries {
		entry := c.normalizeAccessEntry(record.protocol, record.domain, record.path, record.entry)
		localPath := c.buildLocalPath(entry.URL)
		rel, err := filepath.Rel(c.CachePath, localPath)
		if err != nil {
			continue
		}
		filesInMetadata[rel] = true
	}

	// Delete all files that are not in metadata.
	for _, file := range files {
		if _, ok := filesInMetadata[file]; !ok {
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
		if strings.HasSuffix(path, accessCacheMetaSuffix) {
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
