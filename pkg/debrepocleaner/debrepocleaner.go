package debrepocleaner

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

type RepositoryCleanup struct {
	Path    string
	Distrib string

	Components    []string
	Architectures []string
	Date          time.Time
	ValidUntil    time.Time

	Checksums []ChecksumSum
}

type ChecksumAlgorithm string

const (
	ChecksumAlgorithmSHA512 ChecksumAlgorithm = "SHA512"
	ChecksumAlgorithmSHA256 ChecksumAlgorithm = "SHA256"
)

var preferredChecksumAlgorithms = []ChecksumAlgorithm{
	ChecksumAlgorithmSHA512,
	ChecksumAlgorithmSHA256,
}

type ChecksumSum struct {
	Algorithm ChecksumAlgorithm
	Hash      string
	File      string
	Size      int64
}

// New creates a new RepositoryCleanup instance by reading the InRelease file
func New(path, distrib string) (*RepositoryCleanup, error) {
	cl := &RepositoryCleanup{
		Path:      path,
		Distrib:   distrib,
		Checksums: []ChecksumSum{},
	}

	// Read the InRelease file
	if err := cl.readInRelease(); err != nil {
		return nil, err
	}

	return cl, nil
}

// readInRelease reads the InRelease file and parses it
func (cl *RepositoryCleanup) readInRelease() error {
	// Read the InRelease file and parse it
	contents, err := os.ReadFile(filepath.Join(cl.Path, "dists", cl.Distrib, "InRelease"))
	if err != nil {
		return err
	}

	// Parse the InRelease file by looping over the lines. As the file is
	// structured as key-value pairs and may contain multiple lines, we can't
	// use some sort of standard parsing or splitting.
	var checksumAlgorithm ChecksumAlgorithm
	checksums := []ChecksumSum{}

	for line := range strings.SplitSeq(string(contents), "\n") {
		if algorithm, ok := checksumAlgorithmFromBlockHeader(line); ok {
			checksumAlgorithm = algorithm
			continue
		}

		// If we're in a supported checksum block, we can parse the lines. We must break
		// out of the block if we encounter an empty line or the line isn't
		// intended by a space character.
		if checksumAlgorithm != "" {
			if line == "" || !strings.HasPrefix(line, " ") {
				checksumAlgorithm = ""
			} else {
				// A given line lookls like this:
				// " 0142c5460ea3908d1272a84b3fdb9d8fb8f3710e 4195 main/binary-amd64/Packages"
				// We can split the line by spaces and take the first two elements
				// as the hash and the size.
				// Remove multiple spaces from the line and trim the line.
				line = strings.Join(strings.Fields(line), " ")
				parts := strings.Split(strings.TrimSpace(line), " ")
				size, err := strconv.ParseInt(parts[1], 10, 64)
				if err != nil {
					return err
				}

				checksums = append(checksums, ChecksumSum{
					Algorithm: checksumAlgorithm,
					Hash:      parts[0],
					Size:      size,
					File:      parts[2],
				})

				continue
			}
		}

		// Given line may be a key-value pair, so we can split it by the colon
		// character and take the first two elements as the key and the value.
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		switch strings.TrimSpace(key) {
		case "Components":
			cl.Components = strings.Split(strings.TrimSpace(value), " ")
		case "Architectures":
			cl.Architectures = strings.Split(strings.TrimSpace(value), " ")
		case "Date": // Format Sun, 13 Oct 2024 13:53:11 UTC
			date, err := time.Parse("Mon, 2 Jan 2006 15:04:05 UTC", strings.TrimSpace(value))
			if err != nil {
				return err
			}

			cl.Date = date
		case "Valid-Until": // Format Sun, 13 Oct 2024 13:53:11 UTC
			date, err := time.Parse("Mon, 2 Jan 2006 15:04:05 UTC", strings.TrimSpace(value))
			if err != nil {
				return err
			}

			cl.ValidUntil = date
		}
	}

	cl.Checksums = checksums

	return nil
}

// VerifyChecksums verifies the supported checksums of the files in the repository.
// Missing files are skipped as they may not have been requested by the user
// yet.
// @return []string: A list of files with a mismatching checksum
// @return error: An error if one occurred
func (cl *RepositoryCleanup) VerifyChecksums() ([]string, error) {
	mismatches := make(map[string]struct{})
	packageChecksums := make(map[string]ChecksumValue)

	for _, sum := range selectPreferredRepositoryChecksums(cl.Checksums) {
		path := filepath.Join(cl.Path, "dists", cl.Distrib, filepath.FromSlash(sum.File))
		exists, err := fileExists(path)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}

		hash, err := generateChecksumHash(path, sum.Algorithm)
		if err != nil {
			return nil, err
		}

		if hash != sum.Hash {
			mismatches[path] = struct{}{}
			continue
		}

		if !isPackagesIndexFile(sum.File) {
			continue
		}

		packages, err := parsePackagesFile(path)
		if err != nil {
			return nil, err
		}

		for packageFile, packageHash := range packages {
			if !strings.HasSuffix(packageFile, ".deb") {
				continue
			}

			if currentHash, ok := packageChecksums[packageFile]; ok {
				if currentHash.Algorithm == packageHash.Algorithm && currentHash.Hash != packageHash.Hash {
					mismatches[filepath.Join(cl.Path, filepath.FromSlash(packageFile))] = struct{}{}
					continue
				}

				if checksumAlgorithmPriority(currentHash.Algorithm) <= checksumAlgorithmPriority(packageHash.Algorithm) {
					continue
				}
			}

			packageChecksums[packageFile] = packageHash
		}
	}

	for packageFile, expectedHash := range packageChecksums {
		path := filepath.Join(cl.Path, filepath.FromSlash(packageFile))
		exists, err := fileExists(path)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}

		hash, err := generateChecksumHash(path, expectedHash.Algorithm)
		if err != nil {
			return nil, err
		}

		if hash != expectedHash.Hash {
			mismatches[path] = struct{}{}
		}
	}

	result := make([]string, 0, len(mismatches))
	for mismatch := range mismatches {
		result = append(result, mismatch)
	}
	slices.Sort(result)

	return result, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func checksumAlgorithmFromBlockHeader(line string) (ChecksumAlgorithm, bool) {
	key := strings.TrimSuffix(strings.TrimSpace(line), ":")

	switch ChecksumAlgorithm(key) {
	case ChecksumAlgorithmSHA512:
		return ChecksumAlgorithmSHA512, true
	case ChecksumAlgorithmSHA256:
		return ChecksumAlgorithmSHA256, true
	default:
		return "", false
	}
}

