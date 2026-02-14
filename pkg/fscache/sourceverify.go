package fscache

import (
	"bufio"
	"compress/bzip2"
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

	"github.com/ulikunitz/xz"
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
	time.Sleep(time.Minute * 5)
	for {
		log.Printf("[INFO:VERIFY] Starting source verification")
		if err := c.verifySources(); err != nil {
			log.Printf("[ERROR:VERIFY] %v", err)
		} else {
			log.Printf("[INFO:VERIFY] Source verification completed successfully")
		}
		time.Sleep(12 * time.Hour)
	}
}

type verificationRecord struct {
	protocol int
	domain   string
	path     string
	entry    AccessEntry
}

type releaseReference struct {
	domain string
	url    string
}

// verifySources performs a single verification run.
func (c *FSCache) verifySources() error {
	entries, err := c.collectAccessCacheRecords()
	if err != nil {
		return err
	}

	// Normalize once so the remaining steps can be simple, focused passes.
	records := c.normalizeVerificationRecords(entries)
	releases := collectReleaseReferences(records)
	packageChecksums := c.collectPackageChecksums(releases)
	c.verifyDebEntries(records, packageChecksums)

	return nil
}

func (c *FSCache) normalizeVerificationRecords(records []accessCacheRecord) []verificationRecord {
	result := make([]verificationRecord, 0, len(records))
	for _, record := range records {
		normalized, ok := c.normalizeVerificationRecord(record)
		if !ok {
			continue
		}
		result = append(result, normalized)
	}
	return result
}

func (c *FSCache) normalizeVerificationRecord(record accessCacheRecord) (verificationRecord, bool) {
	// Keep path resolution compatible with the previous implementation.
	path := record.path
	if path == "" && record.entry.URL != nil {
		path = record.entry.URL.Path
	}

	entry := c.normalizeAccessEntry(record.protocol, record.domain, record.path, record.entry)
	if entry.URL == nil {
		return verificationRecord{}, false
	}

	domain := record.domain
	if domain == "" {
		domain = entry.URL.Host
	}

	return verificationRecord{
		protocol: record.protocol,
		domain:   domain,
		path:     path,
		entry:    entry,
	}, true
}

func collectReleaseReferences(records []verificationRecord) []releaseReference {
	releases := make([]releaseReference, 0)
	for _, record := range records {
		if !strings.HasSuffix(record.path, "/InRelease") {
			continue
		}
		releases = append(releases, releaseReference{
			domain: record.domain,
			url:    record.entry.URL.String(),
		})
	}
	return releases
}

func (c *FSCache) collectPackageChecksums(releases []releaseReference) map[string]string {
	checksums := make(map[string]string)
	for _, release := range releases {
		c.collectReleasePackageChecksums(release, checksums)
	}
	return checksums
}

func (c *FSCache) collectReleasePackageChecksums(release releaseReference, checksums map[string]string) {
	sums, err := fetchReleaseSHA256(c.client, release.url)
	if err != nil {
		log.Printf("[WARN:VERIFY] failed to fetch release %s: %v", release.url, err)
		return
	}

	releaseBase := strings.TrimSuffix(release.url, "InRelease")
	packagesRootPath, err := resolvePackagesRootPath(releaseBase)
	if err != nil {
		log.Printf("[WARN:VERIFY] failed to parse base URL %s: %v", releaseBase, err)
		return
	}

	for _, sum := range sums {
		if !isPackagesIndexFile(sum.file) {
			continue
		}

		packagesURL := releaseBase + sum.file
		packages, err := fetchPackagesIndex(c.client, packagesURL)
		if err != nil {
			log.Printf("[WARN:VERIFY] failed to fetch packages %s: %v", packagesURL, err)
			continue
		}

		for packagePath, packageHash := range packages {
			checksums[release.domain+packagesRootPath+packagePath] = packageHash
		}
	}
}

func resolvePackagesRootPath(releaseBase string) (string, error) {
	baseURL, err := url.Parse(releaseBase + "../../")
	if err != nil {
		return "", err
	}

	// Normalize `../../` segments so package paths match cached deb paths.
	resolvedBaseURL := baseURL.ResolveReference(&url.URL{Path: baseURL.Path})
	return resolvedBaseURL.Path, nil
}

func isPackagesIndexFile(file string) bool {
	return strings.HasSuffix(file, "Packages") ||
		strings.HasSuffix(file, "Packages.gz") ||
		strings.HasSuffix(file, "Packages.xz") ||
		strings.HasSuffix(file, "Packages.bz2")
}

func (c *FSCache) verifyDebEntries(records []verificationRecord, packageChecksums map[string]string) {
	for _, record := range records {
		if !strings.HasSuffix(record.path, ".deb") {
			continue
		}
		c.verifyDebEntry(record, packageChecksums)
	}
}

func (c *FSCache) verifyDebEntry(record verificationRecord, packageChecksums map[string]string) {
	expectedChecksum, found := packageChecksums[record.domain+record.path]
	if !found {
		log.Printf("[INFO:VERIFY] %s%s not found in packages index, marking for deletion", record.domain, record.path)
		c.MarkForDeletion(record.protocol, record.domain, record.path)
		return
	}

	localPath := c.buildLocalPath(record.entry.URL)
	actualChecksum, err := sha256File(localPath)
	if err != nil {
		return
	}

	if actualChecksum == expectedChecksum {
		return
	}

	log.Printf(
		"[INFO:VERIFY] %s%s checksum mismatch: expected %s, got %s, marking for deletion",
		record.domain,
		record.path,
		expectedChecksum,
		actualChecksum,
	)
	c.MarkForDeletion(record.protocol, record.domain, record.path)
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

	switch {
	case strings.HasSuffix(u, ".gz"):
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz

	case strings.HasSuffix(u, ".bz2"):
		bz2a := bzip2.NewReader(resp.Body)
		if bz2a == nil {
			return nil, errors.New("failed to create bzip2 reader")
		}
		reader = bz2a

	case strings.HasSuffix(u, ".xz"):
		xza, err := xz.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		reader = xza

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
