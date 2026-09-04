---
title: Configuration reference
description: Complete reference for every GoAPTCacher YAML configuration option and default.
outline: deep
---

# Configuration reference

GoAPTCacher parses YAML at startup. Unknown YAML keys are currently ignored, so spelling mistakes may silently leave a field at its zero value. Compare changes with this reference and inspect startup logs after every restart.

## Complete example

```yaml
cache_directory: "/var/cache/goaptcacher"
listen_port: 8090
listen_port_secure: 8091
alternative_ports:
  - 3142

domains:
  - "archive.ubuntu.com"
  - "security.ubuntu.com"
  - ".debian.org"

passthrough_domains:
  - "esm.ubuntu.com"

overrides:
  ubuntu_server: "archive.ubuntu.com"
  debian_server: "deb.debian.org"

remap:
  - from: "/legacy/dists/stable/InRelease"
    to: "/debian/dists/stable/InRelease"

https:
  prevent: false
  intercept: false
  cert: "/etc/goaptcacher/intermediate-ca.crt"
  key: "/etc/goaptcacher/intermediate-ca.key"
  password: ""
  certificate_domain: "cache.example.com"
  aia_address: "http://cache.example.com:8090/_goaptcacher/goaptcacher.crt"
  enable_crl: false

index:
  enable: true
  hostnames:
    - "cache.example.com"
  contact: "<a href=\"mailto:infra@example.com\">Infrastructure team</a>"

mdns: false

expiration:
  unused_days: 90

debug:
  enable: false
  allow_remote: false
  log_interval_seconds: 60
  pprof:
    enable: false
    directory: "/var/cache/goaptcacher/pprof"
    interval_seconds: 60
    retain: 48
```

Do not configure interception key fields unless interception is enabled. Do not copy example domains without confirming the repositories your clients actually use.

## Top-level options

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `cache_directory` | string | `./cache` | Root for cached objects, sidecars, statistics, CRL, and default pprof directory. `CACHE_DIR` overrides it. |
| `listen_port` | integer | `8090` | Primary HTTP proxy and operations listener. |
| `listen_port_secure` | integer | `8091` when interception starts | Direct TLS listener used only with HTTPS interception. |
| `alternative_ports` | integer list | empty | Additional HTTP listeners with identical behavior; `3142` is conventional for apt-cacher compatibility. |
| `domains` | string list | empty | Allowed hostname suffixes eligible for caching. |
| `passthrough_domains` | string list | empty | Allowed hostname suffixes that bypass caching and interception. |
| `mdns` | boolean | `false` | Announce the APT proxy service on the local multicast domain. |

If both domain lists are empty, all hosts are accepted but cacheable methods bypass storage. Passthrough wins when a host matches both lists.

## `index`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `index.enable` | boolean | `false` | Declares the overview feature enabled and changes startup messaging. Current routes remain available regardless of this value. |
| `index.hostnames` | string list | empty | Hostnames shown in UI-generated proxy and DNS examples. The first non-empty value is preferred. |
| `index.contact` | string | empty | Trusted administrator-supplied HTML rendered in the web interface footer. |

## `overrides`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `overrides.ubuntu_server` | string | empty | Replacement host, optionally with path prefix, for hosts ending in `archive.ubuntu.com`. Do not include a scheme. |
| `overrides.debian_server` | string | empty | Replacement host, optionally with path prefix, for Debian FTP mirrors and supported `deb.debian.org` paths. Do not include a scheme. |

See [Domain and mirror routing](/features/domain-routing) for exact host/path behavior and safe cache migration.

## `remap`

`remap` is a list of objects:

| Key | Type | Required | Description |
| --- | --- | --- | --- |
| `remap[].from` | string | yes | Complete request path to match exactly. |
| `remap[].to` | string | yes | Replacement path. |

The current implementation compares `r.URL.Path`, not hostname or full URL, and does not perform prefix replacement.

## `https`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `https.prevent` | boolean | `false` | Reject allowed `CONNECT` requests with `403`. |
| `https.intercept` | boolean | `false` | Terminate client TLS, generate per-host certificates, and run normal cache logic for HTTPS. |
| `https.cert` | string | empty | PEM CA certificate used to sign generated leaf certificates; required for interception. |
| `https.key` | string | empty | PEM RSA or ECDSA private key matching `cert`; required for interception. |
| `https.password` | string | empty | Password for a supported encrypted PEM key. |
| `https.certificate_domain` | string | empty | Fallback certificate domain and basis for automatically derived AIA/CRL URLs. |
| `https.aia_address` | string | derived only when `certificate_domain` is set | Explicit AIA URL embedded in generated leaf certificates. |
| `https.enable_crl` | boolean | `false` | Generate and serve a CRL. Generation requires `certificate_domain`. |

Interception starts both CONNECT interception on HTTP listeners and the direct TLS listener. `prevent` takes precedence for CONNECT; avoid enabling both settings.

## `expiration`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `expiration.unused_days` | unsigned integer | `0` | Delete cached objects whose recorded last use is older than this many days. `0` disables expiration. |

The cleanup loop begins five seconds after startup and then runs every 12 hours.

## `debug`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `debug.enable` | boolean | `false` | Enable periodic runtime logging plus debug JSON and live pprof HTTP routes. |
| `debug.allow_remote` | boolean | `false` | Permit debug HTTP routes from non-loopback source addresses. |
| `debug.log_interval_seconds` | integer | `60` when debug is enabled | Interval for runtime/memory log lines. |
| `debug.pprof.enable` | boolean | `false` | Write periodic heap and goroutine profiles to disk. |
| `debug.pprof.directory` | string | `<cache_directory>/pprof` when pprof is enabled | Profile output directory. |
| `debug.pprof.interval_seconds` | integer | `60` when pprof is enabled | File capture interval. |
| `debug.pprof.retain` | integer | `0` | Number of `.pprof` files to retain; `0` retains all. Two files are created per interval. |

All pprof defaults are applied only when both debug and periodic pprof are enabled. Live pprof HTTP handlers require only `debug.enable`.

## Configuration path and overrides

| Source | Precedence | Meaning |
| --- | --- | --- |
| `--config PATH` or `-c PATH` | highest | YAML file path |
| `CONFIG` | fallback | YAML file path |
| `./config.yaml` | default | YAML file path relative to the working directory |
| `CACHE_DIR` | field override | Replaces `cache_directory` after YAML parsing |

The package service sets `CONFIG=/etc/goaptcacher/config.yaml` and uses `/etc/goaptcacher` as its working directory. The container works in `/config`.

## Reload behavior

No live reload or signal-based reload is implemented. Restart the process after every change:

```bash
sudo systemctl restart goaptcacher
sudo journalctl -u goaptcacher -n 100 --no-pager
```

Startup fails for unreadable or syntactically invalid YAML and for unusable interception key material. Other invalid combinations may fail only when a listener or feature starts, so test changes before rollout.
