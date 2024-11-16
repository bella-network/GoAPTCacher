package main

import (
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	web "gitlab.com/bella.network/goaptcacher/lib/web"
)

// handleIndexRequests is the handler function for requests to the index page of
// the proxy server. It serves a simple interface with a description of the
// proxy server and its purpose. In addition, additional functionality like
// cache management and configuration is available.
func handleIndexRequests(w http.ResponseWriter, r *http.Request) {
	// Path /_goaptcacher is always present, so we strip it for a more simple
	// handling of incoming requests.
	requestedPath := r.URL.Path
	if strings.HasPrefix(r.URL.Path, "/_goaptcacher") {
		requestedPath = strings.TrimPrefix(r.URL.Path, "/_goaptcacher")
	}

	// Set some default headers for the response. This is required to prevent
	// browsers from caching the response and to secure the server.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	// Based on the requested path, serve the appropriate page.
	switch requestedPath {
	case "/style.css", "style.css":
		w.Header().Set("Content-Type", "text/css")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(web.Style)
	case "/", "":
		httpServeSubpage(w, r, "index")
	case "/cache":
		httpServeSubpage(w, r, "cache")
	case "/stats":
		httpServeSubpage(w, r, "stats")
	case "/setup":
		httpServeSubpage(w, r, "setup")
	default:
		// Serve a 404 page
		w.WriteHeader(http.StatusNotFound)
		httpServeSubpage(w, r, "404")
	}

	log.Printf("[INFO:WEB] Requested path: %s\n", requestedPath)
}

// helperHTTPTemplateVars is a helper function that returns the template
// variables for the main page template.
func helperHTTPConstants() map[string]any {
	return map[string]any{
		"ListenPort":       config.ListenPort,
		"ListenPortSecure": config.ListenPortSecure,
		"Domains":          config.Domains,
		"Version":          "0.0.1",
	}
}

