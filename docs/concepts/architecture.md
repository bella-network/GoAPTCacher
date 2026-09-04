---
title: Architecture
description: Understand request routing, caching, storage, and auxiliary services in GoAPTCacher.
outline: deep
---

# Architecture

GoAPTCacher combines an explicit HTTP proxy, a filesystem cache, policy-based routing, and a small operations interface in one process.

```text
                            ┌──────────────────────────────┐
APT client ────────────────▶│ HTTP listener :8090         │
APT client / DNS SRV ──────▶│ alternative listeners       │
Direct TLS / DNS SRV ──────▶│ HTTPS listener :8091        │ interception only
                            └──────────────┬───────────────┘
                                           │
                                  internal route?
                                   │ yes       │ no
                                   ▼           ▼
                           Web UI / API   domain policy
                                           │
                                  CONNECT / GET / HEAD
                                  │                 │
                           tunnel/intercept    cache lookup
                                  │                 │
                                  └────────┬────────┘
                                           ▼
                                  repository origin
```

## Request pipeline

1. Requests below `/_goaptcacher/` are handled internally. The root path has special behavior for `auto-apt-proxy` detection.
2. The original request hostname is matched against `domains` and `passthrough_domains`.
3. Unsupported methods receive `405 Method Not Allowed`; supported methods are `GET`, `HEAD`, and `CONNECT`.
4. `CONNECT` is blocked, tunneled, or intercepted according to the HTTPS policy.
5. Cacheable `GET` and `HEAD` requests receive configured path or distribution overrides.
6. The filesystem cache serves a local object or retrieves it from the origin.

Domain authorization occurs before administrator-configured mirror overrides. Passthrough takes precedence over caching when a host matches both lists.

## Listeners

The primary HTTP listener defaults to port `8090`. Every `alternative_ports` value starts another equivalent HTTP listener; port `3142` is commonly used for apt-cacher compatibility and discovery fallback.

The TLS listener starts only when `https.intercept` is enabled and defaults to `8091`. It dynamically serves certificates from the configured CA.

All listeners bind to all interfaces. Network access control is therefore an external responsibility.

## Filesystem cache

The default cache key maps the normalized hostname and URL path below `cache_directory`:

```text
/var/cache/goaptcacher/
├── archive.ubuntu.com/
│   ├── dists/noble/InRelease
│   ├── dists/noble/InRelease.access.json
│   └── pool/main/e/example/example_1.0_amd64.deb
├── security.debian.org/
├── .stats.json
└── pprof/                         # only when configured
```

Each cached object may have a neighboring `.access.json` sidecar with URL, access/check timestamps, size, ETag, remote modification time, SHA-256, and deletion state. Statistics are stored separately in `.stats.json`.

The URL query string is not part of the local path. GoAPTCacher is designed for APT repositories whose immutable artifacts and metadata are identified by hostname and path, not for general dynamic web caching.

## Cache miss

For a successful origin response, GoAPTCacher:

1. checks available disk space when `Content-Length` is known;
2. creates and preallocates a temporary `.partial` file;
3. streams the response simultaneously to the client, cache file, and SHA-256 calculator;
4. validates the transferred length when known;
5. atomically renames the completed file into place;
6. records sidecar metadata and statistics.

Only an upstream `200 OK` response becomes a cache entry. Concurrent misses for the same URL are serialized with a write lock; waiting requests retry for roughly 25 seconds before failing.

## Cache hit

A hit is served directly from disk. Size metadata is checked before serving; a missing or differently sized file is treated as stale and fetched again. `/pool/` packages have a fast path because they are normally immutable.

Mutable repository metadata can be conditionally refreshed before or after it is served. See [Cache lifecycle](/concepts/cache-lifecycle).

## Auxiliary services

The same process provides:

- an overview, setup guide, cache page, statistics page, and JSON statistics API;
- optional mDNS announcement for `_apt_proxy._tcp.local`;
- optional debug JSON and pprof endpoints;
- periodic cache expiration.

The `verify-repos` command is a separate execution mode. Package installations schedule it through a systemd timer; container deployments must schedule it externally.
