---
title: Security best practices
description: Deploy GoAPTCacher with safe network boundaries, domain policy, storage, and HTTPS interception controls.
outline: deep
---

# Security best practices

GoAPTCacher is part of the software-supply and network-egress path for every configured client. Its safety depends on layered controls: APT signature validation, narrow proxy policy, external network enforcement, protected storage, and disciplined administration.

## Security properties and boundaries

GoAPTCacher provides:

- an allowlist for requested host suffixes;
- a distinct passthrough policy;
- normal Go TLS verification when it connects to HTTPS origins;
- local SHA-256 metadata for cached content;
- optional comparison with checksums in cached repository metadata;
- unprivileged package and container execution.

GoAPTCacher does not provide:

- client authentication or authorization;
- rate limiting or per-client quotas;
- a label-aware hostname allowlist;
- cryptographic verification of `InRelease` signatures in `verify-repos`;
- a complete offline mirror;
- isolation between clients sharing one cache;
- automatic repair of checksum mismatches.

APT clients must continue to verify signed repository metadata and package hashes.

## Restrict client access

All listeners bind to all interfaces and share proxy, UI, API, and debug routes. Allow access only from intended client networks and administrators.

Never expose an instance with empty `domains` and `passthrough_domains` to an untrusted network. That configuration accepts every destination and disables caching, effectively creating an open forward proxy.

If separate trust zones run untrusted workloads, give each zone its own instance and cache rather than allowing one client population to influence content and capacity for another.

## Use a narrow domain policy

The current matcher uses raw suffix comparison. `example.com` therefore also matches `notexample.com`. Prefer full repository hostnames and add `.example.com` only when all subdomains are trusted.

Review redirect behavior as well. Origin downloads use the standard Go HTTP client, which follows redirects; the initial hostname passes GoAPTCacher policy, but redirect destinations are not re-evaluated by that policy. Enforce outbound DNS and destination controls independently.

Keep authenticated, certificate-pinned, client-certificate, or sensitive origins in `passthrough_domains`. Review both lists as code.

## Protect the cache host

- Run as the dedicated `goaptcacher` user or container UID `65532`.
- Keep configuration writable only by administrators.
- Keep `cache_directory` writable only by the service and trusted maintenance processes.
- Apply operating-system updates and restrict interactive access.
- Monitor free space, inodes, process restarts, and unusual egress.
- Do not mount the Docker socket or unnecessary host paths into the container.
- Use a read-only container filesystem with a dedicated writable cache volume.

The `index.contact` value is rendered as administrator-supplied HTML in the UI. Treat configuration write access as trusted code/content deployment.

## Prefer HTTPS tunnel mode

Tunnel mode preserves end-to-end origin TLS and keeps the proxy out of the certificate trust chain. It is the recommended default even though HTTPS content cannot be cached.

HTTPS interception makes the CA private key a high-value secret. If interception is required:

- use a dedicated intermediate CA where organizational PKI supports it;
- store the key outside the image and source repository;
- restrict its file permissions to the service account;
- deploy the public CA through authenticated configuration management;
- verify fingerprints out of band;
- define passthrough exceptions before rollout;
- monitor certificate errors and unexpected hostnames;
- rehearse key rotation, CRL publication, and emergency removal of trust.

Do not use the same interception CA for unrelated services.

## Protect operations and debug endpoints

The web interface reveals repository hosts, routing, capacity, version, and traffic patterns. The debug JSON and pprof handlers expose much more runtime detail.

Keep `debug.enable: false` normally. During diagnosis, retain `allow_remote: false` and access loopback through SSH or another protected mechanism. Disable debug mode again after collecting evidence and remove unneeded profile files.

`index.enable` is not an access-control mechanism in the current implementation. Apply firewall or reverse-proxy policy to the listener.

## Preserve cache integrity

- Stop the service before manual cache changes or filesystem backups.
- Keep object files with their `.access.json` sidecars.
- Run repository verification periodically and alert on warning output.
- Quarantine mismatches instead of deleting evidence immediately.
- Use storage with error monitoring and tested backups where offline cache value matters.
- Leave capacity headroom for concurrent misses and metadata replacement.

Remember that `verify-repos` reports mismatches but does not fail solely for them or repair them.

## Configuration and secret handling

The ordinary YAML contains no required secrets unless an encrypted HTTPS key password is placed in `https.password`. If used, that password and the referenced private key require secret-management controls.

Do not commit:

- interception private keys or passwords;
- internal proxy credentials from unrelated systems;
- production configurations containing sensitive internal names when repository visibility matters;
- debug profiles or logs without review.

The repository `.gitignore` covers common local key filenames, but ignore rules are not a security boundary.

## Monitoring signals

Alert on:

- repeated `[INFO:403]` requests from unexpected clients;
- new repository or redirect destinations;
- spikes in tunnel bytes or cache misses;
- `507` responses and filesystem thresholds;
- refresh and origin failures;
- repository verification warnings;
- unexpected debug enablement;
- service execution as root;
- changes to configuration, unit files, CA material, or firewall rules.

## Production checklist

- [ ] Client networks and outbound destinations are restricted externally.
- [ ] Domain entries are narrow and reviewed; both lists are not empty.
- [ ] Redirect destinations are covered by egress controls.
- [ ] HTTPS mode and passthrough exceptions are documented.
- [ ] Interception CA key controls and rotation are tested, if applicable.
- [ ] Service and cache run without root privileges.
- [ ] Configuration, cache, logs, and debug profiles have appropriate permissions.
- [ ] UI/API and debug routes are inaccessible from untrusted networks.
- [ ] Capacity and integrity monitoring is active.
- [ ] Repository-verification warnings are parsed, not just exit codes.
- [ ] Clients still enforce APT repository signatures.
- [ ] A recovery and direct-access fallback has been tested.
