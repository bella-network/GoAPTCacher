---
title: Domain and mirror routing
description: Control allowed repository targets, passthrough behavior, distribution overrides, and path remaps.
outline: deep
---

# Domain and mirror routing

GoAPTCacher evaluates a domain allowlist before deciding whether to cache, tunnel, or reject a request. Administrator-defined mirror routing is then applied to cacheable `GET` and `HEAD` requests.

## Cached domains

```yaml
domains:
  - "archive.ubuntu.com"
  - "security.ubuntu.com"
  - "deb.debian.org"
  - "security.debian.org"
  - ".debian.org"
```

A matching host is allowed and eligible for caching. Leading-dot entries such as `.debian.org` match subdomains but not the bare parent domain.

::: warning Suffix matching
The current implementation uses raw hostname suffix matching. An entry such as `example.com` also matches `notexample.com`. Prefer complete repository hostnames, add a leading-dot suffix only when all subdomains are trusted, and enforce network egress policy outside the process.
:::

Requests to non-matching hosts receive `403 Forbidden`. Only port `443` receives special handling during host matching; prefer standard repository ports.

## Passthrough domains

```yaml
passthrough_domains:
  - "esm.ubuntu.com"
  - "enterprise.proxmox.com"
```

Passthrough entries are allowed but never cached. A matching `CONNECT` request is tunneled even when HTTPS interception is enabled. Use passthrough for:

- authenticated repositories;
- origins with certificate pinning;
- repositories requiring client TLS certificates;
- sensitive origins that must retain end-to-end TLS;
- content that should not remain on shared storage.

If a hostname matches both lists, passthrough takes precedence.

## Empty policy

When both lists are empty, every hostname is accepted, cacheable methods bypass storage, and HTTPS is tunneled unless blocked. Startup logs a security warning.

This mode turns GoAPTCacher into an unrestricted forward proxy. Do not expose it outside an isolated test environment.

## Ubuntu mirror override

```yaml
overrides:
  ubuntu_server: "archive.ubuntu.com"
```

Requests whose host ends in `archive.ubuntu.com`, including country mirrors such as `at.archive.ubuntu.com`, are sent to the configured host. The value may include a path prefix:

```yaml
overrides:
  ubuntu_server: "mirror.internal.example/ubuntu"
```

The prefix is prepended to the original request path. Do not include `http://` or `https://` in the override value.

## Debian mirror override

```yaml
overrides:
  debian_server: "archive.debian.org"
```

The override handles `ftp.<country>.debian.org` hosts. It also applies special routing for `deb.debian.org`:

- `/debian/` is sent to `debian_server`;
- `/debian-security/`, `/debian-security-debug/`, `/debian-debug/`, and `/debian-ports/` are sent to `security.debian.org`.

As with the Ubuntu setting, the value may contain a path prefix but no URL scheme.

::: tip Keep destination hosts allowed
Add every effective destination to `domains` and to your external firewall policy. Domain authorization evaluates the client-requested host, while the configured override determines the outbound destination.
:::

## Exact path remap

The general `remap` list currently replaces an exact URL path. It does not replace a hostname or perform prefix matching.

```yaml
remap:
  - from: "/legacy/dists/stable/InRelease"
    to: "/debian/dists/stable/InRelease"
```

Use this only for narrow compatibility cases. A rule does nothing unless `from` equals the complete request path. Prefer distribution overrides for mirror consolidation.

## Cache migration after a mirror change

Cache paths include the effective origin hostname. Changing an override therefore creates a new cache namespace.

To reuse compatible cached data:

1. stop GoAPTCacher;
2. verify that old and new mirrors use the same repository path layout;
3. make a backup or snapshot;
4. ensure the destination cache directory does not exist;
5. rename the old hostname directory to the new hostname;
6. start the service and run repository verification.

Never merge two live cache trees while the service is running. The object files and `.access.json` sidecars must remain consistent.

## Recommended policy review

For every configured entry, document:

- which clients require it;
- whether content may be cached;
- whether credentials or client certificates are used;
- the expected outbound destination after overrides;
- whether HTTPS interception is permitted;
- who approves future changes.
