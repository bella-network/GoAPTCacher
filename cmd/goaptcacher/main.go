package main

import (
	"log"
	"os"
	"time"

	"gitlab.com/bella.network/goaptcacher/pkg/fscache"
	"gitlab.com/bella.network/goaptcacher/pkg/httpsintercept"
)

var config *Config                      // Config struct holding the configuration values
var loadedDomains uint64                // Number of loaded domains
var cache *fscache.FSCache              // Cache object used to store cached files
var intercept *httpsintercept.Intercept // Intercept object used to handle HTTPS interception

func main() {
	// Detect if the program is launched by systemd, in that case only print
	// reduced logs.
	if os.Getenv("INVOCATION_ID") != "" {
		log.SetFlags(0)
	} else {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	log.Println("[INFO] Starting goaptcacher")

	// Check if envorinment variable is set with the path to the config file
	configPath := os.Getenv("CONFIG")
	if configPath == "" {
		configPath = "../../config.yaml"
	}

	// Read the config file
	var err error
	config, err = ReadConfig(configPath)
	if err != nil {
		log.Fatal("Error reading config file: ", err)
	}

	// If no domains and passthrough domains are configured, log a warning that
	// all requests will be allowed.
	loadedDomains = uint64(len(config.Domains) + len(config.PassthroughDomains))
	if loadedDomains == 0 {
		log.Println("[WARN] No domains or passthrough domains are configured!")
		log.Println("[WARN] All HTTP requests will be passed through - THIS IS A SECURITY RISK!")
		log.Println("[WARN] Cache will be disabled!")
	} else {
		log.Printf("[INFO] Loaded %d domains and %d passthrough domains\n", len(config.Domains), len(config.PassthroughDomains))
	}

	// If HTTPS interception is enabled, load the certificate and key files.
	// Initialize the interception handler for future processing.
	if config.HTTPS.Intercept {
		// Load the certificate and key files
		privateKeyData, err := os.ReadFile(config.HTTPS.CertificatePrivateKey)
		if err != nil {
			log.Fatal("Error reading private key file: ", err)
		}
		publicKeyData, err := os.ReadFile(config.HTTPS.CertificatePublicKey)
		if err != nil {
			log.Fatal("Error reading public key file: ", err)
		}

		intercept, err = httpsintercept.New(
			publicKeyData,
			privateKeyData,
			config.HTTPS.CertificatePassword,
			nil,
		)
		if err != nil {
			log.Fatal("Error initializing HTTPS interception: ", err)
		}

		log.Println("[INFO] HTTPS interception enabled")

		// Run periodic cleanup of expired certificates
		go func() {
			for {
				time.Sleep(time.Minute * 5)
				intercept.GC()
			}
		}()
	}

	// Initiate cache
	cache = fscache.NewFSCache(config.CacheDirectory)

	go ListenHTTPS()
	go ListenHTTP()

	// Wait forever
	select {}
}
