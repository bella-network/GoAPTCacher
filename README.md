# Go APT Cacher

GoAPTCacher is a pull-through apt cacher similar to apt-cacher-ng and squid with local file cache. It is designed to be used in isolated environments and CI/CD pipelines to speed up package downloads and reduce the load on the package mirrors.

## Features

- Pull-through apt cacher - Only cache packages that were requested by clients
- Local file cache - Cache packages in a local directory to avoid re-downloading, also allows for pre-seeding the cache
- HTTP and HTTPS support - Serve packages over HTTP and HTTPS
- Support for on-the-fly certificate generation - Generate a self-signed certificate for HTTPS on the fly, uses intermediate CA for signing
- HTTPS-passthrough - Serve packages over HTTPS using the original package mirror's certificate
- Support for multiple package mirrors - Provide a list of allowed package repositories which clients can use
- Webinterface - View cache statistics and configuration details
- Proxy support - Configure GoAPTCacher as APT proxy server only for package downloads
- SRV record support - Automatically discover the GoAPTCacher server using DNS SRV records
- Rewrite support - Rewrite URLs to use a different package mirror, e.g. us.archive.ubuntu.com -> de.archive.ubuntu.com

## Installation

## Preview Build

The source within the branch main always contains the latest changes and is considered the preview build. This build is not recommended for production use as it may contain bugs and other issues. The preview build is intended for testing and evaluation purposes only. The main branch is designed to be executable and functional at all times in order to be able to test new functions as quickly as possible.

### APT Repository

GoAPTCacher can be installed by apt using the APT repository hosted on repo.bella.network. A configuration guide can be found on [https://repo.bella.network/](https://repo.bella.network/).

```bash
# Add the APT repository
echo "Types: deb
URIs: https://repo.bella.network/deb
Suites: stable
Components: main
Architectures: amd64 arm64
Signed-By: /usr/share/keyrings/bella-archive-keyring.gpg
" | sudo tee /etc/apt/sources.list.d/bella.sources

# Add the repository key
curl -fsSL https://repo.bella.network/_static/bella-archive-keyring.gpg | sudo tee /usr/share/keyrings/bella-archive-keyring.gpg > /dev/null

# Update the package list and install GoAPTCacher
sudo apt update; sudo apt install goaptcacher
```

## What this program does

This program is a pull-through apt cacher similar to apt-cacher-ng and squid with local file cache. It is designed to be used in isolated environments and CI/CD pipelines to speed up package downloads and reduce the load on the package mirrors. It caches only the packages that were requested by clients and stores them in a local directory to avoid re-downloading. It serves packages over HTTP and HTTPS and supports on-the-fly certificate generation. It also supports multiple package mirrors, HTTPS-passthrough, proxy support, SRV record support, and URL rewriting. It provides a web interface to view cache statistics and configuration details.

## What this progran not does

This program does not cache all packages from a package mirror, only the packages that were requested by clients. It does not cache packages that are not requested by clients, there is no pre-caching of packages.

## Why this package was made?

I was dissatisfied with the performance and quality of the apt-cacher-ng package. As under [Ubuntu Bug Tracker](https://bugs.launchpad.net/ubuntu/+source/apt-cacher-ng) and [Debian Bug report logs](https://bugs.debian.org/cgi-bin/pkgreport.cgi?pkg=apt-cacher-ng) apt-cacher-ng has some unresolved issues. Some of these have also affected me and my friends over the years and have repeatedly hindered my home lab and my company and have repeatedly disrupted normal system operation and CI builds.

That's why I created an alternative with this program that is more in line with my wishes and requirements.

## Configuration

The configuration file is structured in YAML format and contains the following example configuration:
```yaml
# The main cache directory where the packages and database are stored
cache_directory: "/var/cache/goaptcacher"

domains:
  - "archive.ubuntu.com" # Ubuntu archive
  - "security.ubuntu.com" # Ubuntu security
  - "ports.ubuntu.com" # Ubuntu ports
  - "esm.ubuntu.com" # Ubuntu ESM
  - "motd.ubuntu.com" # Ubuntu MOTD

  - "security.debian.org" # Debian security
  - ".debian.org" # Debian archive
  - "packages.microsoft.com" # Microsoft packages (e.g. Visual Studio Code)
  - "download.docker.com" # Docker packages
  - "dl.google.com" # Google packages (e.g. Chrome)
  - "packages.gitlab.com" # GitLab packages (GitLab CE/EE & GitLab Runner)
  - "packages.sury.org" # Ondřej Surý's PHP PPA
  - "ppa.launchpad.net" # Ubuntu PPAs
  - "ppa.launchpadcontent.net" # Ubuntu PPAs (content, redirect from ppa.launchpad.net)
  - "download.proxmox.com" # Proxmox VE
  - "apt.syncthing.net" # Syncthing
  - "syncthing-apt.svc.edge.scw.cloud" # Syncthing (Scaleway CDN for apt.syncthing.net)
  - "raspbian.raspberrypi.org" # Raspbian
  - "archive.raspberrypi.org" # Raspbian archive

passthrough_domains:
  - "esm.ubuntu.com" # Ubuntu ESM (authentication required)
  - "enterprise.proxmox.com" # Proxmox VE with subscription (authentication required)
  - "downloads.plex.tv" # Plex Media Server
  - "downloads.linux.hpe.com" # Hawlett Packard Enterprise (HPE) packages

https:
  intercept: true
  cert: "public.key"
  key: "private.key"
  password: "mysecret"

remap:
  - from: "ubuntu.lagis.at"
    to: "at.archive.ubuntu.com"

overrides:
  ubuntu_server: "mirror.netcologne.de"
  debian_server: "mirror.netcologne.de"

index:
  enable: true
  hostnames:
    - "cache.bella.pm"
    - "10.20.23.244"
  contact: "<a href='mailto:thomas@bella.network'>Thomas Bella</a>"

mdns: true

expiration:
  unused_days: 60
```
