---
title: HTTPS modes
description: Choose between blocking, tunneling, and intercepting HTTPS repository traffic.
outline: deep
---

# HTTPS modes

APT reaches an HTTPS repository through an HTTP proxy by sending `CONNECT repository.example:443`. GoAPTCacher can reject that connection, relay it without decryption, or terminate TLS and cache the decrypted repository response.

## Mode comparison

| Mode | Configuration | End-to-end TLS | HTTPS caching | Operational risk |
| --- | --- | --- | --- | --- |
| Block | `prevent: true` | No connection | No | Low, but HTTPS repositories fail |
| Tunnel | `prevent: false`, `intercept: false` | Yes | No | Recommended default |
| Intercept | `prevent: false`, `intercept: true` | No | Yes | Requires managed CA trust and key protection |

Do not set both `prevent` and `intercept` to `true`. `CONNECT` requests are rejected before interception is considered, while the direct TLS listener may still start.

## Tunnel mode

```yaml
https:
  prevent: false
  intercept: false
```

The proxy creates a TCP connection to the requested host and copies bytes in both directions. Repository certificates are validated by the APT client exactly as they would be without GoAPTCacher. The proxy records tunnel request and byte counts but cannot inspect or cache the content.

Use tunnel mode unless the bandwidth benefit of caching HTTPS content justifies operating an interception CA.

## Block mode

```yaml
https:
  prevent: true
  intercept: false
```

Every allowed `CONNECT` request receives `403 Forbidden`. HTTP repositories continue to use the normal cache. This mode is useful only in controlled environments where HTTPS repositories are intentionally prohibited or clients use direct exceptions.

## Interception mode

```yaml
https:
  prevent: false
  intercept: true
  cert: "/etc/goaptcacher/intermediate-ca.crt"
  key: "/etc/goaptcacher/intermediate-ca.key"
  password: ""
  certificate_domain: "cache.example.com"
  aia_address: "http://cache.example.com:8090/_goaptcacher/goaptcacher.crt"
  enable_crl: true
```

At startup, GoAPTCacher loads the PEM-encoded CA certificate and its matching RSA or ECDSA private key. The key may use a supported encrypted PEM format with `password`. Missing or invalid material stops startup.

For each requested hostname, the proxy creates a short-lived leaf certificate signed by that CA and performs TLS independently with the client and origin. Generated leaf certificates are held in memory and renewed as they approach expiration.

The direct TLS listener also starts on `listen_port_secure` (default `8091`). Most explicit APT proxy deployments still point both APT directives at the HTTP listener on port `8090`.

## Deploy CA trust

Copy only the public CA certificate to each managed client:

```bash
curl -fsS http://cache.example.com:8090/_goaptcacher/goaptcacher.crt \
  | sudo tee /usr/local/share/ca-certificates/goaptcacher.crt >/dev/null
sudo update-ca-certificates
```

Verify the certificate source through a trusted channel before installing it. Never distribute the private key.

Applications that pin origin certificates, use client certificates, or carry sensitive authenticated traffic should bypass interception:

```yaml
passthrough_domains:
  - "esm.ubuntu.com"
  - "enterprise.proxmox.com"
```

## AIA and CRL

`aia_address` is embedded in generated certificates when set. If it is empty but `certificate_domain` is configured, GoAPTCacher derives the public certificate URL on the HTTP listener.

With `enable_crl: true` and a non-empty `certificate_domain`, GoAPTCacher generates `cache_directory/crl.pem` every 30 minutes and advertises:

```text
http://<certificate_domain>:<listen_port>/_goaptcacher/revocation.crl
```

CRL generation is not a substitute for protecting and rotating the CA key. Clients also need network access to the advertised HTTP endpoint if they perform revocation checks.

## Security boundary

Interception gives the GoAPTCacher host and CA key the ability to impersonate approved HTTPS origins to trusting clients. Therefore:

- use a dedicated intermediate CA constrained by organizational policy where possible;
- restrict the private key to the service account and exclude it from images, Git, and ordinary backups;
- limit `domains` and client network access;
- place authenticated or pinned origins in `passthrough_domains`;
- log and review configuration changes;
- test CA rotation and emergency revocation before production use.

See [Security best practices](/security/best-practices) for the complete deployment checklist.
