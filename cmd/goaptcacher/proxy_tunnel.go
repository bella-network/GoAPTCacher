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

// handleTUNNEL tunnels the request to the target host without any caching or
// interception. This is used for CONNECT requests and passthrough domains.
func handleTUNNEL(w http.ResponseWriter, r *http.Request) {
	log.Printf("[INFO:TUNNEL:%s] Tunneling request to %s\n", r.RemoteAddr, r.Host)

	// Connect to the target host
	destConn, err := net.DialTimeout("tcp", r.Host, 5*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer destConn.Close()

	// Send a 200 OK response to the client, indicating that the tunnel is
	// established. The client will then start sending data to the target host.
	w.WriteHeader(http.StatusOK)

	// Hijack the connection to the client so we can read/write data directly
	// from/to the client. This allows us to tunnel data between the client and
	// the target host.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}
	srcConn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer srcConn.Close()

	srcConnStr := fmt.Sprintf("%s->%s", srcConn.LocalAddr().String(), srcConn.RemoteAddr().String())
	dstConnStr := fmt.Sprintf("%s->%s", destConn.LocalAddr().String(), destConn.RemoteAddr().String())

	var wg sync.WaitGroup

	wg.Add(2)
	go transfer(&wg, destConn, srcConn, dstConnStr, srcConnStr)
	go transfer(&wg, srcConn, destConn, srcConnStr, dstConnStr)
	wg.Wait()
}

// transfer copies data from source to destination and logs any errors that
// occur. It is used to tunnel data between the client and the target host.
func transfer(wg *sync.WaitGroup, destination io.Writer, source io.Reader, destName, srcName string) {
	defer wg.Done()
	_, err := io.Copy(destination, source)
	if err != nil {
		// Ignore broken pipe errors
		if netErr, ok := err.(*net.OpError); ok && netErr.Err.Error() == "write: broken pipe" {
			log.Printf("[INFO:TUNNEL] Connection closed: %s -> %s\n", srcName, destName)
		} else {
			fmt.Printf("[ERR:TUNNEL] Error during copy from %s to %s: %v\n", srcName, destName, err)
		}
	}
}
