package main

import (
	"fmt"
	"html"
	htmltemplate "html/template"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	web "gitlab.com/bella.network/goaptcacher/lib/web"
	"gitlab.com/bella.network/goaptcacher/pkg/buildinfo"
)

const statsHistoryDays = 14

// handleIndexRequests is the handler function for requests to the index page of
// the proxy server. It serves a simple interface with a description of the
// proxy server and its purpose. In addition, additional functionality like
// cache management and configuration is available.
func handleIndexRequests(w http.ResponseWriter, r *http.Request) {
	// Path /_goaptcacher is always present, so we strip it for a more simple
	// handling of incoming requests.
	requestedPath := r.URL.Path
	if after, ok := strings.CutPrefix(r.URL.Path, "/_goaptcacher"); ok {
		requestedPath = after
	}

	// Set some default headers for the response. This is required to prevent
	// browsers from caching the response and to secure the server.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	if handleDebugRequests(w, r, requestedPath) {
		return
	}

	// Based on the requested path, serve the appropriate page.
	switch requestedPath {
	case "/style.css", "style.css":
		w.Header().Set("Content-Type", "text/css")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(web.Style)
	case "/favicon.ico", "favicon.ico":
		w.Header().Set("Content-Type", "image/x-icon")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(web.Favicon)
	case "/", "":
		httpServeSubpage(w, "index")
	case "/cache":
		httpServeSubpage(w, "cache")
	case "/stats":
		httpServeSubpage(w, "stats")
	case "/setup":
		httpServeSubpage(w, "setup")
	case "/revocation.crl":
		httpServeCRL(w, r)
	case "/goaptcacher.crt":
		httpServeCertificate(w, r)
	default:
		// Serve a 404 page
		w.WriteHeader(http.StatusNotFound)
		httpServeSubpage(w, "404")
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
		"Version":          buildinfo.Version,
		"Contact":          htmltemplate.HTML(strings.TrimSpace(config.Index.Contact)),
		"Year":             time.Now().Year(),
	}
}

func activeNavFromSubpage(subpage string) string {
	switch subpage {
	case "setup":
		return "setup"
	case "stats", "cache":
		return "stats"
	default:
		return "index"
	}
}

