package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func ListenHTTP() {
	// Create a new HTTP server with the handleRequest function as the handler
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", config.ListenPort),
		Handler: http.HandlerFunc(handleRequest),

		ReadHeaderTimeout: 90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start the server and log any errors
	log.Printf("[INFO] Starting proxy server on port %d\n", config.ListenPort)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal("[ERR] Error starting proxy server: ", err)
	}
}
