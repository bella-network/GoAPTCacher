package fscache

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

// downloadFileSimple downloads a file from the internet and saves it to the
// local path. No checks will be done.
func (c *FSCache) downloadFileSimple(url string, localPath string) error {
	// Create a new request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// Add the user agent to the request
	req.Header.Add("User-Agent", "GoAptCacher/1.0 (+https://gitlab.com/bella.network/goaptcacher)")

	// Send the request
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file
	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write the file
	bw, err := io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(localPath)
		return err
	}

	if resp.ContentLength > 0 && resp.ContentLength != bw {
		os.Remove(localPath)
		return fmt.Errorf("download incomplete: expected %d bytes, got %d", resp.ContentLength, bw)
	}

	return nil
}
