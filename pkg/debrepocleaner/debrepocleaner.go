package debrepocleaner

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RepositoryCleanup struct {
	Path    string
	Distrib string

	Components    []string
	Architectures []string
	Date          time.Time
	ValidUntil    time.Time

	SHA256sums []SHA256Sum
}

type SHA256Sum struct {
	Hash string
	File string
	Size int64
}

// New creates a new RepositoryCleanup instance by reading the InRelease file
func New(path, distrib string) (*RepositoryCleanup, error) {
	cl := &RepositoryCleanup{
		Path:       path,
		Distrib:    distrib,
		SHA256sums: []SHA256Sum{},
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
	var isInSHA256Block bool
	sha256sums := []SHA256Sum{}

	for _, line := range strings.Split(string(contents), "\n") {
		if strings.HasPrefix(line, "SHA256:") {
			isInSHA256Block = true
			continue
		}

		// If we're in the SHA256 block, we can parse the lines. We must break
		// out of the block if we encounter an empty line or the line isn't
		// intended by a space character.
		if isInSHA256Block {
			if line == "" || !strings.HasPrefix(line, " ") {
				isInSHA256Block = false
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

				sha256sums = append(sha256sums, SHA256Sum{
					Hash: parts[0],
					Size: size,
					File: parts[2],
				})

				continue
			}
		}

		// Given line may be a key-value pair, so we can split it by the colon
		// character and take the first two elements as the key and the value.
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		switch strings.TrimSpace(parts[0]) {
		case "Components":
			cl.Components = strings.Split(strings.TrimSpace(parts[1]), " ")
		case "Architectures":
			cl.Architectures = strings.Split(strings.TrimSpace(parts[1]), " ")
		case "Date": // Format Sun, 13 Oct 2024 13:53:11 UTC
			date, err := time.Parse("Mon, 2 Jan 2006 15:04:05 UTC", strings.TrimSpace(parts[1]))
			if err != nil {
				return err
			}

			cl.Date = date
		case "Valid-Until": // Format Sun, 13 Oct 2024 13:53:11 UTC
			date, err := time.Parse("Mon, 2 Jan 2006 15:04:05 UTC", strings.TrimSpace(parts[1]))
			if err != nil {
				return err
			}

			cl.ValidUntil = date
		}
	}

	cl.SHA256sums = sha256sums

	return nil
}

// VerifySHA256sums verifies the SHA256 sums of the files in the repository.
// Missing files are skipped as they may not have been requested by the user
// yet.
// @return []string: A list of files with a mismatching SHA256 sum
// @return error: An error if one occurred
func (cl *RepositoryCleanup) VerifySHA256sums() ([]string, error) {
	var mismatches []string

	for _, sum := range cl.SHA256sums {
		path := filepath.Join(cl.Path, sum.File)
		// Check if the file exists
		if _, err := os.Stat(path); err != nil {
			continue
		}

		// Calculate the SHA256 sum of the file
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}

		if _, err := file.Seek(0, 0); err != nil {
			return nil, err
		}

		gen := sha256.New()
		if _, err := file.WriteTo(gen); err != nil {
			return nil, err
		}

		hash := string(gen.Sum(nil))

		// Compare the calculated hash with the one from the InRelease file
		if hash != sum.Hash {
			mismatches = append(mismatches, path)
		}
	}

	return mismatches, nil
}
