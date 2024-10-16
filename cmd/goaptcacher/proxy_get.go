package main

import "net/http"

// handleHTTP handles basic HTTP requests. Probably the most important function
// as most repositories are accessed over HTTP.
func handleHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if a override is set for the requested URL
	checkOverrides(r)

	// Perform the request and serve the response
	cache.ServeFromRequest(r, w)
}
