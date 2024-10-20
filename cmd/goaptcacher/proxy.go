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
// status code is returned to the client. If a client directly requests the
// proxy server e.g. by entering the IP or hostname of the proxy server in the
// browser, a overview page is shown.
func handleRequest(w http.ResponseWriter, r *http.Request) {
	// If path starts with /_goaptcacher, handle the request as an internal
	// request. This is used for the index page, overview/configuration page,
	// and cache management.
	if r.Method != http.MethodConnect && strings.HasPrefix(r.URL.Path, "/_goaptcacher/") {
		handleIndexRequests(w, r)
		return
	}

	// If "/" is requested, redirect to the index page.
	if r.Method != http.MethodConnect && r.URL.Path == "/" {
		http.Redirect(w, r, "/_goaptcacher/", http.StatusTemporaryRedirect)
		return
	}

	// Answer/skip some standard requests to the proxy server.
	if r.Method == http.MethodGet {
		switch r.URL.Path {
		case "/favicon.ico":
			// Serve a 404 page not providing a favicon.
			w.WriteHeader(http.StatusNotFound)
			return
		case "/robots.txt":
			// Forbid all robots from indexing the proxy server.
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
			return
		case "/_goaptcacher":
			// Redirect to the index page.
			http.Redirect(w, r, "/_goaptcacher/", http.StatusTemporaryRedirect)
			return
		}
	}

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
			handleTUNNEL(w, r)
		} else {
			handleCONNECT(w, r)
		}
	case http.MethodGet:
		// If passthrough is enabled or no domains are configured, forward the
		// request to the target host without any caching or interception.
		if passthrough || loadedDomains == 0 {
			handleTUNNEL(w, r)
		} else {
			handleHTTP(w, r)
		}
	default:
		log.Printf("Unsupported method: %s\n", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
