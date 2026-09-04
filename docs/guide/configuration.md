---
title: Configuration
description: Plan and maintain a GoAPTCacher YAML configuration.
outline: deep
---

# Configuration

GoAPTCacher reads one YAML file at startup. The command-line `--config` path takes precedence over `CONFIG`; if neither is set, the default is `./config.yaml`. `CACHE_DIR` can override the configured cache directory.

Configuration changes are not reloaded automatically. Restart the process after editing the file.

## Recommended baseline

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

https:
  prevent: false
  intercept: false

index:
  enable: true
  hostnames:
    - "cache.example.com"
  contact: "Contact the infrastructure team for support."

mdns: false

expiration:
  unused_days: 90

debug:
  enable: false
```

## Domain policy first

`domains` identifies allowed cache targets. `passthrough_domains` identifies allowed targets that always bypass caching and HTTPS interception. If a hostname appears in both, passthrough wins.

If both lists are empty, every hostname is allowed, `GET`/`HEAD` traffic bypasses the cache, and `CONNECT` traffic is tunneled unless HTTPS is blocked. This is intended as a fallback, not a safe production policy.

Entries are matched as hostname suffixes. Use the narrowest required values and review the implications in [Domain and mirror routing](/features/domain-routing).

## Choose an HTTPS mode

```yaml
https:
  prevent: false
  intercept: false
```

This default tunnels HTTPS end to end. Setting `prevent: true` rejects `CONNECT`; setting `intercept: true` enables caching through TLS interception and requires a CA certificate and private key. Do not enable interception until clients trust the CA and the operational risks are understood. See [HTTPS modes](/concepts/https-modes).

## Configure the web interface

```yaml
index:
  enable: true
  hostnames:
    - "cache.example.com"
  contact: "<a href=\"mailto:infra@example.com\">Infrastructure team</a>"
```

The first hostname is used in generated client examples. `contact` is rendered as HTML, so only administrators who already control the configuration should edit it.

The current server exposes `/_goaptcacher/` routes regardless of `index.enable`; use network policy to restrict access. The option controls the intended presentation and startup messaging, not authentication.

## Cache retention

```yaml
expiration:
  unused_days: 90
```

A non-zero value starts a background cleanup loop. Files whose recorded last-access time is older than the cutoff are removed every 12 hours after an initial five-second delay. `0` disables automatic expiration.

## Mirror routing

Ubuntu and Debian overrides consolidate multiple official mirror hostnames onto a selected mirror. The general `remap` list performs exact request-path substitutions. Review cache migration and examples in [Domain and mirror routing](/features/domain-routing).

## Debugging

Keep debug features disabled until needed:

```yaml
debug:
  enable: true
  allow_remote: false
  log_interval_seconds: 60
  pprof:
    enable: false
    directory: "/var/cache/goaptcacher/pprof"
    interval_seconds: 60
    retain: 24
```

With `allow_remote: false`, debug HTTP routes accept only loopback source addresses. Periodic profiles are written to disk independently of HTTP access, so protect and monitor the configured directory.

## Apply and validate

```bash
sudo systemctl restart goaptcacher
sudo systemctl status goaptcacher
sudo journalctl -u goaptcacher -n 100 --no-pager
curl -i http://127.0.0.1:8090/_goaptcacher/
```

GoAPTCacher fails startup for unreadable or invalid YAML, and for missing or unusable interception key material. It does not currently provide a separate configuration-validation subcommand.

See the [Configuration reference](/reference/configuration) for every key and default.
