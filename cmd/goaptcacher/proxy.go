package main

import (
	"log"
	"net/http"
	"strings"
)

// handleRequest is the main handler function for incoming HTTP requests. It
// checks if the target host is in the whitelist of domains to cache and proxy
// and then forwards the request to the appropriate handler based on the HTTP
// method. If the target host is not allowed to be proxied, a 403 Forbidden
// status code is returned to the client.
func handleRequest(w http.ResponseWriter, r *http.Request) {
	// Check if target host is in whitelist of configured domains to cache and
	// proxy.
	var found bool
	for _, host := range config.Domains {
		if strings.HasSuffix(r.Host, host) || strings.HasSuffix(r.Host, host+":443") {
			found = true
			break
		}
	}

	// Check if target host is in whitelist of configured passthrough domains.
	// Domains in this list are allowed the same was as domains in the domains
	// list, but they are not cached.
	var passthrough bool
	for _, host := range config.PassthroughDomains {
		if strings.HasSuffix(r.Host, host) || strings.HasSuffix(r.Host, host+":443") {
			passthrough = true
			found = true
			break
		}
	}

	// If no domains are configured, allow all requests.
	if loadedDomains == 0 {
		found = true
	}

	// If r.URL.Scheme is empty, a direct request was made to the proxy server.
	// In this case, set the scheme based on the presence of a TLS connection.
	if r.URL.Scheme == "" {
		if r.TLS != nil {
			r.URL.Scheme = "https"
		} else {
			r.URL.Scheme = "http"
		}
	}

	// If the target host is not allowed to be proxied, return a 403 Forbidden
	// status code to the client.
	if !found {
		http.Error(w, "Forbidden", http.StatusForbidden)
		log.Printf("[INFO:403] Domain not allowed: %s\n", r.Host)

		return
	}

	// Handle the request based on the HTTP method.
	switch r.Method {
	case http.MethodConnect:
		// If HTTPS requests are not allowed, return a 403 Forbidden status
		// code to the client.
		if config.HTTPS.Prevent {
			http.Error(w, "Forbidden", http.StatusForbidden)
			log.Printf("[INFO:403:%s] HTTPS requests are not allowed: %s\n", r.RemoteAddr, r.Host)
			return
		}

		// If passthrough is enabled or HTTPS interception is disabled, tunnel
		// the request to the target host without any caching or interception.
		if passthrough || !config.HTTPS.Intercept {
			handleTunnel(w, r)
		} else {
			handleCONNECT(w, r)
		}
	case http.MethodGet:
		// If passthrough is enabled or no domains are configured, forward the
		// request to the target host without any caching or interception.
		if passthrough || loadedDomains == 0 {
			handleTunnel(w, r)
		} else {
			handleHTTP(w, r)
		}
	default:
		log.Printf("Unsupported method: %s\n", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
