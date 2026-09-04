---
title: Repository verification
description: Verify cached repository metadata and package checksums manually or with systemd.
outline: deep
---

# Repository verification

The `verify-repos` command scans cached repository trees and compares present files with checksums published in cached `InRelease` and `Packages` metadata.

## Run manually

The command uses the normal configuration path selection:

```bash
sudo -u goaptcacher goaptcacher \
  --config /etc/goaptcacher/config.yaml verify-repos
```

With the packaged environment:

```bash
sudo systemctl start goaptcacher-repoverify.service
sudo journalctl -u goaptcacher-repoverify.service --no-pager
```

## Discovery and validation

For each file matching `<repository>/dists/<distribution>/InRelease`, the verifier:

1. reads repository metadata from the local cache;
2. selects SHA-512 when published and falls back to SHA-256;
3. validates cached repository index files listed by `InRelease`;
4. parses valid `Packages`, `Packages.gz`, `Packages.xz`, and `Packages.bz2` indexes;
5. validates cached `.deb` files referenced by those package indexes.

Files absent from the pull-through cache are skipped because they may never have been requested. Duplicate repository roots and distributions are processed once.

## Result semantics

Successful files are reported at repository level. Mismatching paths are logged with `[DEBREPOCLEANER-WARN]` and included in the final mismatch count.

::: warning Mismatches are reported, not repaired
The command does not delete, quarantine, or redownload a mismatching file. It also does not currently return a failure solely because mismatches were found. Alert on warning lines and the final `mismatches` count, not only the process exit code.
:::

A non-zero exit occurs when repository discovery fails or one or more repositories cannot be initialized or verified. No `InRelease` files is a successful no-op.

After a mismatch:

1. stop or isolate client traffic if integrity is uncertain;
2. record the repository, distribution, and paths from the log;
3. move the affected object and sidecar to quarantine while the service is stopped;
4. restart the proxy and let a client retrieve the object again;
5. rerun verification;
6. investigate repeated mismatches as a storage, mirror, or software issue.

## Packaged timer

The package installs `goaptcacher-repoverify.timer` with:

- first run about 15 minutes after boot;
- repetition every 24 hours;
- randomized delay up to 30 minutes;
- persistent catch-up after downtime.

Inspect and control it with:

```bash
sudo systemctl status goaptcacher-repoverify.timer
sudo systemctl list-timers goaptcacher-repoverify.timer
sudo systemctl start goaptcacher-repoverify.service
sudo journalctl -u goaptcacher-repoverify.service
```

Disable or re-enable the schedule:

```bash
sudo systemctl disable --now goaptcacher-repoverify.timer
sudo systemctl enable --now goaptcacher-repoverify.timer
```

The one-shot service runs as `goaptcacher` with reduced CPU and I/O priority. Ensure that user can read the complete cache tree.

## Container scheduling

Containers do not run the packaged systemd timer. Invoke the command through the running container:

```bash
docker exec goaptcacher /goaptcacher verify-repos
```

Schedule it with a host timer, container platform CronJob, or CI schedule. Preserve the command output so warning-based alerting remains possible.

## Limitations

- `InRelease` signatures are not cryptographically verified by this command; published hashes are parsed from the cached file.
- Only SHA-512 and SHA-256 checksum sections are supported.
- Only cached files are examined.
- A package is verified only when a cached, valid Packages index references it.
- Verification can detect content mismatch but does not repair it automatically.

APT performs its own repository signature and package integrity checks on clients. Server-side verification is an additional cache-health control, not a replacement.
