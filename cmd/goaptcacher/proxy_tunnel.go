package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// handleTunnel tunnels the request to the target host without any caching or
// interception. This is used for CONNECT requests and passthrough domains.
func handleTunnel(w http.ResponseWriter, r *http.Request) {
	// If in r.Host the port is not specified, append the default HTTP port. Do
	// not simply check for ":" as a IPv6 address would also contain a colon.
	// TODO: Check if this is really necessary

	log.Printf("[INFO:TUNNEL:%s] Tunneling request to %s\n", r.RemoteAddr, r.Host)

	dest_conn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer dest_conn.Close()
	w.WriteHeader(http.StatusOK)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}
	src_conn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer src_conn.Close()

	srcConnStr := fmt.Sprintf("%s->%s", src_conn.LocalAddr().String(), src_conn.RemoteAddr().String())
	dstConnStr := fmt.Sprintf("%s->%s", dest_conn.LocalAddr().String(), dest_conn.RemoteAddr().String())

	var wg sync.WaitGroup

	wg.Add(2)
	go transfer(&wg, dest_conn, src_conn, dstConnStr, srcConnStr)
	go transfer(&wg, src_conn, dest_conn, srcConnStr, dstConnStr)
	wg.Wait()
}

// transfer copies data from source to destination and logs any errors that
// occur. It is used to tunnel data between the client and the target host.
func transfer(wg *sync.WaitGroup, destination io.Writer, source io.Reader, destName, srcName string) {
	defer wg.Done()
	_, err := io.Copy(destination, source)
	if err != nil {
		fmt.Printf("[ERR:TUNNEL] Error during copy from %s to %s: %v\n", srcName, destName, err)
	}
}
