# Go APT Cacher

GoAPTCacher is a pull-through caching proxy for Debian/Ubuntu APT repositories (similar to `apt-cacher-ng` or `squid` with store), built in Go. It speeds up package installs in CI/CD and isolated networks by caching requested artifacts on disk and serving repeat requests locally.

## Features

- **Pull-through cache** – Only caches packages that clients actually request.
- **Local filesystem store** – Avoids re-downloads; supports pre-seeding.
- **HTTP & HTTPS** – Cache both HTTP and HTTPS repositories.
- **On-the-fly certificates (optional)** – Intercept HTTPS by minting per-host leaf certs from an intermediate CA to enable caching of encrypted traffic.
- **HTTPS passthrough mode** – Skip interception and tunnel to origin while still enforcing allowlists.
- **Domain allow/deny** – Restrict to an explicit set of repository domains.
- **URL rewrite & mirror overrides** – e.g., us.archive.ubuntu.com -> de.archive.ubuntu.com, or force NetCologne mirrors.
- **Proxy-only mode** – Use purely as an APT proxy without interception.
- **Auto-discovery** – DNS SRV and mDNS helpers so clients can find the cache automatically.
- **Web interface** – Inspect cache statistics, health, and configuration details.
- **Expiration** – Automatic purge of partial/abandoned downloads and stale entries.

> **Preview Build**
> The main branch contains the latest changes and is considered a preview build. While it is kept executable, it may contain bugs. Use for testing/evaluation; avoid in production.

## Quick Start

