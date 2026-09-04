---
title: Cache management
description: Size, inspect, retain, back up, migrate, and safely reset a GoAPTCacher cache.
outline: deep
---

# Cache management

The cache directory contains downloaded repository objects, sidecar metadata, persistent traffic statistics, optional CRL data, and optional pprof snapshots. Treat it as application state rather than an opaque download directory.

## Size the filesystem

Capacity depends on the number of distributions, architectures, third-party repositories, CI concurrency, and retention period. Begin with a dedicated filesystem and observe real growth.

```bash
du -sh /var/cache/goaptcacher
df -h /var/cache/goaptcacher
df -i /var/cache/goaptcacher
```

The built-in cache page shows object count, object bytes, and overall filesystem use:

```text
http://cache.example.com:8090/_goaptcacher/cache
```

Set external alerts for free space and inodes. The proxy rejects a known-size miss with `507 Insufficient Storage` when the object does not fit, but it cannot reserve capacity for every concurrent or unknown-length response in advance.

## Retention

```yaml
expiration:
  unused_days: 90
```

A non-zero value deletes objects based on their recorded last access. The scan runs shortly after startup and every 12 hours. Choose a window long enough to retain packages shared by periodic rebuilds but short enough to bound storage use.

Setting `unused_days: 0` disables automatic expiration. It does not immediately delete anything when changed from a non-zero value; restart the service to apply the configuration.

## Inspect the layout

```bash
find /var/cache/goaptcacher -maxdepth 3 -type f | head
```

Normal state includes:

- repository objects under `<hostname>/<URL path>`;
- adjacent `*.access.json` sidecars;
- `.stats.json` and temporary `.stats.json.tmp` during persistence;
- temporary `*.partial` files during downloads;
- `crl.pem` when CRL generation is active;
- a configured pprof directory when periodic snapshots are active.

Do not alter object or sidecar files while the service is running.

## Back up and restore

Cached packages are replaceable, but the cache may be valuable in restricted networks. For a consistent filesystem-level backup:

```bash
sudo systemctl stop goaptcacher
sudo tar -C /var/cache -czf /srv/backup/goaptcacher-cache.tar.gz goaptcacher
sudo systemctl start goaptcacher
```

Restore into the same `cache_directory`, preserve ownership for the `goaptcacher` user, and run `goaptcacher verify-repos` before relying on it.

For large installations, a filesystem snapshot is usually faster than a tar archive. The YAML configuration and HTTPS private key, if used, require a separate protected backup policy.

## Remove a single cached object

Stop the service, remove both the object and its adjacent `.access.json` sidecar, then restart. If only the object is removed, a later request detects the missing file and repairs the metadata; removing both avoids a transient stale record.

Use the exact normalized hostname and URL path. Keep a recoverable copy until a subsequent download and repository verification succeed.

## Reset the cache safely

A recoverable reset is preferable to deleting the live tree:

```bash
sudo systemctl stop goaptcacher
sudo mv /var/cache/goaptcacher /var/cache/goaptcacher.previous
sudo install -d -o goaptcacher -g goaptcacher /var/cache/goaptcacher
sudo systemctl start goaptcacher
```

Validate downloads and statistics before deleting `goaptcacher.previous`. This reset also starts traffic statistics from an empty state.

## Move the cache

1. Stop the service.
2. Copy or move the complete cache tree to the new filesystem.
3. Preserve service ownership and permissions.
4. Update `cache_directory` or `CACHE_DIR`.
5. Start the service and inspect the logs.
6. Run repository verification.

Do not split object files and sidecars across storage locations. When changing a mirror hostname, follow the additional namespace guidance in [Domain and mirror routing](/features/domain-routing#cache-migration-after-a-mirror-change).

## Stale temporary files

Normal failed requests remove their own `.partial` files. An abrupt crash can leave one behind. With the service stopped, identify old partial files and move them to quarantine before removal:

```bash
find /var/cache/goaptcacher -type f -name '*.partial' -mtime +1 -print
```

Never remove a recent partial file while a download may still be active.
