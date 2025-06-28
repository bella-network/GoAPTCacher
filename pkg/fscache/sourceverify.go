package fscache

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// StartSourcesVerification starts a background goroutine which
// periodically verifies cached .deb files against the repository
// metadata. Packages which are no longer referenced or have a
// mismatching checksum are marked for deletion.
func (c *FSCache) StartSourcesVerification() {
	go c.runSourcesVerification()
}

func (c *FSCache) runSourcesVerification() {
	// initial delay
	time.Sleep(time.Minute)
	for {
		if err := c.verifySources(); err != nil {
			log.Printf("[ERROR:VERIFY] %v", err)
		}
		time.Sleep(12 * time.Hour)
	}
}

// verifySources performs a single verification run.
func (c *FSCache) verifySources() error {
	db := c.accessCache.GetDatabaseConnection()
	rows, err := db.Query("SELECT domain, path, url FROM access_cache WHERE path LIKE '/dists/%/InRelease'")
	if err != nil {
		return err
	}
	defer rows.Close()

	type releaseEntry struct {
		domain string
		url    string
	}
	releases := []releaseEntry{}
	for rows.Next() {
		var domain, path, u string
		if err := rows.Scan(&domain, &path, &u); err == nil {
			releases = append(releases, releaseEntry{domain: domain, url: u})
		}
	}

	packagesChecksums := make(map[string]string)

	for _, r := range releases {
		sums, err := fetchReleaseSHA256(c.client, r.url)
		if err != nil {
			log.Printf("[WARN:VERIFY] failed to fetch release %s: %v", r.url, err)
			continue
		}
		base := strings.TrimSuffix(r.url, "InRelease")
		for _, sum := range sums {
			if strings.HasSuffix(sum.file, "Packages") || strings.HasSuffix(sum.file, "Packages.gz") {
				url := base + sum.file
				pkgs, err := fetchPackagesIndex(c.client, url)
				if err != nil {
					log.Printf("[WARN:VERIFY] failed to fetch packages %s: %v", url, err)
					continue
				}
				for p, h := range pkgs {
					packagesChecksums["/"+p] = h
				}
			}
		}
	}

	rows2, err := db.Query("SELECT domain, path FROM access_cache WHERE path LIKE '%.deb'")
	if err != nil {
		return err
	}
	defer rows2.Close()

	for rows2.Next() {
		var domain, path string
		if err := rows2.Scan(&domain, &path); err != nil {
			continue
		}
		expected, ok := packagesChecksums[strings.TrimPrefix(path, "/")] // database paths start with /
		if !ok {
			c.accessCache.MarkForDeletion(domain, path)
			continue
		}
		filePath := c.CachePath + "/" + domain + path
		h, err := sha256File(filePath)
		if err != nil {
			continue
		}
		if h != expected {
			c.accessCache.MarkForDeletion(domain, path)
		}
	}

	return nil
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// shaFile represents a file entry in the Release file.
type shaFile struct {
	file string
	hash string
}

// fetchReleaseSHA256 downloads and parses the SHA256 list from a Release/\nInRelease file.
func fetchReleaseSHA256(client *http.Client, u string) ([]shaFile, error) {
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseReleaseSHA256(string(data)), nil
}

func parseReleaseSHA256(data string) []shaFile {
	var in bool
	var result []shaFile
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "SHA256:") {
			in = true
			continue
		}
		if in {
			if line == "" || !strings.HasPrefix(line, " ") {
				in = false
				continue
			}
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) >= 3 {
				result = append(result, shaFile{file: fields[2], hash: fields[0]})
			}
		}
	}
	return result
}

// fetchPackagesIndex downloads and parses a Packages file and returns a
// map of package paths to their SHA256 checksum.
func fetchPackagesIndex(client *http.Client, u string) (map[string]string, error) {
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var reader io.Reader = resp.Body
	if strings.HasSuffix(u, ".gz") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	}
	return parsePackages(reader), nil
}

func parsePackages(r io.Reader) map[string]string {
	pkgSums := make(map[string]string)
	scanner := bufio.NewScanner(r)
	var filename, sha string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Filename:") {
			filename = strings.TrimSpace(strings.TrimPrefix(line, "Filename:"))
		} else if strings.HasPrefix(line, "SHA256:") {
			sha = strings.TrimSpace(strings.TrimPrefix(line, "SHA256:"))
		} else if line == "" {
			if filename != "" && sha != "" {
				pkgSums[filename] = sha
			}
			filename = ""
			sha = ""
		}
	}
	if filename != "" && sha != "" {
		pkgSums[filename] = sha
	}
	return pkgSums
}