GoAPTCacher can be installed by apt using the APT repository hosted on repo.bella.network. A configuration guide can be found on [https://repo.bella.network/](https://repo.bella.network/).

```bash
# 1) Install (via APT repo)
echo "Types: deb
URIs: https://repo.bella.network/deb
Suites: stable
Components: main
Architectures: amd64 arm64
Signed-By: /usr/share/keyrings/bella-archive-keyring.gpg
" | sudo tee /etc/apt/sources.list.d/repo.bella.network.sources

curl -fsSL https://repo.bella.network/_static/bella-archive-keyring.gpg \
  | sudo tee /usr/share/keyrings/bella-archive-keyring.gpg >/dev/null

sudo apt update && sudo apt install goaptcacher

# 2) Minimal config
sudo tee /etc/goaptcacher.yaml >/dev/null <<'YAML'
cache_directory: "/var/cache/goaptcacher"
domains:
  - "archive.ubuntu.com"
  - "security.ubuntu.com"
  - "ports.ubuntu.com"
passthrough_domains:
  - "repo.bella.network"
expiration:
  unused_days: 60
index:
  enable: true
YAML

# 3) Start
sudo systemctl enable --now goaptcacher

# 4) Point a client at the proxy (on the client machine)
echo 'Acquire::http::Proxy "http://<cache-host>:8090/";' \
 | sudo tee /etc/apt/apt.conf.d/01proxy
sudo apt update
```

After installation, you can access the web interface at `http://<cache-host>:8090/_goaptcacher/` or `https://<cache-host>:8091/_goaptcacher/` to view cache statistics and configuration details.

### Docker

You can also run GoAPTCacher in a Docker container. Here is an example command to get you started:

```bash
docker run -d --name goaptcacher \
  -p 8090:8090 \
  -p 8091:8091 \
  -v $PWD/goaptcacher.yaml:/etc/goaptcacher.yaml:ro \
  -v $PWD/cache:/var/cache/goaptcacher \
  registry.gitlab.com/bella.network/goaptcacher:latest
```

## What this program does

This program is a pull-through apt cacher similar to apt-cacher-ng and squid with local file cache. It is designed to be used in isolated environments and CI/CD pipelines to speed up package downloads and reduce the load on the package mirrors. It caches only the packages that were requested by clients and stores them in a local directory to avoid re-downloading. It serves packages over HTTP and HTTPS and supports on-the-fly certificate generation. It also supports multiple package mirrors, HTTPS-passthrough, proxy support, SRV record support, and URL rewriting. It provides a web interface to view cache statistics and configuration details.

## What this progran not does

This program does not cache all packages from a package mirror, only the packages that were requested by clients. It does not cache packages that are not requested by clients, there is no pre-caching of packages.

## Why this package was made?

I was dissatisfied with the performance and quality of the apt-cacher-ng package. As under [Ubuntu Bug Tracker](https://bugs.launchpad.net/ubuntu/+source/apt-cacher-ng) and [Debian Bug report logs](https://bugs.debian.org/cgi-bin/pkgreport.cgi?pkg=apt-cacher-ng) apt-cacher-ng has some unresolved issues. Some of these have also affected me and my friends over the years and have repeatedly hindered my home lab and my company and have repeatedly disrupted normal system operation and CI builds.

That's why I created an alternative with this program that is more in line with my wishes and requirements.

## Configuration

The configuration file is structured in YAML format and contains the following example configuration. This is a sample configuration file with explanations for each section.

```yaml
# The main cache directory where the packages and database are stored
cache_directory: "/var/cache/goaptcacher"

# MariabDB/MySQL connection details (for internal indexing and stats)
# The user must have access to create tables and indexes, these are created automatically if they do not exist.
database:
  hostname: "127.0.0.1"
  username: "goaptcacher"
  password: "this-Is-Not-a-good-password--use-something-better!"
  database: "goaptcacher"
  port: 3306

# List of domains which are allowed to be cached. Requests to other domains will be denied.
# Supports bare domains and leading-dot wildcards (.debian.org).
# If empty or not set, all domains are allowed.
domains:
  - "archive.ubuntu.com" # Ubuntu archive
  - "security.ubuntu.com" # Ubuntu security
  - "ports.ubuntu.com" # Ubuntu ports
  - "esm.ubuntu.com" # Ubuntu ESM
  - "motd.ubuntu.com" # Ubuntu MOTD
  - "ppa.launchpad.net" # Ubuntu PPAs

  - "security.debian.org" # Debian security
  - ".debian.org" # Debian archive

  - "raspbian.raspberrypi.org" # Raspbian
  - "archive.raspberrypi.org" # Raspbian archive

# Passthrough domains are always tunneled to origin, even when interception is enabled.
# Use this for domains that require authentication or have certificate pinning. Nothing is cached for these domains.
passthrough_domains:
  - "esm.ubuntu.com" # Ubuntu ESM (authentication required)
  - "enterprise.proxmox.com" # Proxmox VE with subscription (authentication required)
  - "repo.bella.network" # Bella Network APT repo (self-hosted - GoAPTCacher)

# HTTPS interception settings (if enabled, requires cert/key) and allows to intercept HTTPS traffic to cache packages that are served over HTTPS.
https:
  intercept: true
  cert: "public.key"
  key: "private.key"
  password: "mysecret" # Optional password for encrypted key files
  certificate_domain: "cache.example.com" # The domain name that will be used in the generated leaf certificates (must match the SAN of the cert)
  aia_address: "http://cache.example.com/goaptcacher.crt" # Authority Information Access (AIA) URL to include in leaf certs for clients to download the CA cert
  enable_crl: false # Enable CRL generation and serving (allows clients to check for revoked certs)

# Overrides specific distributions to use a different default mirror than the official one.
# Useful for forcing local mirrors or faster mirrors.
overrides:
  ubuntu_server: "mirror.netcologne.de"
  debian_server: "mirror.netcologne.de"

# Allows overriding specific domains to use a different mirror. Useful for forcing local mirrors or faster mirrors.
remap:
  - from: "ubuntu.lagis.at"
    to: "at.archive.ubuntu.com"

# Web interface settings to display overview of configured domains, setup guide and cache stats.
index:
  enable: true
  hostnames: # List of hostnames or IPs the web interface defaults to for certificate generation and display.
    - "cache.example.com"
    - "10.20.30.40"
  contact: "Contact <a href='mailto:your@mail.address'>First Last</a> for support" # Contact information to display in the web interface footer.

# mDNS/Bonjour service announcement to allow clients to discover the proxy automatically.
# Clients can then find the proxy using tools like avahi-browse or dns-sd.
mdns: true

# Expiration settings to automatically delete unused or old packages from the cache.
expiration:
  unused_days: 60 # Delete packages that have not been accessed in the last N days.
```

**Notes**

- Set https.intercept: false to run in pure proxy/tunnel mode for TLS.
- domains supports bare domains and leading-dot wildcards (.debian.org).
- passthrough_domains are always tunneled even when interception is on.

### Client configuration (APT)

Set a proxy (works for HTTP repositories):

```bash
echo 'Acquire::http::Proxy "http://<cache-host>:8080/";' \
 | sudo tee /etc/apt/apt.conf.d/01proxy
```

If you use HTTPS interception, clients must trust your CA (see next section).

### HTTPS interception & trust (optional)

If `https.intercept: true`:

**Intermediate CA**: Provide an intermediate CA certificate/key in the paths configured under https.cert and https.key.

**Distribute CA to clients** so they trust the on-the-fly leaf certs:

```bash
sudo cp your-ca.crt /usr/local/share/ca-certificates/goaptcacher.crt
sudo update-ca-certificates
```

Verify:

```bash
export https_proxy="http://<cache-host>:8090"
curl -v https://archive.ubuntu.com/ubuntu/dists/
```

If you can't roll out a CA, set `intercept: false` and rely on passthrough.

For HTTPS repositories you may need to add the following to your APT config:

```bash
echo 'Acquire::https::Proxy "http://<cache-host>:8080/";' \
 | sudo tee /etc/apt/apt.conf.d/01proxy
```

### Auto-discovery (optional)

GoAPTCacher is able to announce itself via DNS records, mDNS and DNS-SRV records so clients can find it automatically.

- **DNS records**: Using auto-apt-proxy (`apt install auto-apt-proxy`), clients can discover the proxy via DNS. You need to create a DNS SRV record for `_apt_proxy._tcp.<your-dns-suffix>` in your domain pointing to the GoAPTCacher instance. E.g. `_apt_proxy._tcp.example.com. 3600 IN SRV 0 0 8090 cache.example.com.`
- **DNS-SRV override**: Configure DNS-SRV records for the domains you want to cache. E.g. `_http._tcp.at.archive.ubuntu.com. 3600 IN SRV 0 0 8090 cache.example.com.`. This will make clients use the proxy for that domain.
- **mDNS**: Enable `mdns: true` to announce the service via mDNS/Bonjour. Clients can then discover the service using tools like `avahi-browse` or `dns-sd`.

## Troubleshooting

- Clients see TLS errors -> The client doesn't trust your interception CA. Either deploy the CA or disable interception.
- Misses for expected packages -> Confirm the client is actually using the proxy (01proxy present and correct host/port).
- Origin errors (403/401) -> Repository requires subscription/auth; add the domain to passthrough_domains.
- Slow or no UI -> Check service logs; verify `index.enable: true` and that the service binds to the expected interface.
- Hash mismatches -> Check if a mirror override or remap is causing packages to be fetched from unexpected hosts. You can also delete the corrupted files from the cache directory.

# Feedback and Contributions

Feedback, bug reports, and contributions are welcome! Please open an issue or a merge request on the [GitLab repository](https://gitlab.com/bella.network/goaptcacher).
