package debrepocleaner

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestVerifySHA256sumsIncludesDebFilesFromMultiplePackageIndexes(t *testing.T) {
	repo := t.TempDir()

	goodDebRelativePath := "pool/main/g/good/good_1.0_amd64.deb"
	badDebRelativePath := "pool/main/b/bad/bad_1.0_all.deb"

	goodDebPath := filepath.Join(repo, filepath.FromSlash(goodDebRelativePath))
	badDebPath := filepath.Join(repo, filepath.FromSlash(badDebRelativePath))

	writeFile(t, goodDebPath, []byte("good deb"))
	writeFile(t, badDebPath, []byte("actual bad deb"))

	packagesPlainRelativePath := "main/binary-amd64/Packages"
	packagesPlainBody := []byte(
		"Package: good\n" +
			"Filename: " + goodDebRelativePath + "\n" +
			"SHA256: " + checksumHexSHA256([]byte("good deb")) + "\n\n",
	)
	writeFile(t, filepath.Join(repo, "dists", "stable", filepath.FromSlash(packagesPlainRelativePath)), packagesPlainBody)

	packagesGzipRelativePath := "main/binary-all/Packages.gz"
	packagesGzipBody := []byte(
		"Package: bad\n" +
			"Filename: " + badDebRelativePath + "\n" +
			"SHA256: " + checksumHexSHA256([]byte("expected bad deb")) + "\n\n",
	)
	packagesGzipBytes := gzipBytes(t, packagesGzipBody)
	writeFile(t, filepath.Join(repo, "dists", "stable", filepath.FromSlash(packagesGzipRelativePath)), packagesGzipBytes)

	inRelease := "Date: Sun, 13 Oct 2024 13:53:11 UTC\n" +
		"Valid-Until: Mon, 14 Oct 2024 13:53:11 UTC\n" +
		"SHA256:\n" +
		" " + checksumHexSHA256(packagesPlainBody) + " " + fileSize(packagesPlainBody) + " " + packagesPlainRelativePath + "\n" +
		" " + checksumHexSHA256(packagesGzipBytes) + " " + fileSize(packagesGzipBytes) + " " + packagesGzipRelativePath + "\n"
	writeFile(t, filepath.Join(repo, "dists", "stable", "InRelease"), []byte(inRelease))

	cleanup, err := New(repo, "stable")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	mismatches, err := cleanup.VerifyChecksums()
	if err != nil {
		t.Fatalf("VerifyChecksums() returned error: %v", err)
	}

	want := []string{badDebPath}
	if !reflect.DeepEqual(mismatches, want) {
		t.Fatalf("VerifyChecksums() = %v, want %v", mismatches, want)
	}
}

func TestVerifyChecksumsReportsIndexMismatchInsideDistPath(t *testing.T) {
	repo := t.TempDir()

	packagesRelativePath := "main/binary-amd64/Packages"
	packagesPath := filepath.Join(repo, "dists", "stable", filepath.FromSlash(packagesRelativePath))
	writeFile(t, packagesPath, []byte("Package: hello\n"))

	inRelease := "SHA256:\n" +
		" " + checksumHexSHA256([]byte("other content")) + " " + fileSize([]byte("Package: hello\n")) + " " + packagesRelativePath + "\n"
	writeFile(t, filepath.Join(repo, "dists", "stable", "InRelease"), []byte(inRelease))

	cleanup, err := New(repo, "stable")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	mismatches, err := cleanup.VerifyChecksums()
	if err != nil {
		t.Fatalf("VerifyChecksums() returned error: %v", err)
	}

	want := []string{packagesPath}
	if !reflect.DeepEqual(mismatches, want) {
		t.Fatalf("VerifyChecksums() = %v, want %v", mismatches, want)
	}
}

func TestVerifyChecksumsSupportsSHA512(t *testing.T) {
	repo := t.TempDir()

	debRelativePath := "pool/main/h/hello/hello_1.0_amd64.deb"
	debPath := filepath.Join(repo, filepath.FromSlash(debRelativePath))
	writeFile(t, debPath, []byte("actual deb content"))

	packagesRelativePath := "main/binary-amd64/Packages.gz"
	packagesBody := []byte(
		"Package: hello\n" +
			"Filename: " + debRelativePath + "\n" +
			"SHA512: " + checksumHexSHA512([]byte("expected deb content")) + "\n" +
			"SHA256: " + checksumHexSHA256([]byte("actual deb content")) + "\n\n",
	)
	packagesBytes := gzipBytes(t, packagesBody)
	writeFile(t, filepath.Join(repo, "dists", "stable", filepath.FromSlash(packagesRelativePath)), packagesBytes)

	inRelease := "SHA256:\n" +
		" " + checksumHexSHA256([]byte("different index bytes")) + " " + fileSize(packagesBytes) + " " + packagesRelativePath + "\n" +
		"SHA512:\n" +
		" " + checksumHexSHA512(packagesBytes) + " " + fileSize(packagesBytes) + " " + packagesRelativePath + "\n"
	writeFile(t, filepath.Join(repo, "dists", "stable", "InRelease"), []byte(inRelease))

	cleanup, err := New(repo, "stable")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	mismatches, err := cleanup.VerifyChecksums()
	if err != nil {
		t.Fatalf("VerifyChecksums() returned error: %v", err)
	}

	want := []string{debPath}
	if !reflect.DeepEqual(mismatches, want) {
		t.Fatalf("VerifyChecksums() = %v, want %v", mismatches, want)
	}
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned error: %v", path, err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", path, err)
	}
}

func gzipBytes(t *testing.T, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(content); err != nil {
		t.Fatalf("gzip writer Write() returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip writer Close() returned error: %v", err)
	}

	return buf.Bytes()
}

func fileSize(content []byte) string {
	return strconv.Itoa(len(content))
}

func checksumHexSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func checksumHexSHA512(content []byte) string {
	sum := sha512.Sum512(content)
	return hex.EncodeToString(sum[:])
}
