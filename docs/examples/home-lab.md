---
title: Home lab example
description: Deploy one GoAPTCacher instance for Debian, Ubuntu, Docker, and Proxmox hosts on a trusted LAN.
outline: deep
---

# Home lab example

This example serves a trusted LAN at `192.0.2.0/24`. The cache host is `apt-cache.home.example` and HTTPS remains end-to-end encrypted.

## Server configuration

```yaml
cache_directory: "/var/cache/goaptcacher"
listen_port: 8090
alternative_ports:
  - 3142

domains:
  - "archive.ubuntu.com"
  - "security.ubuntu.com"
  - "ports.ubuntu.com"
  - "deb.debian.org"
  - "security.debian.org"
  - "download.docker.com"
  - "download.proxmox.com"
  - "repo.bella.network"

passthrough_domains:
  - "enterprise.proxmox.com"
  - "esm.ubuntu.com"

https:
  prevent: false
  intercept: false

overrides:
  ubuntu_server: "archive.ubuntu.com"
  debian_server: "deb.debian.org"

index:
  enable: true
  hostnames:
    - "apt-cache.home.example"
  contact: "Home lab administrator"

mdns: false
expiration:
  unused_days: 120

debug:
  enable: false
```

HTTP repository traffic is cached. HTTPS repository traffic is tunneled and counted but not cached. Authenticated Proxmox and Ubuntu ESM origins are explicitly passthrough even if interception is enabled later.

## Network policy

Permit TCP `8090` and optionally `3142` only from the LAN. Permit management access to the web interface from administrator devices; because it shares the proxy listener, a reverse proxy or host firewall is needed for finer separation.

Allow the cache host outbound TCP `80` and `443` plus DNS. Restrict forwarding from guest and untrusted VLANs.

## Client configuration

On persistent hosts, create `/etc/apt/apt.conf.d/10proxy`:

```text
Acquire::http::Proxy "http://apt-cache.home.example:8090/";
Acquire::https::Proxy "http://apt-cache.home.example:8090/";
```

For laptops that should use the cache only at home, install `auto-apt-proxy` and publish:

```text
_apt_proxy._tcp.home.example. 3600 IN SRV 0 0 8090 apt-cache.home.example.
```

## Validation

On two clients:

```bash
sudo apt clean
sudo apt update
sudo apt install --reinstall hello
```

Inspect:

```bash
curl -s http://apt-cache.home.example:8090/_goaptcacher/api/stats | jq
sudo journalctl -u goaptcacher --since '-10 min'
```

Expect HTTPS-only repositories such as Docker to appear as tunnel traffic in this configuration.

## Operational routine

- Review cache growth and hit rate monthly.
- Keep at least one normal upgrade cycle inside `unused_days`.
- Monitor `goaptcacher-repoverify.timer` and its journal.
- Upgrade from tagged releases.
- Keep a protected backup of the YAML configuration.
- Test direct APT access as a documented emergency fallback.
