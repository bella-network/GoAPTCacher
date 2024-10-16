package main

import (
	"log"
	"net/http"
	"strings"
)

func checkOverrides(r *http.Request) {
	// Check if the request URL matches any of the remap entries
	for _, remap := range config.Remap {
		if r.URL.Path == remap.From {
			log.Printf("[INFO:OVERRIDE] Remapping %s to %s\n", r.URL.Path, remap.To)
			r.URL.Path = remap.To
		}
	}

	// Check if Ubuntu Server override is set
	if config.Overrides.UbuntuServer != "" {
		// If destination host is *.archive.ubuntu.com or archive.ubuntu.com, remap to the configured server
		if strings.HasSuffix(r.Host, "archive.ubuntu.com") || strings.HasSuffix(r.Host, ".archive.ubuntu.com") && r.Host != config.Overrides.UbuntuServer {
			log.Printf("[INFO:OVERRIDE:UBUNTU] Overriding %s to %s\n", r.Host, config.Overrides.UbuntuServer)
			r.Host = config.Overrides.UbuntuServer
			r.URL.Host = config.Overrides.UbuntuServer
		}
	}

	// Check if Debian Server override is set
	if config.Overrides.DebianServer != "" {
		// If destination host is ftp.{country}.debian.org, remap to the configured server
		if (strings.HasSuffix(r.Host, "debian.org") && strings.HasPrefix(r.Host, "ftp.") || r.Host == "deb.debian.org") && r.Host != config.Overrides.DebianServer {
			log.Printf("[INFO:OVERRIDE:DEBIAN] Overriding %s to %s\n", r.Host, config.Overrides.DebianServer)
			r.Host = config.Overrides.DebianServer
			r.URL.Host = config.Overrides.DebianServer
		}
	}
}
