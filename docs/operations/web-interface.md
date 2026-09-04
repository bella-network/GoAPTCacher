---
title: Web interface and API
description: Use the built-in overview, cache, statistics, setup, and JSON API endpoints.
outline: deep
---

# Web interface and API

GoAPTCacher exposes its operations interface below `/_goaptcacher/` on every configured listener. It has no authentication; restrict it with a firewall, reverse proxy, or trusted management network.

## Pages

| URL | Purpose |
| --- | --- |
| `/_goaptcacher/` | Version, listeners, HTTPS mode, domains, passthrough rules, and mirror routing |
| `/_goaptcacher/cache` | Cached object count, cached bytes, filesystem use, and retention policy |
| `/_goaptcacher/stats` | Lifetime totals, hit rate, traffic savings estimate, and 14-day daily breakdown |
| `/_goaptcacher/setup` | Generated client, discovery, DNS SRV, and GitLab CI examples |

The first `index.hostnames` value is used in generated setup examples. If none is available, GoAPTCacher chooses the first non-loopback IPv4 address and finally falls back to `127.0.0.1`.

The UI sends `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, and `X-Frame-Options: DENY`.

::: warning `index.enable` is not access control
The current server routes the web pages and API regardless of `index.enable`. Protect listener access at the network boundary.
:::

## JSON statistics API

```bash
curl -s http://cache.example.com:8090/_goaptcacher/api/stats
```

Example response:

```json
{
  "totals": {
    "requests": 120,
    "hits": 78,
    "misses": 32,
    "tunnel": 10,
    "traffic_down": 524288000,
    "traffic_up": 1258291200,
    "tunnel_transfer": 104857600
  },
  "daily": [
    {
      "date": "2026-09-04",
      "requests": 120,
      "hits": 78,
      "misses": 32,
      "tunnel": 10,
      "traffic_down": 524288000,
      "traffic_up": 1258291200,
      "tunnel_transfer": 104857600
    }
  ],
  "oldest_day": "2026-09-04"
}
```

Byte counters are unscaled integers. `daily` contains at most the 14 most recent recorded days in newest-first order. `totals` aggregates all persisted days, including days omitted from the detail list.

A tunnel records the combined bidirectional transfer in `tunnel_transfer`. The general traffic counters represent data fetched from upstream and data delivered by the proxy.

## Monitoring examples

Check reachability without querying `/`, whose `406` response is intentional:

```bash
curl --fail --silent --show-error \
  http://cache.example.com:8090/_goaptcacher/api/stats >/dev/null
```

Extract a simple hit rate with `jq`:

```bash
curl -s http://cache.example.com:8090/_goaptcacher/api/stats \
  | jq '.totals | if .requests == 0 then 0 else (.hits * 100 / .requests) end'
```

Also monitor:

- process and listener availability;
- free bytes and inodes on `cache_directory`;
- repeated `403`, refresh, download, and disk errors in logs;
- repository-verification warnings;
- a sustained drop in hits relative to normal workload.

## Certificate endpoints

| URL | Availability |
| --- | --- |
| `/_goaptcacher/goaptcacher.crt` | When HTTPS interception is enabled |
| `/_goaptcacher/revocation.crl` | When CRL generation is enabled and a file exists |

These routes expose public certificate material only. Verify the CA fingerprint over a trusted channel before importing it.

## Standard endpoints

`/robots.txt` disallows indexing. `/.well-known/security.txt` points to the GitLab project and receives a seven-day expiry timestamp. `/favicon.ico` serves the embedded icon.

A request for `/_goaptcacher` redirects temporarily to `/_goaptcacher/`. A request for `/` returns `406 Not Acceptable` with a browser redirect hint for compatibility with `auto-apt-proxy`.
