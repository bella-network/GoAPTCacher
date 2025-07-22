package main

import (
	"log"
	"os"
	"time"

	"gitlab.com/bella.network/goaptcacher/lib/dbc"
	"gitlab.com/bella.network/goaptcacher/pkg/fscache"
	"gitlab.com/bella.network/goaptcacher/pkg/httpsintercept"
	"gitlab.com/bella.network/goaptcacher/pkg/odb"
)

var config *Config                      // Config struct holding the configuration values
var loadedDomains uint64                // Number of loaded domains
var cache *fscache.FSCache              // Cache object used to store cached files
var intercept *httpsintercept.Intercept // Intercept object used to handle HTTPS interception
var db *odb.DBConnection                // Database connection object used to access the database

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
		configPath = "./config.yaml"
	}

	// Read the config file
	var err error
	config, err = ReadConfig(configPath)
	if err != nil {
		log.Fatal("Error reading config file: ", err)
	}

	// Initialize the database connection
	db, err = odb.NewMySQL(odb.DatabaseOptions{
		Host:     config.Database.Hostname,
		Username: config.Database.Username,
		Password: config.Database.Password,
		Database: config.Database.Database,
		Port:     config.Database.Port,
	})
	if err != nil {
		log.Fatal("Error initializing database connection: ", err)
	}

	// Run database creation and migration
	err = dbc.CheckSchemaCreation(db)
	if err != nil {
		log.Fatal("Error checking database schema creation: ", err)
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

		// Adding domains to the database before first request is made to ensure
		// that the database is ready to handle requests.
		for _, domain := range config.Domains {
			err = dbc.AddDomain(db, domain)
			if err != nil {
				log.Println("[WARN] Error adding domain to database: ", err)
			}
		}
		for _, domain := range config.PassthroughDomains {
			err = dbc.AddDomain(db, domain)
			if err != nil {
				log.Println("[WARN] Error adding passthrough domain to database: ", err)
			}
		}
	}

	// Show warning if index page is not enabled and show hint how to use the proxy
	if !config.Index.Enable {
		log.Printf("[INFO] Index page is disabled. Use this servers IP address or hostname to access the proxy server.")
	} else {
		if len(config.Index.Hostnames) == 0 {
			log.Printf("[WARN] Index page is enabled but no hostnames are configured. The index page will not be shown.")
		} else {
			log.Printf("[INFO] Index page is enabled. Access the proxy server using the following hostnames: %v", config.Index.Hostnames)
		}
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

		// Initialize the HTTPS interception handler
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

		// Set domain for certificate if configured
		if len(config.Domains) > 0 {
			intercept.SetDomain(config.Domains[0])
		}

		// Run periodic cleanup of expired certificates
		go func() {
			for {
				time.Sleep(time.Minute * 5)
				intercept.GC()
			}
		}()
	}

	// Initiate cache
	cache = fscache.NewFSCache(config.CacheDirectory, db.GetDB())
	// Start periodic verification of cached packages
	//cache.StartSourcesVerification()

	// Set expiration days for the cache
	if config.Expiration.UnusedDays > 0 {
		cache.SetExpirationDays(config.Expiration.UnusedDays)
	} else {
		log.Println("[INFO] File expiration is disabled, old packages are not automatically deleted")
	}

	// If HTTPS interception is enabled, start the HTTPS listener
	if config.HTTPS.Intercept {
		go ListenHTTPS()
	}

	// Start the HTTP listener
	go ListenHTTP()

	// If mDNS is enabled, announce the service
	if config.MDNS {
		go mDNSAnnouncement()
	}

	// Wait forever
	select {}
}
