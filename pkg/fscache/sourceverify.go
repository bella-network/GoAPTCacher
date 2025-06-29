package fscache

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
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
	time.Sleep(time.Second * 5)
	for {
		if err := c.verifySources(); err != nil {
			log.Printf("[ERROR:VERIFY] %v", err)
		} else {
			log.Printf("[INFO:VERIFY] Source verification completed successfully")
		}
		time.Sleep(12 * time.Hour)
	}
}

// verifySources performs a single verification run.
func (c *FSCache) verifySources() error {
	db := c.accessCache.GetDatabaseConnection()
	rows, err := db.Query("SELECT domain, path, url FROM access_cache WHERE path LIKE '%/InRelease'")
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
		domain := r.domain
		for _, sum := range sums {
			if strings.HasSuffix(sum.file, "Packages") || strings.HasSuffix(sum.file, "Packages.gz") {
				newUrl := base + sum.file
				pkgs, err := fetchPackagesIndex(c.client, newUrl)
				if err != nil {
					log.Printf("[WARN:VERIFY] failed to fetch packages %s: %v", newUrl, err)
					continue
				}

				base2, err := url.Parse(base + "../../")
				if err != nil {
					log.Printf("[WARN:VERIFY] failed to parse base URL %s: %v", base, err)
					continue
				}

				// Resolve the URL references to get the correct domain
				base3 := base2.ResolveReference(&url.URL{Path: base2.Path})

				for p, h := range pkgs {
					packagesChecksums[domain+base3.Path+p] = h
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
		expected, ok := packagesChecksums[domain+path]
		if !ok {
			log.Printf("[INFO:VERIFY] %s%s not found in packages index, marking for deletion", domain, path)
			c.accessCache.MarkForDeletion(domain, path)
			continue
		}
		filePath := c.CachePath + "/" + domain + path
		h, err := sha256File(filePath)
		if err != nil {
			continue
		}
		if h != expected {
			log.Printf("[INFO:VERIFY] %s%s checksum mismatch: expected %s, got %s, marking for deletion", domain, path, expected, h)
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

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to fetch release SHA256: " + resp.Status)
	}

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

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to fetch packages index: " + resp.Status)
	}

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
		if after, ok := strings.CutPrefix(line, "Filename:"); ok {
			filename = strings.TrimSpace(after)
		} else if after0, ok0 := strings.CutPrefix(line, "SHA256:"); ok0 {
			sha = strings.TrimSpace(after0)
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
