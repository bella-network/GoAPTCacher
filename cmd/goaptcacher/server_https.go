package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"
)

func ListenHTTPS() {
	tlsconfig := &tls.Config{
		GetCertificate:           intercept.ReturnCert,
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS12,
		MaxVersion:               tls.VersionTLS13,
	}

	// If config.ListenPortSecure is 0, start the server on port 8091
	if config.ListenPortSecure == 0 {
		config.ListenPortSecure = 8091
	}

	ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", config.ListenPortSecure), tlsconfig)
	if err != nil {
		log.Println(err)
		return
	}
	defer ln.Close()

	// HTTP handler
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.ListenPortSecure),
		Handler: http.HandlerFunc(handleRequest),

		ReadHeaderTimeout: 90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// start TLS server
	log.Printf("[INFO] Starting proxy server on port %d\n", config.ListenPortSecure)
	err = server.Serve(ln)
	if err != nil {
		log.Fatal("Web server (HTTPS): ", err)
	}
}
