package fscache

import (
	"net/url"
	"os"
)

// DeleteFile deletes a file from the cache including references in the metadata.
func (c *FSCache) DeleteFile(file *url.URL) error {
	// Get the local path of the file
	localPath := c.buildLocalPath(file)

	// Delete the file
	err := os.Remove(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File does not exist, delete the database entry anyway
			c.Delete(DetermineProtocolFromURL(file), file.Host, file.Path)
			return nil
		}
		return err
	}

	// Delete the file from the database
	c.Delete(DetermineProtocolFromURL(file), file.Host, file.Path)

	return nil
}
