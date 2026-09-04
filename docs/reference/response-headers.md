---
title: Response header reference
description: Interpret GoAPTCacher cache headers and understand request and response forwarding.
outline: deep
---

# Response header reference

## `X-Cache`

| Value | Meaning |
| --- | --- |
| `HIT` | The response was served from a completed local object. |
| `MISS` | A successful origin response was streamed to the client while populating the cache, or a `HEAD` miss downloaded the object first. |

Tunnel traffic does not carry an application-level `X-Cache` header because the proxy does not inspect the encrypted HTTP exchange.

Test an HTTP repository object:

```bash
curl -I -x http://cache.example.com:8090 \
  http://archive.ubuntu.com/ubuntu/dists/noble/InRelease
```

Run it twice. The first result should be `MISS` when no local object exists; a later result should be `HIT`.

## `X-Proxy-Server`

Cacheable `GET` responses include:

```text
X-Proxy-Server: GoAptCacher/<version>
```

The corresponding origin request uses a more descriptive `X-Proxy-Server` value with the project URL. This identifies the intermediary to both client and repository.

`HEAD` responses currently do not add this header.

## Hit response headers

A local `GET` or `HEAD` response sets:

```text
X-Cache: HIT
Content-Length: <local file size>
Content-Type: application/octet-stream
Last-Modified: <local file modification time>
```

`GET` uses Go's file-serving behavior, so range and conditional response handling may add or alter standard headers and status codes. Repository-specific origin headers not persisted in sidecar metadata are unavailable on a later hit.

## Miss response headers

A `GET` miss copies end-to-end origin response headers, adds `X-Cache: MISS`, and normalizes usable `Last-Modified` and `ETag` values. It strips hop-by-hop fields:

- `Connection`;
- `Proxy-Connection`;
- `Keep-Alive`;
- `Proxy-Authenticate`;
- `Proxy-Authorization`;
- `TE`;
- `Trailer`;
- `Transfer-Encoding`;
- `Upgrade`.

Only origin `200 OK` responses enter the cache. Other origin statuses are returned to the client as a generic `404` fetch error by the current implementation.

A `HEAD` miss downloads the object but returns locally constructed size, type, modification, and cache headers rather than the original header set.

## Forwarded origin request

On a cache miss, most client request headers are copied to the origin. Hop-by-hop headers and client validators `If-Modified-Since`, `If-None-Match`, and `E-Tag` are removed so the proxy receives a complete cacheable response.

GoAPTCacher then sets:

```text
X-Forwarded-For: <client remote address>
X-Proxy-Server: GoAptCacher/<version> (+https://gitlab.com/bella.network/goaptcacher)
```

The current `X-Forwarded-For` value replaces any client-supplied value and can include the source port from the accepted connection.

## Refresh request headers

Conditional background or pre-serve refreshes set:

```text
User-Agent: GoAptCacher/<version> (+https://gitlab.com/bella.network/goaptcacher)
X-ACTION: refresh
If-None-Match: <recorded ETag>             # when available
If-Modified-Since: <recorded timestamp>    # when available
X-SHA256: <recorded local SHA-256>         # when available
```

`X-SHA256` is a GoAPTCacher-specific validator; ordinary HTTP origins may ignore it.

## Operations response headers

Routes below `/_goaptcacher/` set:

```text
Cache-Control: no-store, no-cache, must-revalidate
Pragma: no-cache
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
```

Use these headers for diagnostics, not as an authentication mechanism. Access control must be enforced outside GoAPTCacher.
