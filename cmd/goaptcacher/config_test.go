package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadConfig(t *testing.T) {
	// Create a temporary config file
	configContent := `
cache_directory: "/tmp/cache"
listen_port: 8080
listen_port_secure: 8443
index:
  enable: true
  hostnames:
    - "example.com"
  contact: "admin@example.com"
domains:
  - "example.com"
passthrough_domains:
  - "passthrough.com"
overrides:
  ubuntu_server: "http://ubuntu.example.com"
  debian_server: "http://debian.example.com"
remap:
  - from: "http://old.example.com"
    to: "http://new.example.com"
https:
  prevent: false
  intercept: true
  cert: "/path/to/cert.pem"
  key: "/path/to/key.pem"
  password: "password"
mdns: true
expiration:
  unused_days: 30
`
	tmpFile, err := os.CreateTemp("", "config.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Read the config file
	config, err := ReadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	// Validate the config values
	if config.CacheDirectory != "/tmp/cache" {
		t.Errorf("Expected CacheDirectory to be '/tmp/cache', got '%s'", config.CacheDirectory)
	}
	if config.ListenPort != 8080 {
		t.Errorf("Expected ListenPort to be 8080, got %d", config.ListenPort)
	}
	if config.ListenPortSecure != 8443 {
		t.Errorf("Expected ListenPortSecure to be 8443, got %d", config.ListenPortSecure)
	}
	if !config.Index.Enable {
		t.Errorf("Expected Index.Enable to be true, got false")
	}
	if len(config.Index.Hostnames) != 1 || config.Index.Hostnames[0] != "example.com" {
		t.Errorf("Expected Index.Hostnames to contain 'example.com', got %v", config.Index.Hostnames)
	}
	if config.Index.Contact != "admin@example.com" {
		t.Errorf("Expected Index.Contact to be 'admin@example.com', got '%s'", config.Index.Contact)
	}
	if len(config.Domains) != 1 || config.Domains[0] != "example.com" {
		t.Errorf("Expected Domains to contain 'example.com', got %v", config.Domains)
	}
	if len(config.PassthroughDomains) != 1 || config.PassthroughDomains[0] != "passthrough.com" {
		t.Errorf("Expected PassthroughDomains to contain 'passthrough.com', got %v", config.PassthroughDomains)
	}
	if config.Overrides.UbuntuServer != "http://ubuntu.example.com" {
		t.Errorf("Expected Overrides.UbuntuServer to be 'http://ubuntu.example.com', got '%s'", config.Overrides.UbuntuServer)
	}
	if config.Overrides.DebianServer != "http://debian.example.com" {
		t.Errorf("Expected Overrides.DebianServer to be 'http://debian.example.com', got '%s'", config.Overrides.DebianServer)
	}
	if len(config.Remap) != 1 || config.Remap[0].From != "http://old.example.com" || config.Remap[0].To != "http://new.example.com" {
		t.Errorf("Expected Remap to contain 'http://old.example.com' -> 'http://new.example.com', got %v", config.Remap)
	}
	if config.HTTPS.Prevent {
		t.Errorf("Expected HTTPS.Prevent to be false, got true")
	}
	if !config.HTTPS.Intercept {
		t.Errorf("Expected HTTPS.Intercept to be true, got false")
	}
	if config.HTTPS.CertificatePublicKey != "/path/to/cert.pem" {
		t.Errorf("Expected HTTPS.CertificatePublicKey to be '/path/to/cert.pem', got '%s'", config.HTTPS.CertificatePublicKey)
	}
	if config.HTTPS.CertificatePrivateKey != "/path/to/key.pem" {
		t.Errorf("Expected HTTPS.CertificatePrivateKey to be '/path/to/key.pem', got '%s'", config.HTTPS.CertificatePrivateKey)
	}
	if config.HTTPS.CertificatePassword != "password" {
		t.Errorf("Expected HTTPS.CertificatePassword to be 'password', got '%s'", config.HTTPS.CertificatePassword)
	}
	if !config.MDNS {
		t.Errorf("Expected mDNS to be true, got false")
	}
	if config.Expiration.UnusedDays != 30 {
		t.Errorf("Expected Expiration.UnusedDays to be 30, got %d", config.Expiration.UnusedDays)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	return path
}

func withTestConfig(t *testing.T, cfg *Config) {
	t.Helper()
	old := config
	config = cfg
	t.Cleanup(func() {
		config = old
	})
}

func TestReadConfigAppliesDefaultsAndDebugDefaults(t *testing.T) {
	path := writeTempConfig(t, `
debug:
  enable: true
  pprof:
    enable: true
`)

	cfg, err := ReadConfig(path)
	if err != nil {
		t.Fatalf("ReadConfig() returned error: %v", err)
	}

	if cfg.CacheDirectory != "./cache" {
		t.Fatalf("CacheDirectory = %q, want %q", cfg.CacheDirectory, "./cache")
	}
	if cfg.ListenPort != 8090 {
		t.Fatalf("ListenPort = %d, want %d", cfg.ListenPort, 8090)
	}
	if cfg.Debug.LogIntervalSeconds != 60 {
		t.Fatalf("Debug.LogIntervalSeconds = %d, want %d", cfg.Debug.LogIntervalSeconds, 60)
	}
	if cfg.Debug.Pprof.IntervalSeconds != 60 {
		t.Fatalf("Debug.Pprof.IntervalSeconds = %d, want %d", cfg.Debug.Pprof.IntervalSeconds, 60)
	}
	if cfg.Debug.Pprof.Directory != filepath.Join("./cache", "pprof") {
		t.Fatalf("Debug.Pprof.Directory = %q, want %q", cfg.Debug.Pprof.Directory, filepath.Join("./cache", "pprof"))
	}
}

func TestReadConfigCacheDirEnvironmentOverride(t *testing.T) {
	t.Setenv("CACHE_DIR", "/env/cache")

	path := writeTempConfig(t, `
cache_directory: "/from/config"
debug:
  enable: true
  pprof:
    enable: true
`)

	cfg, err := ReadConfig(path)
	if err != nil {
		t.Fatalf("ReadConfig() returned error: %v", err)
	}

	if cfg.CacheDirectory != "/env/cache" {
		t.Fatalf("CacheDirectory = %q, want %q", cfg.CacheDirectory, "/env/cache")
	}
	if cfg.Debug.Pprof.Directory != filepath.Join("/env/cache", "pprof") {
		t.Fatalf("Debug.Pprof.Directory = %q, want %q", cfg.Debug.Pprof.Directory, filepath.Join("/env/cache", "pprof"))
	}
}

func TestReadConfigInvalidYAML(t *testing.T) {
	path := writeTempConfig(t, "listen_port: [")

	if _, err := ReadConfig(path); err == nil {
		t.Fatalf("expected ReadConfig() to fail for invalid YAML")
	}
}

func TestPrettifyBytes(t *testing.T) {
	tcs := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.00 KiB"},
		{1536, "1.50 KiB"},
		{1024 * 1024, "1.00 MiB"},
	}

	for _, tc := range tcs {
		if got := prettifyBytes(tc.in); got != tc.want {
			t.Fatalf("prettifyBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSafePercent(t *testing.T) {
	if got := safePercent(10, 0); got != 0 {
		t.Fatalf("safePercent(10, 0) = %d, want 0", got)
	}
	if got := safePercent(25, 200); got != 12 {
		t.Fatalf("safePercent(25, 200) = %d, want 12", got)
	}
	if got := safePercent(200, 100); got != 200 {
		t.Fatalf("safePercent(200, 100) = %d, want 200", got)
	}
}

func TestFormatBytes(t *testing.T) {
	tcs := []struct {
		in   uint64
		want string
	}{
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0KiB"},
		{1536, "1.5KiB"},
		{1024 * 1024, "1.0MiB"},
	}

	for _, tc := range tcs {
		if got := formatBytes(tc.in); got != tc.want {
			t.Fatalf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsLocalRequest(t *testing.T) {
	tcs := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		{"ipv4 loopback", "127.0.0.1:12345", true},
		{"ipv6 loopback", "[::1]:12345", true},
		{"loopback without port", "127.0.0.1", true},
		{"public ip", "8.8.8.8:53", false},
		{"hostname", "localhost:80", false},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tc.remoteAddr}
			if got := isLocalRequest(r); got != tc.want {
				t.Fatalf("isLocalRequest(%q) = %v, want %v", tc.remoteAddr, got, tc.want)
			}
		})
	}
}

func TestCleanupOldProfilesRetainsNewest(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	create := func(name string, modTime time.Time) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatalf("failed to create %s: %v", name, err)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("failed to set mtime for %s: %v", name, err)
		}
	}

	create("old.pprof", now.Add(-3*time.Hour))
	create("mid.pprof", now.Add(-2*time.Hour))
	create("new.pprof", now.Add(-1*time.Hour))
	create("ignore.txt", now)

	if err := cleanupOldProfiles(dir, 2); err != nil {
		t.Fatalf("cleanupOldProfiles() returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "old.pprof")); !os.IsNotExist(err) {
		t.Fatalf("expected old.pprof to be deleted, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mid.pprof")); err != nil {
		t.Fatalf("expected mid.pprof to exist, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.pprof")); err != nil {
		t.Fatalf("expected new.pprof to exist, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ignore.txt")); err != nil {
		t.Fatalf("expected ignore.txt to remain, got err=%v", err)
	}
}

func TestHTTPServeCRL(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		withTestConfig(t, &Config{})

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example/_goaptcacher/revocation.crl", nil)
		httpServeCRL(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "crl.pem")
		if err := os.WriteFile(path, []byte("test-crl"), 0o644); err != nil {
			t.Fatalf("failed to write crl file: %v", err)
		}

		cfg := &Config{CacheDirectory: dir}
		cfg.HTTPS.EnableCRL = true
		withTestConfig(t, cfg)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example/_goaptcacher/revocation.crl", nil)
		httpServeCRL(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if rr.Body.String() != "test-crl" {
			t.Fatalf("body = %q, want %q", rr.Body.String(), "test-crl")
		}
	})
}

func TestHTTPServeCertificate(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		withTestConfig(t, &Config{})

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example/_goaptcacher/goaptcacher.crt", nil)
		httpServeCertificate(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		dir := t.TempDir()
		certPath := filepath.Join(dir, "ca.crt")
		if err := os.WriteFile(certPath, []byte("test-cert"), 0o644); err != nil {
			t.Fatalf("failed to write cert file: %v", err)
		}

		cfg := &Config{}
		cfg.HTTPS.Intercept = true
		cfg.HTTPS.CertificatePublicKey = certPath
		withTestConfig(t, cfg)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example/_goaptcacher/goaptcacher.crt", nil)
		httpServeCertificate(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if rr.Body.String() != "test-cert" {
			t.Fatalf("body = %q, want %q", rr.Body.String(), "test-cert")
		}
	})
}
