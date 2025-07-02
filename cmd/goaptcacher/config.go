package main

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	CacheDirectory   string `yaml:"cache_directory"`    // Directory where the cache files are stored
	ListenPort       int    `yaml:"listen_port"`        // Port on which the proxy server listens
	ListenPortSecure int    `yaml:"listen_port_secure"` // Port on which the proxy server listens for HTTPS requests

	Database struct {
		Hostname string `yaml:"hostname"` // Hostname of the database server
		Username string `yaml:"username"` // Username for the database
		Password string `yaml:"password"` // Password for the database
		Database string `yaml:"database"` // Name of the database to use
		Port     int    `yaml:"port"`     // Port of the database server
	} `yaml:"database"`

	Index struct {
		Enable    bool     `yaml:"enable"`    // Enable the overview page which is shown when accessing the proxy server directly. This also sets a AIA extension in the certificate.
		Hostnames []string `yaml:"hostnames"` // List of hostnames which should be used for configuration or for direct access to the overview page
		Contact   string   `yaml:"contact"`   // Contact information which is shown on the overview page (HTML is allowed)
	} `yaml:"index"`

	Domains            []string `yaml:"domains"`             // List of domains which are allowed to be cached and proxied
	PassthroughDomains []string `yaml:"passthrough_domains"` // List of domains which are allowed to be proxied without caching

	Overrides struct {
		UbuntuServer string `yaml:"ubuntu_server"` // Override the Ubuntu server URL and map all locations to this server
		DebianServer string `yaml:"debian_server"` // Override the Debian server URL and map all locations to this server
	} `yaml:"overrides"`

	Remap []struct {
		From string `yaml:"from"` // Remap the URL from this value
		To   string `yaml:"to"`   // Remap the URL to this value
	} `yaml:"remap"`

	HTTPS struct {
		Prevent   bool `yaml:"prevent"`   // Prevent HTTPS requests from being cached and proxied
		Intercept bool `yaml:"intercept"` // Enable HTTPS interception which allows the proxy to cache HTTPS requests

		CertificatePublicKey  string `yaml:"cert"`     // Path to the public key file of the Intermediate CA or Root CA
		CertificatePrivateKey string `yaml:"key"`      // Path to the private key file of the Intermediate CA or Root CA
		CertificatePassword   string `yaml:"password"` // Password for the private key file of the Intermediate CA or Root CA
		//CertificateChain 	 string `yaml:"certificate_chain"` // Path to the certificate chain file of the Intermediate CA (may only contain the Root CA certificate)
	} `yaml:"https"`

	MDNS bool `yaml:"mdns"` // Enable mDNS announcement for apt proxy auto-discovery

	Expiration struct {
		UnusedDays uint64 `yaml:"unused_days"` // Number of days after which unused cached files are deleted
	} `yaml:"expiration"`
}

func ReadConfig(path string) (*Config, error) {
	// Read the config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse the config file
	config := &Config{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	// Cache directory may be set by environment variable, e.g for Docker,
	// development, etc.
	if cacheDir := os.Getenv("CACHE_DIR"); cacheDir != "" {
		config.CacheDirectory = cacheDir
	}

	// Set default cache directory if not set
	if config.CacheDirectory == "" {
		config.CacheDirectory = "./cache"
	}

	// Set default listen port if not set
	if config.ListenPort == 0 {
		config.ListenPort = 8090
	}

	return config, nil
}
