---
title: Cache lifecycle
description: Learn how cache misses, hits, refreshes, expiration, metadata, and statistics behave.
outline: deep
---

# Cache lifecycle

GoAPTCacher keeps immutable packages for reuse while periodically checking mutable repository metadata. Cache state survives restarts through content files, `.access.json` sidecars, and `.stats.json`.

## Request outcomes

| Request | Local state | Behavior |
| --- | --- | --- |
| `GET` | valid object present | Serve from disk with `X-Cache: HIT` |
| `GET` | object absent | Stream origin `200` to client and temporary file with `X-Cache: MISS` |
| `HEAD` | object present | Return local size and modification headers with `X-Cache: HIT` |
| `HEAD` | object absent | Download the entire object, then return headers with `X-Cache: MISS` |
| passthrough or `CONNECT` tunnel | any | Relay bytes without cache storage |

A `HEAD` miss intentionally fills the cache; account for that when using monitoring probes against large artifacts.

## Refresh schedule

Refresh checks depend on the URL and filename:

| Object category | Recheck interval |
| --- | --- |
| Repository metadata below `/dists/` named `InRelease`, `Release`, `Release.gpg`, `Packages`, `Packages.gz`, `Packages.bz2`, `Packages.xz`, `Sources`, `Sources.gz`, or `Index` | 5 minutes |
| Paths containing `/pool/` | 7 days |
| Paths containing `/by-hash/` | 7 days |
| Other cached objects | 24 hours |

Frequently changing metadata below `/dists/` is refreshed synchronously before a hit is served when its check time is stale. Other eligible refresh work is started after serving the local response.

## Conditional origin checks

Refresh requests use available validators:

- `If-None-Match` from the recorded ETag;
- `If-Modified-Since` from the recorded remote timestamp;
- `X-SHA256` as a GoAPTCacher-specific validator for supporting origins.

An origin `304 Not Modified` updates the last-check time without replacing content. A changed `200 OK` response is downloaded to a temporary file and swapped into place atomically. A `404 Not Found` records a deletion marker in the sidecar; it does not synchronously remove the cached object.

When `InRelease`, `Release`, or `Release.gpg` changes, GoAPTCacher also attempts to refresh related metadata entries that already exist in the cache.

## Access metadata

A sidecar named `<object>.access.json` records:

- HTTP/HTTPS protocol identifier, domain, path, and source URL;
- last access, last origin check, and remote modification time;
- ETag, object size, and locally calculated SHA-256;
- whether and when an object was marked for deletion.

Sidecars are loaded on demand and dirty records are flushed approximately every 30 seconds. Do not edit them while the service is running.

## Statistics

Daily counters track requests, hits, misses, tunnel requests, upstream bytes, bytes served, and tunnel transfer. They are flushed to `.stats.json` approximately every 30 seconds and loaded on startup.

The web page shows lifetime totals and the 14 most recent recorded days. The JSON endpoint uses the same 14-day detail window while its `totals` cover all persisted daily entries.

## Automatic expiration

Set a non-zero retention window:

```yaml
expiration:
  unused_days: 90
```

The first scan begins shortly after startup and repeats every 12 hours. Objects whose last-access timestamp is older than the cutoff are deleted together with their metadata entry. A value of `0` leaves old objects indefinitely.

Expiration is based on use, not repository publication date. A frequently reused package remains cached even if it is old.

## Interrupted downloads

Cache misses are written to uniquely named `.partial` files and become visible as cache entries only after a complete successful transfer. A failed request removes its temporary file when normal cleanup runs. After an unclean process termination, review and remove stale `*.partial` files while the service is stopped.

## Capacity behavior

When an origin supplies `Content-Length`, GoAPTCacher checks free space and preallocates the target size. Insufficient capacity returns HTTP `507 Insufficient Storage`. Large downloads also ask Linux to release completed write ranges from the page cache after 128 MiB, reducing pressure on memory.

Monitor the filesystem rather than relying only on configured expiration. A burst of new immutable packages can consume space before they become eligible for deletion.
