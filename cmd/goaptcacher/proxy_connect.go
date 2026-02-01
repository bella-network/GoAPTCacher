package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// handleCONNECT handles HTTPS CONNECT requests of clients which want to fetch a
// repository over HTTPS. This function intercepts the HTTPS request, applies
// the same caching as handleHTTP and serves a self-signed certificate to the
// client. This allows the proxy to cache HTTPS requests.
func handleCONNECT(w http.ResponseWriter, r *http.Request) {

	// "Hijack" the client connection to get a TCP (or TLS) socket we can read
	// and write arbitrary data to/from.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		log.Println("webserver doesn't support hijacking")
		return
	}

	clientConn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println("http hijacking failed")
		return
	}

	// proxyReq.Host will hold the CONNECT target host, which will typically have
	// a port - e.g. example.org:443
	// To generate a fake certificate for example.org, we have to first split off
	// the host from the port.
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println("error splitting host/port:", err)
		return
	}

	// Get intercept certificate
	certBundle := intercept.GetCertificate(host)

	// Send an HTTP OK response back to the client; this initiates the CONNECT
	// tunnel. From this point on the client will assume it's connected directly
	// to the target.
	if _, err := clientConn.Write(proxyCONNECTStatus(200, "OK")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println("error writing status to client:", err)
		return
	}

	// Configure a new TLS server, pointing it at the client connection, using
	// our certificate. This server will now pretend being the target.
	tlsConfig := &tls.Config{
		PreferServerCipherSuites: true,
		CurvePreferences:         []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256},
		MinVersion:               tls.VersionTLS12,
		Certificates:             []tls.Certificate{*certBundle},
	}

	tlsConn := tls.Server(clientConn, tlsConfig)
	defer tlsConn.Close()

	// Versuche TLS-Handshake und erkenne Zertifikatsfehler
	if err := tlsConn.Handshake(); err != nil {
		if strings.Contains(err.Error(), "unknown certificate") || strings.Contains(err.Error(), "certificate") || strings.Contains(err.Error(), "alert") {
			log.Printf("[TLS-ALERT] Client %s has aborted the TLS-connection due to a certificate error: %v", r.RemoteAddr, err)
		} else {
			log.Printf("[TLS-ERROR] TLS-Handshake with client %s failed: %v", r.RemoteAddr, err)
		}
		return
	}

	// Create a buffered reader for the client connection; this is required to
	// use http package functions with this connection.
	connReader := bufio.NewReader(tlsConn)

	// Run the proxy in a loop until the client closes the connection.
	for {
		// Read an HTTP request from the client; the request is sent over TLS that
		// connReader is configured to serve. The read will run a TLS handshake in
		// the first invocation (we could also call tlsConn.Handshake explicitly
		// before the loop, but this isn't necessary).
		// Note that while the client believes it's talking across an encrypted
		// channel with the target, the proxy gets these requests in "plain text"
		// because of the MITM setup.
		incomingRequest, err := http.ReadRequest(connReader)
		if err == io.EOF {
			break
		} else if err != nil {
			if strings.Contains(err.Error(), "connection reset by") {
				break
			}
			log.Println("error reading request from client:", err)
			break
		}

		// Set missing fields in the request
		incomingRequest.URL.Scheme = "https"
		incomingRequest.URL.Host = host
		incomingRequest.Method = http.MethodGet
		incomingRequest.RemoteAddr = r.RemoteAddr
		incomingRequest.RequestURI = fmt.Sprintf("https://%s%s", host, incomingRequest.URL.Path)

		// Log the incoming request
		log.Printf("[CONNECT] %s %s from %s\n", incomingRequest.Method, incomingRequest.URL.String(), incomingRequest.RemoteAddr)

		writer := newConnectResponseWriter(tlsConn)
		// Handle the request
		handleRequest(writer, incomingRequest)

		if err := writer.Close(); err != nil {
			log.Println("error writing response back:", err)
		}

		// Close the connection if the client closed the connection
		if incomingRequest.Close || writer.closeAfter {
			break
		}
	}
}

// proxyCONNECTStatus returns a HTTP response for a CONNECT request, with the
// given status code and message.
func proxyCONNECTStatus(code int, message string) []byte {
	content := fmt.Sprintf("HTTP/1.1 %d %s\r\n\r\n", code, message)
	return []byte(content)
}

type connectResponseWriter struct {
	conn        net.Conn
	bw          *bufio.Writer
	header      http.Header
	wroteHeader bool
	status      int
	chunked     bool
	closeAfter  bool
}

func newConnectResponseWriter(conn net.Conn) *connectResponseWriter {
	return &connectResponseWriter{
		conn:   conn,
		bw:     bufio.NewWriterSize(conn, 32*1024),
		header: make(http.Header),
	}
}

func (w *connectResponseWriter) Header() http.Header {
	return w.header
}

func (w *connectResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	_ = w.writeHeader()
}

func (w *connectResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		if err := w.writeHeader(); err != nil {
			return 0, err
		}
	}

	if w.chunked {
		if len(p) == 0 {
			return 0, nil
		}
		if _, err := fmt.Fprintf(w.bw, "%x\r\n", len(p)); err != nil {
			return 0, err
		}
		if _, err := w.bw.Write(p); err != nil {
			return 0, err
		}
		if _, err := w.bw.WriteString("\r\n"); err != nil {
			return 0, err
		}
		return len(p), nil
	}

	return w.bw.Write(p)
}

func (w *connectResponseWriter) Flush() {
	if !w.wroteHeader {
		_ = w.writeHeader()
	}
	_ = w.bw.Flush()
}

func (w *connectResponseWriter) Close() error {
	if !w.wroteHeader {
		if err := w.writeHeader(); err != nil {
			return err
		}
	}
	if w.chunked {
		if _, err := w.bw.WriteString("0\r\n\r\n"); err != nil {
			return err
		}
	}
	return w.bw.Flush()
}

func (w *connectResponseWriter) writeHeader() error {
	if w.wroteHeader {
		return nil
	}

	status := w.status
	if status == 0 {
		status = http.StatusOK
	}

	if v := w.header.Get("Connection"); v != "" && strings.EqualFold(strings.TrimSpace(v), "close") {
		w.closeAfter = true
	}

	if w.header.Get("Content-Length") == "" {
		te := w.header.Get("Transfer-Encoding")
		if te == "" {
			if !statusNoBody(status) {
				w.chunked = true
				w.header.Set("Transfer-Encoding", "chunked")
			}
		} else if strings.Contains(strings.ToLower(te), "chunked") {
			w.chunked = true
		}
	}

	if w.chunked {
		w.header.Del("Content-Length")
	}

	if _, err := fmt.Fprintf(w.bw, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status)); err != nil {
		return err
	}
	for key, values := range w.header {
		for _, value := range values {
			if _, err := fmt.Fprintf(w.bw, "%s: %s\r\n", key, value); err != nil {
				return err
			}
		}
	}
	if _, err := w.bw.WriteString("\r\n"); err != nil {
		return err
	}

	w.wroteHeader = true
	return nil
}

func statusNoBody(status int) bool {
	if status >= 100 && status <= 199 {
		return true
	}
	return status == http.StatusNoContent || status == http.StatusNotModified
}