func generateChecksumHash(path string, algorithm ChecksumAlgorithm) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hashGenerator, err := newHashGenerator(algorithm)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(hashGenerator, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hashGenerator.Sum(nil)), nil
}

func newHashGenerator(algorithm ChecksumAlgorithm) (hash.Hash, error) {
	switch algorithm {
	case ChecksumAlgorithmSHA512:
		return sha512.New(), nil
	case ChecksumAlgorithmSHA256:
		return sha256.New(), nil
	default:
		return nil, errors.New("unsupported checksum algorithm: " + string(algorithm))
	}
}

func checksumAlgorithmPriority(algorithm ChecksumAlgorithm) int {
	for idx, supported := range preferredChecksumAlgorithms {
		if supported == algorithm {
			return idx
		}
	}

	return len(preferredChecksumAlgorithms)
}

func selectPreferredRepositoryChecksums(checksums []ChecksumSum) []ChecksumSum {
	selectedByFile := make(map[string]ChecksumSum)

	for _, checksum := range checksums {
		current, exists := selectedByFile[checksum.File]
		if !exists || checksumAlgorithmPriority(checksum.Algorithm) < checksumAlgorithmPriority(current.Algorithm) {
			selectedByFile[checksum.File] = checksum
		}
	}

	selected := make([]ChecksumSum, 0, len(selectedByFile))
	for _, checksum := range selectedByFile {
		selected = append(selected, checksum)
	}

	slices.SortFunc(selected, func(a, b ChecksumSum) int {
		return strings.Compare(a.File, b.File)
	})

	return selected
}

func isPackagesIndexFile(path string) bool {
	return strings.HasSuffix(path, "Packages") ||
		strings.HasSuffix(path, "Packages.gz") ||
		strings.HasSuffix(path, "Packages.xz") ||
		strings.HasSuffix(path, "Packages.bz2")
}

func parsePackagesFile(path string) (map[string]ChecksumValue, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var reader io.Reader = file

	switch {
	case strings.HasSuffix(path, ".gz"):
		gz, err := gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	case strings.HasSuffix(path, ".bz2"):
		reader = bzip2.NewReader(file)
	case strings.HasSuffix(path, ".xz"):
		xzReader, err := xz.NewReader(file)
		if err != nil {
			return nil, err
		}
		reader = xzReader
	}

	return parsePackages(reader)
}

type ChecksumValue struct {
	Algorithm ChecksumAlgorithm
	Hash      string
}

func parsePackages(r io.Reader) (map[string]ChecksumValue, error) {
	packages := make(map[string]ChecksumValue)
	scanner := bufio.NewScanner(r)

	var filename string
	hashes := make(map[ChecksumAlgorithm]string)

	for scanner.Scan() {
		line := scanner.Text()

		if after, ok := strings.CutPrefix(line, "Filename:"); ok {
			filename = strings.TrimSpace(after)
			continue
		}

		if algorithm, value, ok := checksumAlgorithmFromPackageField(line); ok {
			hashes[algorithm] = value
			continue
		}

		if line != "" {
			continue
		}

		if filename != "" {
			if checksum, ok := selectPreferredChecksumValue(hashes); ok {
				packages[filename] = checksum
			}
		}
		filename = ""
		clear(hashes)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if filename != "" {
		if checksum, ok := selectPreferredChecksumValue(hashes); ok {
			packages[filename] = checksum
		}
	}

	return packages, nil
}

func checksumAlgorithmFromPackageField(line string) (ChecksumAlgorithm, string, bool) {
	for _, algorithm := range preferredChecksumAlgorithms {
		if after, ok := strings.CutPrefix(line, string(algorithm)+":"); ok {
			return algorithm, strings.TrimSpace(after), true
		}
	}

	return "", "", false
}

func selectPreferredChecksumValue(hashes map[ChecksumAlgorithm]string) (ChecksumValue, bool) {
	for _, algorithm := range preferredChecksumAlgorithms {
		if hash, ok := hashes[algorithm]; ok && hash != "" {
			return ChecksumValue{
				Algorithm: algorithm,
				Hash:      hash,
			}, true
		}
	}

	return ChecksumValue{}, false
}
