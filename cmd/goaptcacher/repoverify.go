package main

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"slices"

	"gitlab.com/bella.network/goaptcacher/pkg/debrepocleaner"
)

type cachedRepository struct {
	rootPath string
	distrib  string
}

func runVerifyRepositories(cacheDirectory string) error {
	repositories, err := discoverCachedRepositories(cacheDirectory)
	if err != nil {
		return err
	}

	if len(repositories) == 0 {
		log.Printf("[DEBREPOCLEANER-INFO] No repositories with InRelease metadata found under %s", cacheDirectory)
		return nil
	}

	var failedRepositories int
	var mismatchingFiles int

	log.Printf("[DEBREPOCLEANER-INFO] Verifying %d cached repositories", len(repositories))

	for _, repository := range repositories {
		cleanup, err := debrepocleaner.New(repository.rootPath, repository.distrib)
		if err != nil {
			failedRepositories++
			log.Printf(
				"[DEBREPOCLEANER-WARN] Failed to initialize repository %s (%s): %v",
				repository.rootPath,
				repository.distrib,
				err,
			)
			continue
		}

		mismatches, err := cleanup.VerifyChecksums()
		if err != nil {
			failedRepositories++
			log.Printf(
				"[DEBREPOCLEANER-WARN] Failed to verify repository %s (%s): %v",
				repository.rootPath,
				repository.distrib,
				err,
			)
			continue
		}

		if len(mismatches) == 0 {
			log.Printf(
				"[DEBREPOCLEANER-INFO] Repository verified successfully: %s (%s)",
				repository.rootPath,
				repository.distrib,
			)
			continue
		}

		mismatchingFiles += len(mismatches)
		log.Printf(
			"[DEBREPOCLEANER-WARN] Repository has %d mismatching files: %s (%s)",
			len(mismatches),
			repository.rootPath,
			repository.distrib,
		)
		for _, mismatch := range mismatches {
			log.Printf("[DEBREPOCLEANER-WARN] mismatch: %s", mismatch)
		}
	}

	log.Printf(
		"[DEBREPOCLEANER-INFO] Repository verification finished: repositories=%d mismatches=%d failures=%d",
		len(repositories),
		mismatchingFiles,
		failedRepositories,
	)

	if failedRepositories > 0 {
		return fmt.Errorf("%d repositories could not be verified", failedRepositories)
	}

	return nil
}

func discoverCachedRepositories(cacheDirectory string) ([]cachedRepository, error) {
	seen := make(map[string]cachedRepository)

	err := filepath.WalkDir(cacheDirectory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "InRelease" {
			return nil
		}

		repository, ok := cachedRepositoryFromInReleasePath(path)
		if !ok {
			return nil
		}

		seen[repository.rootPath+"\x00"+repository.distrib] = repository
		return nil
	})
	if err != nil {
		return nil, err
	}

	repositories := make([]cachedRepository, 0, len(seen))
	for _, repository := range seen {
		repositories = append(repositories, repository)
	}

	slices.SortFunc(repositories, func(a, b cachedRepository) int {
		if a.rootPath == b.rootPath {
			return compareStrings(a.distrib, b.distrib)
		}
		return compareStrings(a.rootPath, b.rootPath)
	})

	return repositories, nil
}

func cachedRepositoryFromInReleasePath(path string) (cachedRepository, bool) {
	distribPath := filepath.Dir(path)
	distsPath := filepath.Dir(distribPath)
	if filepath.Base(distsPath) != "dists" {
		return cachedRepository{}, false
	}

	rootPath := filepath.Dir(distsPath)
	distrib := filepath.Base(distribPath)
	if rootPath == "." || rootPath == "" || distrib == "." || distrib == "" {
		return cachedRepository{}, false
	}

	return cachedRepository{
		rootPath: rootPath,
		distrib:  distrib,
	}, true
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