// httpServeSubpage is a helper function that serves a subpage of the main page
// template.
func httpServeSubpage(w http.ResponseWriter, subpage string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// pageContent contains the main content of the requested page.
	var pageContent string
	var title string

	switch subpage {
	case "index":
		pageContent = httpPageIndex()
		title = "GoAPTCacher - Overview"
	case "cache":
		pageContent = httpPageCache()
		title = "GoAPTCacher - Cache"
	case "stats":
		pageContent = httpPageStats()
		title = "GoAPTCacher - Statistics"
	case "setup":
		pageContent = httpPageSetup()
		title = "GoAPTCacher - Setup"
	case "404":
		pageContent = `<section class="panel stack-lg">
			<p class="eyebrow">Error 404</p>
			<h2>Page not found</h2>
			<p class="lead">The requested route does not exist on this instance.</p>
			<div class="actions">
				<a class="button" href="/_goaptcacher/">Back to overview</a>
				<a class="button button-secondary" href="/_goaptcacher/setup">Open setup guide</a>
			</div>
		</section>`
		title = "GoAPTCacher - Not found"
	}

	// Execute the template with the main page content and the template
	// variables.
	temp, err := web.GetTemplate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = temp.Execute(w, map[string]any{
		"Title":   title,
		"Content": htmltemplate.HTML(pageContent),
		"Const":   helperHTTPConstants(),
		"Active":  activeNavFromSubpage(subpage),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func httpPageIndex() string {
	host := preferredIndexHost()
	httpEndpoint := fmt.Sprintf("http://%s:%d", host, config.ListenPort)

	httpsModeClass := "badge--danger"
	httpsModeLabel := "Disabled"
	httpsModeDetails := "HTTPS requests are blocked by configuration."
	if config.HTTPS.Intercept {
		httpsModeClass = "badge--warn"
		httpsModeLabel = "Interception enabled"
		httpsModeDetails = "HTTPS traffic is decrypted on the proxy so encrypted repositories can be cached."
	} else if !config.HTTPS.Prevent {
		httpsModeClass = "badge--good"
		httpsModeLabel = "Tunnel mode"
		httpsModeDetails = "HTTPS traffic is forwarded end-to-end without decryption."
	}

	var builder strings.Builder

	builder.WriteString(`<section class="hero stack-md">
		<p class="eyebrow">Service Overview</p>
		<h2>APT cache proxy control center</h2>
		<p class="lead">This instance accelerates package installs by keeping frequently requested APT files on local storage and reusing them across clients.</p>
		<div class="actions">
			<a class="button" href="/_goaptcacher/stats">View live statistics</a>
			<a class="button button-secondary" href="/_goaptcacher/setup">Open setup guide</a>
		</div>
	</section>`)

	builder.WriteString(`<section class="grid">
		<article class="panel stack-md">
			<h3>Proxy status</h3>
			<ul class="status-list">
				<li><span>HTTP proxy</span><span class="badge badge--good">Enabled</span></li>
				<li><span>HTTPS proxy</span><span class="badge ` + httpsModeClass + `">` + escapeHTML(httpsModeLabel) + `</span></li>
			</ul>
			<br>
			<p class="muted">` + escapeHTML(httpsModeDetails) + `</p>
			<dl class="detail-list">
				<div><dt>HTTP endpoint</dt><dd><code>` + escapeHTML(httpEndpoint) + `</code></dd></div>`)

	if config.HTTPS.Intercept {
		httpsEndpoint := fmt.Sprintf("https://%s:%d", host, effectiveHTTPSPort())
		builder.WriteString(`<div><dt>HTTPS endpoint</dt><dd><code>` + escapeHTML(httpsEndpoint) + `</code></dd></div>`)
	}

	if len(config.AlternativePorts) > 0 {
		builder.WriteString(`<div><dt>Additional ports</dt><dd>` + renderChipList(config.AlternativePorts, "port") + `</dd></div>`)
	} else {
		builder.WriteString(`<div><dt>Additional ports</dt><dd class="muted">No alternative listener ports configured.</dd></div>`)
	}

	builder.WriteString(`</dl>
		</article>
		<article class="panel stack-md">
			<h3>Domain policy</h3>
			<p class="muted">Domain filtering controls which repositories are cached versus proxied without caching.</p>
			<h4>Cached domains</h4>`)

	if len(config.Domains) == 0 {
		builder.WriteString(`<p class="muted">No allowlist set. Requests to all domains are accepted.</p>`)
	} else {
		builder.WriteString(renderChipList(config.Domains, "domain"))
	}

	builder.WriteString(`<h4>Passthrough domains</h4>`)
	if len(config.PassthroughDomains) == 0 {
		builder.WriteString(`<p class="muted">No passthrough domains configured.</p>`)
	} else {
		builder.WriteString(renderChipList(config.PassthroughDomains, "domain"))
	}

	builder.WriteString(`</article>
	</section>`)

	builder.WriteString(`<section class="panel stack-md">
		<h3>Mirror routing</h3>
		<p class="muted">Remaps and overrides are applied before cache lookup.</p>
		<div class="grid">`)

	builder.WriteString(`<article class="panel panel-inner stack-sm">
		<h4>URL remaps</h4>`)
	if len(config.Remap) == 0 {
		builder.WriteString(`<p class="muted">No remap rules configured.</p>`)
	} else {
		builder.WriteString(`<div class="data-table-wrap"><table class="data-table data-table-compact"><thead><tr><th>From</th><th>To</th></tr></thead><tbody>`)
		for _, remap := range config.Remap {
			builder.WriteString(`<tr><td><code>` + escapeHTML(remap.From) + `</code></td><td><code>` + escapeHTML(remap.To) + `</code></td></tr>`)
		}
		builder.WriteString(`</tbody></table></div>`)
	}
	builder.WriteString(`</article>`)

	builder.WriteString(`<article class="panel panel-inner stack-sm">
		<h4>Distribution overrides</h4>
		<ul class="simple-list">`)
	if config.Overrides.UbuntuServer != "" {
		builder.WriteString(`<li><strong>Ubuntu:</strong> <code>` + escapeHTML(config.Overrides.UbuntuServer) + `</code></li>`)
	}
	if config.Overrides.DebianServer != "" {
		builder.WriteString(`<li><strong>Debian:</strong> <code>` + escapeHTML(config.Overrides.DebianServer) + `</code></li>`)
	}
	if config.Overrides.UbuntuServer == "" && config.Overrides.DebianServer == "" {
		builder.WriteString(`<li class="muted">No distribution overrides configured.</li>`)
	}
	builder.WriteString(`</ul>
	</article>
	</div>
	</section>`)

	if config.HTTPS.Intercept {
		builder.WriteString(`<section class="note note-warning">HTTPS interception is active. Clients must trust the proxy certificate chain to avoid TLS errors.<br>See the <a href="/_goaptcacher/setup">setup page</a> for instructions.</section>`)
	}

	return builder.String()
}

// httpPageStats returns the page content for the stats page containing the
// cache statistics of this proxy server.
func httpPageStats() string {
	var filesCached, totalSize uint64
	filesCached, totalSize, err := cache.GetCacheUsage()
	if err != nil {
		log.Printf("[ERROR:WEB] Error collecting cache usage: %s\n", err)
	}

	statsSnapshot := cache.GetStatsSnapshot(statsHistoryDays)
	totalRequests := statsSnapshot.Totals.Requests
	totalHits := statsSnapshot.Totals.Hits
	totalMisses := statsSnapshot.Totals.Misses
	totalTunnel := statsSnapshot.Totals.Tunnel
	totalTrafficDown := statsSnapshot.Totals.TrafficDown
	totalTrafficUp := statsSnapshot.Totals.TrafficUp
	totalTunnelTransfer := statsSnapshot.Totals.TunnelTransfer

	hitRate := safePercent(totalHits, totalRequests)
	missRate := safePercent(totalMisses, totalRequests)
	tunnelShare := safePercent(totalTunnel, totalRequests)
	upstreamShare := safePercent(totalTrafficDown, totalTrafficUp)

	var estimatedSaved uint64
	if totalTrafficUp > totalTrafficDown {
		estimatedSaved = totalTrafficUp - totalTrafficDown
	}

	storageTotal, storageUsed, storageErr := getStorageInfo()
	storageUsage := uint64(0)
	if storageErr == nil {
		storageUsage = safePercent(storageUsed, storageTotal)
	}

	firstSeenText := "No traffic recorded yet"
	if totalRequests > 0 {
		firstSeenText = statsSnapshot.OldestDay.Format("2006-01-02")
	}

	var builder strings.Builder

	builder.WriteString(`<section class="hero stack-md">
		<p class="eyebrow">Statistics</p>
		<h2>Cache and traffic analytics</h2>
		<p class="lead">Lifetime totals start at <code>` + escapeHTML(firstSeenText) + `</code>. Daily breakdown below shows the last ` + strconv.Itoa(statsHistoryDays) + ` recorded days.</p>
	</section>`)

	builder.WriteString(`<section class="metric-grid">`)
	builder.WriteString(renderMetricCard("Requests", strconv.FormatUint(totalRequests, 10), "All proxied requests"))
	builder.WriteString(renderMetricCard("Cache hit rate", fmt.Sprintf("%d%%", hitRate), fmt.Sprintf("Miss rate %d%%", missRate)))
	builder.WriteString(renderMetricCard("Cached files", strconv.FormatUint(filesCached, 10), prettifyBytes(totalSize)+" total"))
	builder.WriteString(renderMetricCard("Traffic to clients", prettifyBytes(totalTrafficUp), "Data delivered by the proxy"))
	builder.WriteString(renderMetricCard("Traffic from upstream", prettifyBytes(totalTrafficDown), fmt.Sprintf("%d%% of served traffic", upstreamShare)))
	builder.WriteString(renderMetricCard("Tunnel requests", strconv.FormatUint(totalTunnel, 10), fmt.Sprintf("%d%% request share", tunnelShare)))
	builder.WriteString(renderMetricCard("Tunnel transfer", prettifyBytes(totalTunnelTransfer), "Traffic that bypassed cache storage"))

	if storageErr == nil {
		builder.WriteString(renderMetricCard("Filesystem usage", fmt.Sprintf("%d%%", storageUsage), fmt.Sprintf("%s of %s used", prettifyBytes(storageUsed), prettifyBytes(storageTotal))))
	} else {
		builder.WriteString(renderMetricCard("Filesystem usage", "n/a", "Unable to read storage stats"))
	}
	builder.WriteString(`</section>`)

	builder.WriteString(`<section class="panel stack-sm">
		<h3>Efficiency summary</h3>
		<p class="muted">Estimated upstream traffic saved by cache hits: <strong>` + escapeHTML(prettifyBytes(estimatedSaved)) + `</strong>.</p>
	</section>`)

	builder.WriteString(`<section class="panel stack-md">
		<h3>Daily breakdown</h3>`)

	if len(statsSnapshot.Daily) == 0 {
		builder.WriteString(`<p class="muted">No daily entries available yet.</p>`)
	} else {
		builder.WriteString(`<div class="data-table-wrap"><table class="data-table">
			<thead>
				<tr>
					<th>Date</th>
					<th>Requests</th>
					<th>Hits</th>
					<th>Misses</th>
					<th>Tunnel</th>
					<th>Served traffic</th>
					<th>Upstream traffic</th>
				</tr>
			</thead>
			<tbody>`)
		for _, entry := range statsSnapshot.Daily {
			builder.WriteString(fmt.Sprintf(
				"<tr><td>%s</td><td>%d</td><td>%d (%d%%)</td><td>%d (%d%%)</td><td>%d (%d%%)</td><td>%s</td><td>%s (%d%%)</td></tr>",
				entry.Date.Format("2006-01-02"),
				entry.Requests,
				entry.Hits,
				safePercent(entry.Hits, entry.Requests),
				entry.Misses,
				safePercent(entry.Misses, entry.Requests),
				entry.Tunnel,
				safePercent(entry.Tunnel, entry.Requests),
				prettifyBytes(entry.TrafficUp),
				prettifyBytes(entry.TrafficDown),
				safePercent(entry.TrafficDown, entry.TrafficUp),
			))
		}
		builder.WriteString(`</tbody></table></div>`)
	}

	builder.WriteString(`</section>`)
	return builder.String()
}

func httpPageCache() string {
	filesCached, totalSize, err := cache.GetCacheUsage()
	if err != nil {
		log.Printf("[ERROR:WEB] Error collecting cache usage: %s\n", err)
	}

	storageTotal, storageUsed, storageErr := getStorageInfo()
	storageUsage := uint64(0)
	if storageErr == nil {
		storageUsage = safePercent(storageUsed, storageTotal)
	}

	expirationText := "Automatic expiration is disabled."
	if config.Expiration.UnusedDays > 0 {
		expirationText = fmt.Sprintf("Unused cached files are removed after %d days.", config.Expiration.UnusedDays)
	}

	var builder strings.Builder
	builder.WriteString(`<section class="hero stack-md">
		<p class="eyebrow">Cache</p>
		<h2>Storage overview</h2>
		<p class="lead">Current cache footprint and lifecycle settings for this instance.</p>
	</section>`)

	builder.WriteString(`<section class="metric-grid">`)
	builder.WriteString(renderMetricCard("Cached files", strconv.FormatUint(filesCached, 10), "Tracked entries on disk"))
	builder.WriteString(renderMetricCard("Cached size", prettifyBytes(totalSize), "Total local data volume"))
	if storageErr == nil {
		builder.WriteString(renderMetricCard("Filesystem usage", fmt.Sprintf("%d%%", storageUsage), fmt.Sprintf("%s of %s used", prettifyBytes(storageUsed), prettifyBytes(storageTotal))))
	} else {
		builder.WriteString(renderMetricCard("Filesystem usage", "n/a", "Unable to read storage stats"))
	}
	builder.WriteString(`</section>`)

	builder.WriteString(`<section class="panel stack-sm">
		<h3>Retention policy</h3>
		<p class="muted">` + escapeHTML(expirationText) + `</p>
		<div class="actions">
			<a class="button" href="/_goaptcacher/stats">Open statistics</a>
			<a class="button button-secondary" href="/_goaptcacher/setup">Open setup guide</a>
		</div>
	</section>`)

	return builder.String()
}

// httpPageSetup returns the page content for the setup page containing the
// configuration of this proxy server.
func httpPageSetup() string {
	domain := preferredIndexHost()
	httpPort := strconv.Itoa(config.ListenPort)
	httpsPort := strconv.Itoa(effectiveHTTPSPort())

	httpsNote := "HTTPS requests are tunneled without interception."
	if config.HTTPS.Intercept {
		httpsNote = "HTTPS interception is enabled. Clients must trust the proxy CA certificate."
	} else if config.HTTPS.Prevent {
		httpsNote = "HTTPS proxying is disabled. Only HTTP repositories can use the proxy."
	}

	var builder strings.Builder
	builder.WriteString(`<section class="hero stack-md">
		<p class="eyebrow">Setup</p>
		<h2>Connect clients to this cache</h2>
		<p class="lead">Choose one of the configuration methods below. The static proxy directives are the simplest and most reliable option.</p>
		<section class="note">` + escapeHTML(httpsNote) + `</section>
	</section>`)

	builder.WriteString(`<section class="grid">`)

	builder.WriteString(`<article class="panel stack-md">
		<h3>1) Static APT proxy directives</h3>
		<p>Add this file on each client:</p>
		<pre><code>/etc/apt/apt.conf.d/10proxy</code></pre>
		<pre><code>Acquire::http::Proxy "http://` + escapeHTML(domain) + `:` + escapeHTML(httpPort) + `/";
Acquire::https::Proxy "http://` + escapeHTML(domain) + `:` + escapeHTML(httpPort) + `/";</code></pre>
		<p class="muted">
			Works for managed servers, VMs and persistent hosts. Configuration is static and not suitable for mobile clients.<br>
			Best used with Ansible, Puppet, Chef or similar configuration management tools.
		</p>
	</article>`)

	builder.WriteString(`<article class="panel stack-md">
		<h3>2) Auto-discovery with auto-apt-proxy</h3>
		<p>Install discovery helper on clients:</p>
		<pre><code>apt install auto-apt-proxy</code></pre>
		<p>Create an SRV record for your internal domain:</p>
		<pre><code>_apt_proxy._tcp.example.com. 3600 IN SRV 0 0 ` + escapeHTML(httpPort) + ` ` + escapeHTML(domain) + `.</code></pre>
		<p class="muted">
			Useful for ephemeral workers, CI runners and laptops switching networks.<br>
			<strong>Note:</strong> auto-apt-proxy uses the domain name of your hosts FQDN to discover the proxy and DNS-Suffix.
		</p>
	</article>`)

	builder.WriteString(`<article class="panel stack-md">
		<h3>3) Per-repository DNS SRV override</h3>
		<p>Use this when you want DNS to steer repository traffic to the proxy without host-level apt.conf changes.</p>
		<pre><code>_http._tcp.at.archive.ubuntu.com. 3600 IN SRV 0 0 ` + escapeHTML(httpPort) + ` ` + escapeHTML(domain) + `.
_https._tcp.download.docker.com. 3600 IN SRV 0 0 ` + escapeHTML(httpsPort) + ` ` + escapeHTML(domain) + `.</code></pre>
		<p class="muted">Add records for every repository domain that should pass through the proxy.</p>
	</article>`)

	builder.WriteString(`<article class="panel stack-md">
		<h3>4) GitLab CI integration</h3>
		<p>Add the following lines to your .gitlab-ci.yml to enable the proxy for CI jobs:</p>
		<pre><code>  before_script:
    - echo 'Acquire::http::Proxy "http://` + escapeHTML(domain) + `:` + escapeHTML(httpPort) + `/";' > /etc/apt/apt.conf.d/10proxy
    - echo 'Acquire::https::Proxy "http://` + escapeHTML(domain) + `:` + escapeHTML(httpPort) + `/";' >> /etc/apt/apt.conf.d/10proxy
</code></pre>
		<p class="muted">Works for ephemeral CI runners without static configuration. Not suitable for general client use.</p>
	</article>`)

	builder.WriteString(`<article class="panel stack-md">
		<h3>Validation checklist</h3>
		<ul class="simple-list">
			<li>Verify APT proxy settings with <code>apt-config dump | grep -E 'Acquire::(http|https)::Proxy'</code>.</li>
			<li>Run <code>apt update</code> and then check <a href="/_goaptcacher/stats">statistics</a> for incoming requests.</li>
			<li>If HTTPS interception is enabled, deploy the CA certificate to all clients.</li>
		</ul>
	</article>`)

	builder.WriteString(`<article class="panel stack-md">
		<h3>Troubleshooting</h3>
		<ul class="simple-list">
			<li>Check the <a href="/_goaptcacher/stats">statistics</a> page for incoming requests and cache performance.</li>
			<li>Inspect proxy logs for errors or misconfigurations.</li>
			<li>Ensure clients can resolve the proxy hostname and connect to the specified ports.</li>
			<li>If using HTTPS interception, verify that clients trust the proxy CA certificate.</li>
			<li>Review domain filtering and remap rules if certain repositories are not being cached as expected.</li>
		</ul>
	</article>`)

	builder.WriteString(`</section>`)
	return builder.String()
}

func renderMetricCard(label string, value string, hint string) string {
	return `<article class="metric-card">
		<p class="metric-label">` + escapeHTML(label) + `</p>
		<p class="metric-value">` + escapeHTML(value) + `</p>
		<p class="metric-hint">` + escapeHTML(hint) + `</p>
	</article>`
}

func renderChipList(values any, valuePrefix string) string {
	var builder strings.Builder
	builder.WriteString(`<ul class="chip-list">`)

	switch v := values.(type) {
	case []string:
		for _, value := range v {
			builder.WriteString(`<li class="chip"><code>` + escapeHTML(value) + `</code></li>`)
		}
	case []int:
		for _, value := range v {
			builder.WriteString(`<li class="chip"><code>` + escapeHTML(valuePrefix+" "+strconv.Itoa(value)) + `</code></li>`)
		}
	}

	builder.WriteString(`</ul>`)
	return builder.String()
}

func escapeHTML(value string) string {
	return html.EscapeString(value)
}

func preferredIndexHost() string {
	if len(config.Index.Hostnames) > 0 {
		host := strings.TrimSpace(config.Index.Hostnames[0])
		if host != "" {
			return host
		}
	}

	ip, err := getLocalIP()
	if err != nil {
		log.Printf("[ERROR:WEB] Error getting local IP address: %s\n", err)
		return "127.0.0.1"
	}
	if ip == "" {
		return "127.0.0.1"
	}

	return ip
}

func effectiveHTTPSPort() int {
	if config.ListenPortSecure > 0 {
		return config.ListenPortSecure
	}

	return 8091
}

// prettifyBytes is a helper function that returns a human-readable string of
// the given bytes. It converts the bytes to the most appropriate unit (B, KiB,
// MiB, GiB, TiB, PiB, EiB).
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

func safePercent(value uint64, total uint64) uint64 {
	if total == 0 {
		return 0
	}
	return (value * 100) / total
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

// httpServeCRL serves the Certificate Revocation List (CRL) if enabled in the configuration.
func httpServeCRL(w http.ResponseWriter, r *http.Request) {
	if !config.HTTPS.EnableCRL {
		http.Error(w, "CRL not enabled", http.StatusNotFound)
		return
	}

	// Serve the CRL file
	http.ServeFile(w, r, config.CacheDirectory+"/crl.pem")
}

// httpServeCertificate serves the public certificate used for HTTPS
// interception when requested through AIA.
func httpServeCertificate(w http.ResponseWriter, r *http.Request) {
	if !config.HTTPS.Intercept {
		http.Error(w, "HTTPS interception not enabled", http.StatusNotFound)
		return
	}

	// Serve the public certificate file
	http.ServeFile(w, r, config.HTTPS.CertificatePublicKey)
}

// getStorageInfo returns the total and used storage space of the cache directory.
func getStorageInfo() (total uint64, used uint64, err error) {
	var stat syscall.Statfs_t
	err = syscall.Statfs(config.CacheDirectory, &stat)
	if err != nil {
		return 0, 0, err
	}

	blockSize, err := strconv.ParseUint(strconv.FormatInt(stat.Bsize, 10), 10, 64)
	if err != nil {
		return 0, 0, err
	}

	total = stat.Blocks * blockSize
	used = (stat.Blocks - stat.Bfree) * blockSize
	return total, used, nil
}
