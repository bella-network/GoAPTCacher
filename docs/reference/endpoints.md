---
title: HTTP endpoint reference
description: Reference for proxy behavior, web routes, API, certificate files, and debug endpoints.
outline: deep
---

# HTTP endpoint reference

Every configured listener uses one handler. Proxy requests and the operations interface therefore share the same address and network security boundary.

## Proxy methods

| Method | Behavior |
| --- | --- |
| `GET` | Serve from cache or stream a successful origin response into the cache. |
| `HEAD` | Return local headers, or download the entire missing object before returning headers. |
| `CONNECT` | Reject, create an end-to-end TCP tunnel, or intercept TLS according to configuration. |
| Other | Return `405 Method Not Allowed` for normal proxy targets. |

Target hosts outside both configured domain lists receive `403 Forbidden`. Invalid cacheable requests receive `400 Bad Request`.

## Operations pages and API

| Path | Content type | Conditions | Purpose |
| --- | --- | --- | --- |
| `/_goaptcacher/` | HTML | always routed | Instance overview |
| `/_goaptcacher/cache` | HTML | always routed | Cache and filesystem summary |
| `/_goaptcacher/stats` | HTML | always routed | Lifetime and 14-day statistics |
| `/_goaptcacher/setup` | HTML | always routed | Generated client setup examples |
| `/_goaptcacher/api/stats` | JSON | always routed | Machine-readable statistics |
| `/_goaptcacher/style.css` | CSS | always routed | Embedded UI stylesheet |
| `/_goaptcacher/favicon.ico` | icon | always routed | Embedded UI icon |

`index.enable` does not currently disable these routes. There is no authentication or authorization layer.

## Certificate endpoints

| Path | Condition | Result when unavailable |
| --- | --- | --- |
| `/_goaptcacher/goaptcacher.crt` | `https.intercept: true` | `404 HTTPS interception not enabled` |
| `/_goaptcacher/revocation.crl` | `https.enable_crl: true` and CRL file exists | `404 CRL not enabled` or file-not-found response |

The certificate route serves the file configured by `https.cert`. The CRL route serves `<cache_directory>/crl.pem`.

## Debug endpoints

When `debug.enable: true`:

| Path | Purpose |
| --- | --- |
| `/_goaptcacher/debug` | Build, runtime, pprof configuration, and memory JSON |
| `/_goaptcacher/debug/pprof/` | Profile index |
| `/_goaptcacher/debug/pprof/cmdline` | Process command line |
| `/_goaptcacher/debug/pprof/profile` | CPU profile |
| `/_goaptcacher/debug/pprof/symbol` | Program-counter lookup |
| `/_goaptcacher/debug/pprof/trace` | Runtime trace |
| `/_goaptcacher/debug/pprof/<name>` | Named Go runtime profile such as `heap` or `goroutine` |

With `debug.allow_remote: false`, non-loopback source addresses receive `403 Forbidden`. These routes are present independently of `debug.pprof.enable`, which controls periodic files only.

## Root and standard paths

| Path | Status | Purpose |
| --- | --- | --- |
| `/` | `406 Not Acceptable` | Compatibility signal for `auto-apt-proxy`, with browser redirect hint |
| `/_goaptcacher` | `307 Temporary Redirect` | Canonical slash redirect |
| `/favicon.ico` | `200 OK` | Embedded icon |
| `/robots.txt` | `200 OK` | Disallow all crawlers |
| `/.well-known/security.txt` | `200 OK` | GitLab security contact and dynamic seven-day expiry |

The `Location` header for `/` is set after the response status is written in the current implementation, so clients should use the HTML hint or request `/_goaptcacher/` directly rather than relying on an HTTP redirect.

## Unknown internal route

A path below `/_goaptcacher/` that is not recognized returns a rendered `404` page. An unknown normal proxy target is processed according to domain policy and method rather than treated as a local route.

## Health check

Use the JSON API:

```bash
curl --fail --silent --show-error \
  http://127.0.0.1:8090/_goaptcacher/api/stats >/dev/null
```

Do not use `/` with a health checker that expects `2xx`.
