package fscache

import (
	"net/url"
	"os"
)

// DeleteFile deletes a file from the cache including references in the database.
func (c *FSCache) DeleteFile(file *url.URL) error {
	// Get the local path of the file
	localPath := c.buildLocalPath(file)

	// Delete the file
	err := os.Remove(localPath)
	if err != nil {
		return err
	}

	// Delete the file from the database
	c.accessCache.Delete(file.Host, file.Path)

	return nil
}
