# Go APT Cacher 🚀

GoAPTCacher is a pull-through caching proxy for Debian/Ubuntu style APT repositories (similar to `apt-cacher-ng` or `squid` with store).
It caches requested artifacts on local disk and serves repeated requests locally. This can save bandwidth and speed up package installations in environments with multiple machines or CI runners. Additionally it allows to isolate systems from accessing the public internet while still providing access to necessary package repositories.

## Project status ⚠️

`main` is the current development branch. It is kept runnable but can contain breaking changes or bugs.
For production use, please check the [releases](https://gitlab.com/bella.network/goaptcacher/-/releases) and use tagged versions. The preferred installation method is via the Debian/Ubuntu package, but you can also build from source or use the provided Docker image.

## Feature overview ✨

- Pull-through cache for APT traffic (`GET`/`HEAD`), including streaming cache-miss downloads.
- HTTP proxying and HTTPS support via:
  - tunnel mode (`CONNECT`, no TLS interception)
  - optional TLS interception (MITM) for caching HTTPS repositories
- Domain policy controls:
  - `domains` (cacheable domains)
  - `passthrough_domains` (always proxied, never cached)
- Mirror routing:
  - distro overrides (`ubuntu_server`, `debian_server`)
  - path remap rules (`remap`)
- Automatic cache refresh logic with conditional upstream checks (`If-Modified-Since`/`If-None-Match`).
- Automatic expiration of unused cache entries.
- Built-in web UI (`/_goaptcacher/`) with overview, cache metrics, and setup guide.
- Persistent statistics in `cache_directory/.stats.json`.
- Persistent per-file metadata in sidecar files (`*.access.json`).
- Optional mDNS announcement (`_apt_proxy._tcp.local`).
- Optional CRL generation and certificate download endpoint for interception setups.
- Debug mode with JSON diagnostics and optional pprof endpoints/snapshots.

## Installation 📦

### Debian/Ubuntu package 🐧

GoAPTCacher can be installed by apt using the APT repository hosted on repo.bella.network. A configuration guide can be found on [https://repo.bella.network/](https://repo.bella.network/).

```bash
echo "Types: deb
URIs: https://repo.bella.network/deb
Suites: stable
Components: main
Architectures: amd64 arm64
Signed-By: /usr/share/keyrings/bella-archive-keyring.gpg
" | sudo tee /etc/apt/sources.list.d/repo.bella.network.sources

curl -fsSL https://repo.bella.network/_static/bella-archive-keyring.gpg \
  | sudo tee /usr/share/keyrings/bella-archive-keyring.gpg >/dev/null

sudo apt update
sudo apt install goaptcacher
```

After installation, you can access the web interface at `http://<cache-host>:8090/_goaptcacher/` or `https://<cache-host>:8091/_goaptcacher/` to view cache statistics and configuration details.

The Debian/Ubuntu package also installs and enables `goaptcacher-repoverify.timer`. This `systemd` timer periodically verifies cached repository metadata and package files below `cache_directory`.

### Build from source 🛠️

You can also build GoAPTCacher from source. Make sure you have Go installed (latest version recommended). Then run:

```bash
go build -o goaptcacher ./cmd/goaptcacher
```

### Docker (example) 🐳

You can also run GoAPTCacher in a Docker container. Here is an example command to get you started:

```bash
docker run -d --name goaptcacher \
  -p 8090:8090 \
  -p 8091:8091 \
  -p 3142:3142 \
  -v "$PWD/config.yaml":/etc/goaptcacher.yaml:ro \
  -v "$PWD/cache":/var/cache/goaptcacher \
  registry.gitlab.com/bella.network/goaptcacher:latest
```

## What this program does

This program is a pull-through apt cacher similar to apt-cacher-ng and squid with local file cache. It is designed to be used in isolated environments and CI/CD pipelines to speed up package downloads and reduce the load on the package mirrors. It caches only the packages that were requested by clients and stores them in a local directory to avoid re-downloading. It serves packages over HTTP and HTTPS and supports on-the-fly certificate generation. It also supports multiple package mirrors, HTTPS-passthrough, proxy support, SRV record support, and URL rewriting. It provides a web interface to view cache statistics and configuration details.

## What this progran not does

This program does not cache all packages from a package mirror, only the packages that were requested by clients. It does not cache packages that are not requested by clients, there is no pre-caching of packages.

## Why this package was made?

I was dissatisfied with the performance and quality of the apt-cacher-ng package. As under [Ubuntu Bug Tracker](https://bugs.launchpad.net/ubuntu/+source/apt-cacher-ng) and [Debian Bug report logs](https://bugs.debian.org/cgi-bin/pkgreport.cgi?pkg=apt-cacher-ng) apt-cacher-ng has some unresolved issues. Some of these have also affected me and my friends over the years and have repeatedly hindered my home lab and my company and have repeatedly disrupted normal system operation and CI builds.

That's why I created an alternative with this program that is more in line with my wishes and requirements.

## Quick start ⚡

The configuration file is structured in YAML format and contains the following example configuration. This is a sample configuration file with explanations for each section.

1. 📝 Create config (example: `/etc/goaptcacher.yaml`):

```yaml
cache_directory: "/var/cache/goaptcacher"
listen_port: 8090
listen_port_secure: 8091

alternative_ports:
  - 3142

domains:
  - "archive.ubuntu.com"
  - "security.ubuntu.com"
  - "ports.ubuntu.com"
  - "security.debian.org"
  - ".debian.org"

passthrough_domains:
  - "esm.ubuntu.com"

https:
  prevent: false
  intercept: false

index:
  enable: true
  hostnames:
    - "cache.example.com"

```

All configuration options are explained within the example configuration file at [config.yaml-example](./config.yaml-example). You can use this file as a template and modify it according to your needs.

**Notes**

- Set https.intercept: false to run in pure proxy/tunnel mode for TLS.
- domains supports bare domains and leading-dot wildcards (.debian.org).
- passthrough_domains are always tunneled even when interception is on.

2. ▶️ Start service:

```bash
sudo systemctl enable --now goaptcacher
```

3. 👥 Configure client proxy:

```bash
cat <<'APT' | sudo tee /etc/apt/apt.conf.d/10proxy
Acquire::http::Proxy "http://<cache-host>:8090/";
Acquire::https::Proxy "http://<cache-host>:8090/";
APT

sudo apt update
```

## Request flow and cache behavior 🔄

- Supported methods: `GET`, `HEAD`, `CONNECT`.
- `GET`:
  - cache hit => serves file with `X-Cache: HIT`
  - cache miss => streams upstream response to client and cache with `X-Cache: MISS`
- `HEAD`:
  - if cached, returns file metadata headers
  - if not cached, file is fetched once and then headers are returned (`X-Cache: MISS`)
- `CONNECT`:
  - `https.prevent: true` => request is rejected (`403`)
  - passthrough domain or `https.intercept: false` => plain tunnel
  - `https.intercept: true` => intercepted TLS flow handled via proxy logic

### Important: empty domain configuration ❗

If both `domains` and `passthrough_domains` are empty, all hosts are allowed, but `GET`/`HEAD` requests are tunneled (effectively no cache usage). The service logs a warning for this mode.

## Web and debug endpoints 🌐

Base path: `/_goaptcacher/`

Note: requesting `/` returns `406 Not Acceptable` with a redirect hint to `/_goaptcacher/` (for `auto-apt-proxy` compatibility checks).

- `/_goaptcacher/` overview
- `/_goaptcacher/cache` cache/storage overview
- `/_goaptcacher/stats` request and traffic stats
- `/_goaptcacher/setup` client setup guide
- `/_goaptcacher/goaptcacher.crt` interception CA certificate (if interception is enabled)
- `/_goaptcacher/revocation.crl` CRL file (if CRL is enabled)
- `/robots.txt` disallow-all robots policy
- `/.well-known/security.txt` contact metadata for security reporting

Debug (only when `debug.enable: true`):

- `/_goaptcacher/debug` JSON runtime diagnostics
- `/_goaptcacher/debug/pprof` pprof handlers

`debug.allow_remote: false` restricts debug endpoints to loopback requests.

## Runtime options 🏁

Command line:

- `-h`, `--help` show help
- `-v`, `--version` show version/build info
- `-c`, `--config <path>` config file path
- `verify-repos` scan cached repositories and verify their metadata and package checksums

Environment variables:

- `CONFIG` config file path (used when `-c` is not set)
- `CACHE_DIR` overrides `cache_directory`

## Repository verification 🔍

GoAPTCacher can verify all cached repositories by scanning `cache_directory` for `dists/<distribution>/InRelease` files and then validating repository index files plus referenced `.deb` files. For each file, the strongest supported checksum from the repository metadata is used (`SHA512` preferred, `SHA256` fallback).

Manual execution:

```bash
goaptcacher verify-repos
```

When installed from the Debian/Ubuntu package, the following `systemd` units are added automatically:

- `goaptcacher-repoverify.service`
- `goaptcacher-repoverify.timer`

The timer runs once about 15 minutes after boot and then roughly every 24 hours with a randomized delay. It is enabled automatically on install and disabled automatically on package removal.

Useful administration commands:

```bash
sudo systemctl status goaptcacher-repoverify.timer
sudo systemctl list-timers goaptcacher-repoverify.timer
sudo systemctl start goaptcacher-repoverify.service
sudo journalctl -u goaptcacher-repoverify.service
```

Disable periodic verification manually:

```bash
sudo systemctl disable --now goaptcacher-repoverify.timer
```

Re-enable it:

```bash
sudo systemctl enable --now goaptcacher-repoverify.timer
```

## Auto-discovery notes 📡

- `auto-apt-proxy` clients can discover the proxy via DNS SRV `_apt_proxy._tcp.<domain>`.
- You can also use per-repository SRV records such as `_http._tcp.<repo-domain>` or `_https._tcp.<repo-domain>` to steer repository traffic through this proxy.

## Troubleshooting 🩺

- TLS failures on clients:
  - client does not trust interception CA; distribute CA or disable interception
- Packages not cached as expected:
  - verify domain policy (`domains`, `passthrough_domains`), then check `X-Cache` headers and `/_goaptcacher/stats`
- Unexpected passthrough/no cache:
  - verify that domain lists are not empty unless tunnel-only behavior is desired
- 403 errors:
  - host not in allowlist or `https.prevent` blocks `CONNECT`
- Storage errors:
  - cache miss writes can fail when disk space is insufficient (`507`/storage errors)

## Tested ✅

This program has been tested with the following repositories and environments:

- Ubuntu Desktop and Server
- Debian
- Proxmox VE and Proxmox Mail Gateway
- PPAs from Launchpad
- Raspbian / Raspberry Pi OS
- Private APT repositories like
  - Microsoft (VS Code, PowerShell, Edge, )
  - GitLab (GitLab CE/EE, GitLab Runner)
  - Docker
  - Syncthing
  - Cloudsmith (ISC Kea DHCP)
  - Smallstep (step-ca)
  - Google (Google Chrome)
- Repositories created with
  - aptly
  - reprepro

Please note that for Debian, requests to `/debian-security/` will always be mapped to `security.debian.org` regardless of the mirror used for other requests. This is due to the way Debian sometimes serves security updates from a different domain and the need to ensure that security updates are always fetched from the official security repository.

## Non-goals 🚫

- No full mirror sync/preload; only requested artifacts are cached.
- No generic forward proxy feature set beyond APT-oriented `GET`/`HEAD`/`CONNECT` flows.

# Feedback and Contributions

Feedback, bug reports, and contributions are welcome! Please open an issue or a merge request on the [GitLab repository](https://gitlab.com/bella.network/goaptcacher).
