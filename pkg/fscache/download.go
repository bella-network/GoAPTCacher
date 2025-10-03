package fscache

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"gitlab.com/bella.network/goaptcacher/pkg/buildinfo"
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
	req.Header.Add("User-Agent", fmt.Sprintf("GoAptCacher/%s (+https://gitlab.com/bella.network/goaptcacher)", buildinfo.Version))

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