// httpServeSubpage is a helper function that serves a subpage of the main page
// template.
func httpServeSubpage(w http.ResponseWriter, r *http.Request, subpage string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// pageContent contains the main content of the requested page.
	var pageContent string
	var title string

	switch subpage {
	case "index":
		// Index only contains a main description of this proxy server without
		// any additional functionality.
		pageContent = `<h2>Welcome to GoAPTCacher</h2>
		<p>
			GoAPTCacher is a simple caching proxy server for APT based repositories.
			It caches packages and metadata from upstream repositories to speed up package installations and updates on multiple systems.<br>
			<br>
			This page is shown to you because you have accessed the proxy server directly or with an invalid configuration.<br>
			<br>
			If you want to use this proxy server, please configure your APT client to use this server as a proxy. For more information, please check the <a href="/_goaptcacher/setup">setup page</a>.<br>
			<br>
			Detailed cache statistics and management are available on the <a href="/_goaptcacher/stats">stats page</a>.<br>
			<br>
			For more information about this project, please visit the <a href="https://gitlab.com/bella.network/goaptcacher">GitLab repository</a>.
		</p>`
		title = "GoAPTCacher"
	case "stats":
		// Stats page contains the cache statistics of this proxy server.
		pageContent = httpPageStats()
		title = "Cache statistics - GoAPTCacher"
	case "setup":
		// Setup page contains the configuration of this proxy server.
		pageContent = httpPageSetup()
		title = "Setup - GoAPTCacher"
	case "404":
		// 404 page is shown if the requested page does not exist.
		pageContent = `
<h2>Page not found</h2>
<p>
	The requested page you are looking for does not exist on this server. <br>
	Please check the URL and try again.
</p>
`
		title = "Page not found - GoAPTCacher"
	}

	// Execute the template with the main page content and the template
	// variables.
	temp, err := web.GetTemplate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = temp.Execute(w, map[string]interface{}{
		"Title":   title,
		"Content": pageContent,
		"Const":   helperHTTPConstants(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// httpPageStats returns the page content for the stats page containing the
// cache statistics of this proxy server.
func httpPageStats() string {
	db := cache.GetDatabaseConnection()

	// Query the database for some statistics
	var filesCached, totalSize uint64
	err := db.QueryRow("SELECT COUNT(*), SUM(size) FROM access_cache").Scan(&filesCached, &totalSize)
	if err != nil {
		log.Printf("[ERROR:WEB] Error querying database: %s\n", err)
	}

	// Get total cache statistics
	var totalRequests, totalHits, totalMisses, totalTrafficUp, totalTrafficDown uint64
	err = db.QueryRow("SELECT SUM(requests), SUM(hits), SUM(misses), SUM(traffic_down), SUM(traffic_up) FROM stats").Scan(&totalRequests, &totalHits, &totalMisses, &totalTrafficDown, &totalTrafficUp)
	if err != nil {
		log.Printf("[ERROR:WEB] Error querying database: %s\n", err)
	}

	// From stats, get last 14 days of traffic
	type statsEntry struct {
		Date        string
		Requests    uint64
		Hits        uint64
		Misses      uint64
		TrafficUp   uint64
		TrafficDown uint64
	}
	var entryList []statsEntry
	rows, err := db.Query("SELECT date, requests, hits, misses, traffic_down, traffic_up FROM stats ORDER BY date DESC LIMIT 14")
	if err != nil {
		log.Printf("[ERROR:WEB] Error querying database: %s\n", err)
	}
	defer rows.Close()

	for rows.Next() {
		var entry statsEntry
		err = rows.Scan(&entry.Date, &entry.Requests, &entry.Hits, &entry.Misses, &entry.TrafficDown, &entry.TrafficUp)
		if err != nil {
			log.Printf("[ERROR:WEB] Error scanning database row: %s\n", err)
		}

		entryList = append(entryList, entry)
	}

	response := `<h2>Cache statistics</h2>
	<p>
		This page shows the cache statistics of this proxy server including the total number of cached files, the total number of requests, hits, misses, and the total traffic served to clients and total traffic fetched from the repository upstream servers.
		You can also see the last 14 days of traffic statistics in detail below.
	</p>
	<h3>Lifetime statistics</h3>
	<p><ul>`
	response += fmt.Sprintf("<li>Total files cached: %d</li>", filesCached)
	response += fmt.Sprintf("<li>Total size cached: %s</li>", prettifyBytes(totalSize))
	response += fmt.Sprintf("<li>Total requests: %d</li>", totalRequests)
	response += fmt.Sprintf("<li>Total hits: %d (%d%%)</li>", totalHits, 100*totalHits/totalRequests)
	response += fmt.Sprintf("<li>Total misses: %d (%d%%)</li>", totalMisses, 100*totalMisses/totalRequests)
	response += fmt.Sprintf("<li>Total traffic served to clients: %s</li>", prettifyBytes(totalTrafficUp))
	response += fmt.Sprintf("<li>Total traffic fetched from repo servers: %s</li>", prettifyBytes(totalTrafficDown))
	response += `</ul></p>
	<h3>Last 14 days statistics</h3>
	<p>
		<table>
			<tr>
				<th>Date</th>
				<th>Requests</th>
				<th>Hits</th>
				<th>Misses</th>
				<th>Traffic served</th>
				<th>Traffic fetched</th>
			</tr>`
	for _, entry := range entryList {
		response += fmt.Sprintf(
			"<tr><td>%s</td><td>%d</td><td>%d (%d%%)</td><td>%d (%d%%)</td><td>%s</td><td>%s (%d%%)</td></tr>",
			entry.Date,
			entry.Requests,
			entry.Hits,
			100*entry.Hits/entry.Requests,
			entry.Misses,
			100*entry.Misses/entry.Requests,
			prettifyBytes(entry.TrafficUp),
			prettifyBytes(entry.TrafficDown),
			100*entry.TrafficDown/entry.TrafficUp,
		)

	}
	response += `</table></p>`

	return response
}

// httpPageSetup returns the page content for the setup page containing the
// configuration of this proxy server.
func httpPageSetup() string {
	// Determine domain to be used for configuration directives
	var domain string
	if len(config.Index.Hostnames) > 0 {
		domain = config.Index.Hostnames[0]
	} else {
		// Detect IP address of this server and use it as domain
		ip, err := getLocalIP()
		if err != nil || ip == "" {
			log.Printf("[ERROR:WEB] Error getting local IP address: %s\n", err)
			domain = "127.0.0.1"
		} else {
			domain = ip
		}
	}

	httpPort := strconv.Itoa(config.ListenPort)
	httpsPort := strconv.Itoa(config.ListenPortSecure)

	var response string
	response += `<h2>Setup</h2>`
	response += `<p>
		This page shows the configuration of this proxy server. You can use this information to configure your APT client to use this proxy server.<br>
		For configuration, there are multiple options available. Please choose the one that fits your needs.<br>
	</p>`

	// Configuration: Proxy Servers (HTTP and HTTPS)
	response += `<h3>APT Proxy Directives</h3>`
	response += `<p>
		To use this proxy server with APT, you need to add the following lines to your APT configuration file (usually located at <code>/etc/apt/apt.conf.d/10proxy.conf</code>):<br>
		<pre>
Acquire::http::Proxy "http://<span style="color: #ff0000;">` + domain + `:` + httpPort + `</span>";
Acquire::https::Proxy "http://<span style="color: #ff0000;">` + domain + `:` + httpPort + `</span>";
		</pre>
	</p>
	`

	// Configuration: APT Proxy Server Discovery
	response += `<h3>APT Proxy Discovery</h3>`
	response += `<p>
		To use this proxy server with APT, you can also use the APT proxy discovery feature. This feature allows you to configure the proxy server once and let the APT client discover the proxy server automatically.<br>
		To use this feature, you need to install the auto-apt-proxy package using <code>apt install <strong>auto-apt-proxy</strong></code> on your system.<br>
		<br>
		Assuming your internal domain is <strong>example.com</strong>, add the follwing DNS SRV record to your DNS server:<br>
		<pre>
_apt_proxy._tcp.<span style="color: #ff0000;">example.com</span>. 3600 IN SRV 0 0 <span style="color: #ff0000;">` + httpPort + ` ` + domain + `</span>
		</pre>

		You can verify if auto-apt-proxy is detecting the proxy server by running <code>auto-apt-proxy -v</code> on your system.
	</p>
	`

	// Configuration: DNS SRV Record Override
	response += `<h3>DNS SRV Override</h3>`
	response += `<p>
		To use this proxy server with APT, you can also use the DNS SRV record override feature. This feature allows you to override the destination of the APT requests by using a DNS SRV record.<br>
		If the client does not support SRV record lookups, it will fall back to the default DNS resolution.<br>
		<br>
		For this feature to work, you need to configure your local DNS server to return the following SRV record for <strong>every single domain</strong> you want to use with this proxy server:<br>
		<pre>
_http._tcp.<span style="color: #ff0000;">at.archive.ubuntu.com</span>. 3600 IN SRV 0 0 <span style="color: #ff0000;">` + httpPort + ` ` + domain + `</span>
		</pre>

		For HTTPS destinations like <strong>download.docker.com</strong>, you can use the following SRV record:<br>
		<pre>
_https._tcp.<span style="color: #ff0000;">download.docker.com</span>. 3600 IN SRV 0 0 <span style="color: #ff0000;">` + httpsPort + ` ` + domain + `</span>
		</pre>
	</p>
	`

	return response
}

// prettifyBytes is a helper function that returns a human-readable string of
// the given bytes.
func prettifyBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// getLocalIP is a helper function that returns the local IP address of the
// server by checking all network interfaces. Loopback addresses are ignored.
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no IP address found")
}
