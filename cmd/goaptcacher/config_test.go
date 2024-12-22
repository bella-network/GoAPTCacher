package main

import (
	"os"
	"testing"
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
