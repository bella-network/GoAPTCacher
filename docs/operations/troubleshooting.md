---
title: Troubleshooting
description: Diagnose startup, connectivity, cache, HTTPS, storage, and verification failures.
outline: deep
---

# Troubleshooting

Start with the effective configuration, service log, listener state, and JSON statistics:

```bash
sudo systemctl status goaptcacher
sudo journalctl -u goaptcacher -n 200 --no-pager
sudo ss -ltnp | grep -E ':(8090|8091|3142)\b'
curl -s http://127.0.0.1:8090/_goaptcacher/api/stats
```

## Service does not start

### Configuration cannot be read

Confirm the selected path. Precedence is `--config`, then `CONFIG`, then `./config.yaml`. The package sets `CONFIG=/etc/goaptcacher/config.yaml`.

```bash
sudo -u goaptcacher test -r /etc/goaptcacher/config.yaml
sudo systemctl show goaptcacher -p Environment
```

Inspect YAML indentation and field types. There is no separate validation command, so run the executable interactively only in a safe maintenance window or validate YAML with an external parser.

### Cache directory permission denied

```bash
sudo install -d -o goaptcacher -g goaptcacher /var/cache/goaptcacher
sudo -u goaptcacher test -w /var/cache/goaptcacher
```

For containers, the image runs as UID/GID `65532`; fix the mounted directory rather than running as root.

### Interception key or certificate error

Both `https.cert` and `https.key` must point to readable PEM files containing a matching CA certificate and RSA or ECDSA key. Check service ownership, encryption password, certificate dates, and CA constraints.

## Client cannot connect

1. Confirm the proxy hostname resolves from the client.
2. Test the configured TCP port.
3. Verify host and network firewalls.
4. Check `apt-config dump` for an older overriding proxy directive.

```bash
getent hosts cache.example.com
nc -vz cache.example.com 8090
apt-config dump | grep -i proxy
```

Do not use `/` as an HTTP health endpoint; its expected status is `406`. Use `/_goaptcacher/api/stats`.

## `403 Forbidden`

The requested host did not match `domains` or `passthrough_domains`, or `https.prevent` rejected a `CONNECT` request.

Inspect the log entry and add only the exact required repository hostname. Remember that redirects may introduce a second hostname, such as `ppa.launchpadcontent.net`.

Restart GoAPTCacher after editing YAML.

## Packages are not cached

- HTTPS tunnel mode cannot inspect or cache content.
- A passthrough match takes precedence over a cached-domain match.
- Empty domain lists deliberately disable caching.
- A mirror redirect may lead to a hostname absent from the allowlist.
- A request method other than `GET` or `HEAD` is unsupported.

Check `X-Cache` on an HTTP request:

```bash
curl -I -x http://cache.example.com:8090 \
  http://archive.ubuntu.com/ubuntu/dists/noble/InRelease
```

Repeat the request and compare the stats page. A first `HEAD` miss downloads the full object.

## TLS errors on clients

In tunnel mode, diagnose the origin certificate, client time, and repository URL as usual.

In interception mode:

- verify that the client trusts the configured CA;
- compare the installed CA fingerprint with a trusted copy;
- confirm that the requested hostname is in the generated leaf certificate;
- place pinned or mutually authenticated origins in `passthrough_domains`;
- verify access to AIA/CRL URLs if clients require them.

Switching interception off restores end-to-end TLS after a restart, but HTTPS content will no longer be cached.

## Stale repository metadata

Mutable files under `/dists/` are rechecked every five minutes when requested. Inspect refresh logs for origin errors or unexpected validators.

To force a clean retrieval, stop the service and quarantine the affected object together with its `.access.json` sidecar, then restart and run `apt update`. Avoid deleting an entire cache until the problem is isolated.

## `507 Insufficient Storage`

GoAPTCacher could not reserve the origin's declared object size. Check both bytes and inodes:

```bash
df -h /var/cache/goaptcacher
df -i /var/cache/goaptcacher
du -sh /var/cache/goaptcacher
```

Enable or shorten expiration, expand the filesystem, or perform a controlled cache cleanup. Concurrent unknown-length responses can still exhaust a nearly full filesystem, so retain headroom.

## Download waits and retry failures

Concurrent misses for the same URL wait for the active writer. After roughly 25 retries, a waiting request can return an internal error stating that the file is being downloaded.

Look for a slow or stalled origin, disk latency, and incomplete `.partial` files. Do not remove a partial file while the process may still be writing it.

## Repository verification reports mismatches

The verifier logs mismatches without deleting them and may still exit successfully. Follow the quarantine and rerun procedure in [Repository verification](/operations/repository-verification#result-semantics).

## Discovery fails

Query the exact SRV name, verify its target address and port, and confirm the client's DNS search suffix. mDNS normally does not cross routers. See [Automatic discovery](/features/discovery#validation).

## Information to include in an issue

- `goaptcacher --version` output;
- operating system or container image tag;
- redacted configuration;
- relevant client repository URLs and APT proxy configuration;
- logs around one failing request;
- HTTP status and response headers;
- available filesystem space and inodes;
- whether the same origin works directly.

Do not include private keys, proxy credentials, access tokens, or internal data that is not required to reproduce the problem.
