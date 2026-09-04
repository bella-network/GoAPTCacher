---
title: Getting started
description: Install GoAPTCacher, connect an APT client, and confirm that caching works.
outline: deep
---

# Getting started

This guide installs GoAPTCacher from the bella.network APT repository, applies a conservative domain policy, and connects one Debian or Ubuntu client.

## Prerequisites

You need:

- a Debian or Ubuntu host with `amd64` or `arm64` architecture;
- enough persistent disk space for the package cache;
- network access from clients to the proxy and from the proxy to repository origins;
- a stable IP address or internal DNS name such as `cache.example.com`.

Use a host on a trusted network. GoAPTCacher listens on all interfaces and does not provide client authentication.

## 1. Install the package

```bash
curl -fsSL https://repo.bella.network/_static/bella-archive-keyring.gpg \
  | sudo tee /usr/share/keyrings/bella-archive-keyring.gpg >/dev/null

sudo tee /etc/apt/sources.list.d/repo.bella.network.sources >/dev/null <<'EOF'
Types: deb
URIs: https://repo.bella.network/deb
Suites: stable
Components: main
Architectures: amd64 arm64
Signed-By: /usr/share/keyrings/bella-archive-keyring.gpg
EOF

sudo apt update
sudo apt install goaptcacher
```

The package installs the executable, configuration, systemd service, and repository-verification timer. See [Installation](/guide/installation) for all paths and source builds.

## 2. Configure the proxy

Edit `/etc/goaptcacher/config.yaml`:

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
  - ".debian.org"

passthrough_domains:
  - "esm.ubuntu.com"
  - "enterprise.proxmox.com"

https:
  prevent: false
  intercept: false

index:
  enable: true
  hostnames:
    - "cache.example.com"

expiration:
  unused_days: 90
```

Add every repository hostname used by your clients. Entries in `domains` are eligible for caching; entries in `passthrough_domains` are tunneled and never cached. Do not leave both lists empty in production.

The default HTTPS tunnel mode preserves end-to-end TLS but cannot cache HTTPS content. Read [HTTPS modes](/concepts/https-modes) before considering interception.

## 3. Start the service

```bash
sudo systemctl enable --now goaptcacher
sudo systemctl status goaptcacher
sudo journalctl -u goaptcacher -n 100 --no-pager
```

Confirm that the proxy is reachable:

```bash
curl -i http://cache.example.com:8090/_goaptcacher/
```

The `/` path intentionally returns `406 Not Acceptable`; use `/_goaptcacher/` for the web interface.

## 4. Connect a client

Create `/etc/apt/apt.conf.d/10proxy` on the client:

```text
Acquire::http::Proxy "http://cache.example.com:8090/";
Acquire::https::Proxy "http://cache.example.com:8090/";
```

Although the second line configures an HTTP proxy URL, APT uses the HTTP `CONNECT` method to reach HTTPS repositories through it.

Verify the effective configuration and update package indexes:

```bash
apt-config dump | grep -E 'Acquire::(http|https)::Proxy'
sudo apt update
```

See [Client configuration](/guide/client-configuration) for per-host exceptions, CI jobs, and automatic discovery.

## 5. Confirm caching

Run `sudo apt update` or install the same package from two clients. Then open:

```text
http://cache.example.com:8090/_goaptcacher/stats
```

Cached HTTP responses carry `X-Cache: HIT`; the first successful download carries `X-Cache: MISS`. HTTPS requests in tunnel mode appear as tunnel traffic and are not cache hits.

You can also inspect JSON statistics:

```bash
curl -s http://cache.example.com:8090/_goaptcacher/api/stats
```

## Production checklist

- [ ] `domains` contains only required repository suffixes.
- [ ] Authentication and certificate-pinned origins are in `passthrough_domains`.
- [ ] Client access is restricted by firewall or network policy.
- [ ] The cache directory is persistent, writable only by the service, and monitored for capacity.
- [ ] HTTPS interception is disabled unless there is an explicit requirement and managed CA lifecycle.
- [ ] The web interface and debug endpoints are not exposed to untrusted networks.
- [ ] Cache expiration and repository verification are enabled and monitored.
- [ ] A second client or repeated request produces a cache hit.

## Next steps

Read [Architecture](/concepts/architecture) and [Cache lifecycle](/concepts/cache-lifecycle), then choose an [example deployment](/examples/home-lab) closest to your environment.
