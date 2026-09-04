---
title: Logging and debugging
description: Interpret GoAPTCacher logs and use JSON diagnostics or pprof safely.
outline: deep
---

# Logging and debugging

GoAPTCacher writes structured-prefix text logs to standard output. Under systemd, inspect them with `journalctl`; in containers, use the runtime log command.

## Routine logging

```bash
sudo journalctl -u goaptcacher -f
docker logs --follow goaptcacher
```

Common prefixes include:

| Prefix | Meaning |
| --- | --- |
| `[INFO:GET:HIT]` | Object served from local cache |
| `[INFO:DL:CREATED]` | Cache miss completed and stored |
| `[INFO:TUNNEL]` | End-to-end tunnel activity |
| `[INFO:REFRESH:*]` | Conditional metadata or object refresh |
| `[INFO:EXPIRE]` | Retention scan and deletion activity |
| `[INFO:403]` | Host denied by policy or HTTPS blocked |
| `[ERROR:GET:*]` | Origin, disk, or cache-miss failure |
| `[DEBREPOCLEANER-*]` | Repository verification result |

Under systemd, timestamp and source-file flags are omitted because the journal already supplies metadata. Direct process execution includes standard timestamps and short source locations.

## Periodic runtime metrics

Enable debug logging:

```yaml
debug:
  enable: true
  allow_remote: false
  log_interval_seconds: 60
```

A memory/runtime line is written immediately and at the selected interval. It includes goroutine count, heap state, total Go runtime memory, garbage-collection count, and accumulated GC pause time.

Set a reasonable interval; very small values create unnecessary log volume. Restart the process after configuration changes.

## Debug JSON

With `debug.enable: true`:

```bash
curl -s http://127.0.0.1:8090/_goaptcacher/debug | jq
```

The response contains build information, Go version, goroutine and `GOMAXPROCS` values, pprof settings, and detailed memory counters.

When `allow_remote: false`, only requests whose TCP source address is loopback are accepted. A request forwarded by a reverse proxy normally appears to come from the proxy address and may therefore be rejected unless the reverse proxy itself connects over loopback.

## Live pprof endpoints

GoAPTCacher exposes the standard Go profiles below:

```text
/_goaptcacher/debug/pprof/
/_goaptcacher/debug/pprof/heap
/_goaptcacher/debug/pprof/goroutine
/_goaptcacher/debug/pprof/profile
/_goaptcacher/debug/pprof/trace
/_goaptcacher/debug/pprof/cmdline
/_goaptcacher/debug/pprof/symbol
```

Example CPU capture from the same host:

```bash
go tool pprof \
  http://127.0.0.1:8090/_goaptcacher/debug/pprof/profile?seconds=30
```

Live pprof routes are available whenever debug mode is enabled; `debug.pprof.enable` controls periodic files, not the HTTP handler.

::: danger Do not expose pprof publicly
Profiles can reveal request patterns, paths, memory contents, and operational details. Keep `allow_remote: false` and use local access or a protected tunnel.
:::

## Periodic profile files

```yaml
debug:
  enable: true
  allow_remote: false
  pprof:
    enable: true
    directory: "/var/cache/goaptcacher/pprof"
    interval_seconds: 300
    retain: 48
```

Each interval writes one heap profile and one goroutine profile with a UTC timestamp. Files are first written with `.tmp` and atomically renamed. The current retention cleanup counts individual `.pprof` files, so `retain: 48` keeps approximately 24 two-file capture rounds. `0` keeps all files.

When no directory is configured, the default is `<cache_directory>/pprof`. Monitor its size, especially when retention is disabled.

## Useful captures

```bash
curl -s http://127.0.0.1:8090/_goaptcacher/debug > debug.json
sudo journalctl -u goaptcacher --since '-30 min' > goaptcacher.log
goaptcacher --version
```

Before sharing logs or profiles, review them for internal hostnames, repository paths, addresses, and sensitive traffic metadata.
