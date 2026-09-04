---
title: Restricted network example
description: Provide controlled APT access to systems without general internet connectivity.
outline: deep
---

# Restricted network example

GoAPTCacher can be the only permitted path from a server segment to approved package repositories. It is a pull-through proxy, not an offline mirror: the proxy itself still needs access to an origin on every cold miss and refresh.

## Traffic model

```text
Restricted clients ──TCP 8090──▶ GoAPTCacher ──TCP 80/443──▶ approved repositories
        │                              │
        └──── no direct internet ──────┴──── controlled DNS and egress policy
```

Use both application policy and network enforcement. The YAML allowlist does not replace firewall rules, and the process does not authenticate clients.

## Example configuration

```yaml
cache_directory: "/var/cache/goaptcacher"
listen_port: 8090

domains:
  - "deb.debian.org"
  - "security.debian.org"
  - "archive.ubuntu.com"
  - "security.ubuntu.com"
  - "repo.bella.network"

passthrough_domains: []

https:
  prevent: false
  intercept: false

index:
  enable: true
  hostnames:
    - "apt-egress.internal.example"
  contact: "Package access: infra@example.com"

mdns: false
expiration:
  unused_days: 180
```

This permits approved HTTPS repositories through end-to-end tunnels. Host policy limits the destination name, while external DNS and egress controls should limit where that name can resolve and which ports the proxy can reach.

## Firewall policy

A typical design permits:

- restricted clients to reach only proxy TCP `8090`;
- management systems to reach the operations interface;
- the proxy to query controlled DNS resolvers;
- the proxy to reach TCP `80` and `443` only for approved destinations;
- no direct client egress to repository networks.

Because IP addresses behind public mirrors and CDNs change, implement outbound controls through an approved secure web gateway, dynamic address sets, or another mechanism appropriate to the network. Do not hard-code short-lived CDN addresses without an update process.

## HTTPS choice

Tunnel mode preserves repository TLS but allows no HTTPS cache reuse. Interception provides caching and path visibility at the cost of a highly sensitive CA key and loss of end-to-end TLS.

For a restricted network, choose explicitly:

- tunnel when destination control and origin TLS are more important than HTTPS cache savings;
- interception when bandwidth is constrained, all clients are managed, and an interception CA lifecycle already exists;
- block when clients must use only approved HTTP mirrors.

Place authenticated and pinned repositories in `passthrough_domains` if interception is enabled.

## Availability during outages

A cache hit can be served without contacting the origin in many cases, but mutable metadata is periodically rechecked and a cache miss always needs upstream access. Do not treat GoAPTCacher as a complete offline repository.

If offline operation is a requirement:

- maintain a proper signed repository mirror or snapshot;
- pre-stage and verify all required packages and metadata;
- document repository expiry behavior;
- test a full rebuild with the upstream link disabled.

GoAPTCacher can still sit in front of that internal mirror to reduce repeated reads.

## Change workflow

1. Receive a request for a new repository hostname.
2. Review ownership, transport, authentication, redirect targets, and signing key distribution.
3. Add the narrowest domain or passthrough entry.
4. Update external DNS and firewall policy.
5. Restart GoAPTCacher.
6. test `apt update` from a representative client;
7. review proxy logs and statistics;
8. record the approval and rollback plan.

## Verification and monitoring

Run repository verification daily and alert on warning text as well as service failure. Monitor cache filesystem capacity, denied host attempts, tunnel volume, and unexpected changes in outbound destinations.

APT clients must continue to validate signed repository metadata. A cache server compromise must not silently become sufficient to install untrusted packages.
