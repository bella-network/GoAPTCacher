package main

import (
	"log"
	"net/http"
	"strings"
)

// checkOverrides checks if the request URL matches any of the remap entries and
// overrides the destination host if necessary.
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
		// The override destination may contain a path, so we need to split it
		overrideParts := strings.Split(config.Overrides.UbuntuServer, "/")
		overrideHost := overrideParts[0]
		overridePath := strings.Join(overrideParts[1:], "/")

		// If destination host is *.archive.ubuntu.com or archive.ubuntu.com, remap to the configured server
		if strings.HasSuffix(r.Host, "archive.ubuntu.com") || strings.HasSuffix(r.Host, ".archive.ubuntu.com") && r.Host != overrideHost {
			log.Printf("[INFO:OVERRIDE:UBUNTU] Overriding %s to %s\n", r.Host, config.Overrides.UbuntuServer)
			r.Host = overrideHost
			r.URL.Host = overrideHost

			// If the override path is set, append it to the request URL
			if overridePath != "" {
				r.URL.Path = overridePath + r.URL.Path
			}
		}
	}

	// Check if Debian Server override is set
	if config.Overrides.DebianServer != "" {
		// The override destination may contain a path, so we need to split it
		overrideParts := strings.Split(config.Overrides.DebianServer, "/")
		overrideHost := overrideParts[0]
		overridePath := strings.Join(overrideParts[1:], "/")

		// If destination host is ftp.{country}.debian.org, remap to the configured server
		if (strings.HasSuffix(r.Host, "debian.org") && strings.HasPrefix(r.Host, "ftp.")) && r.Host != overrideHost {
			log.Printf("[INFO:OVERRIDE:DEBIAN] Overriding %s to %s\n", r.Host, config.Overrides.DebianServer)
			r.Host = overrideHost
			r.URL.Host = overrideHost

			// If the override path is set, append it to the request URL
			if overridePath != "" {
				r.URL.Path = overridePath + r.URL.Path
			}
		}

		// The host deb.debian.org is a special case, as at this host all paths
		// are available. Remap some paths to another host.
		if r.Host == "deb.debian.org" {
			if strings.HasPrefix(r.URL.Path, "/debian/") {
				r.Host = overrideHost
				r.URL.Host = overrideHost

				log.Printf("[INFO:OVERRIDE:DEBIAN] Overriding %s to %s for path %s\n", r.Host, config.Overrides.DebianServer, r.URL.Path)
				if overridePath != "" {
					r.URL.Path = overridePath + r.URL.Path
				}
			} else if strings.HasPrefix(r.URL.Path, "/debian-security/") ||
				strings.HasPrefix(r.URL.Path, "/debian-security-debug/") ||
				strings.HasPrefix(r.URL.Path, "/debian-debug/") ||
				strings.HasPrefix(r.URL.Path, "/debian-ports/") {
				r.Host = "security.debian.org"
				r.URL.Host = "security.debian.org"

				log.Printf("[INFO:OVERRIDE:DEBIAN] Overriding %s to security.debian.org for path %s\n", r.Host, r.URL.Path)
			}
		}
	}
}
