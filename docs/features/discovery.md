---
title: Automatic discovery
description: Discover GoAPTCacher with auto-apt-proxy, DNS SRV records, or mDNS.
outline: deep
---

# Automatic discovery

Static APT proxy directives are the most predictable option. Discovery is useful for laptops, ephemeral workers, and CI runners that move between networks.

## auto-apt-proxy with DNS SRV

Install the client helper:

```bash
sudo apt install auto-apt-proxy
```

Publish an SRV record in the DNS suffix used by the clients:

```text
_apt_proxy._tcp.example.com. 3600 IN SRV 0 0 8090 cache.example.com.
```

The fields after `SRV` are priority, weight, port, and target. The target must be a hostname with an address record and normally ends with a dot in zone-file syntax.

`auto-apt-proxy` derives the search domain from the client hostname and resolver configuration, discovers the proxy, and probes its root URL. GoAPTCacher intentionally returns `406 Not Acceptable` at `/`, which the client uses as a compatible proxy signal.

## mDNS announcement

Enable local multicast announcement:

```yaml
mdns: true
```

GoAPTCacher announces the APT proxy service on the primary HTTP port in the `.local` multicast domain. mDNS is limited to the local broadcast domain unless the network deliberately reflects multicast DNS.

Use DNS SRV for routed networks and mDNS only on small trusted LANs. Verify multicast firewall policy before relying on it.

## Per-repository SRV records

Repository-specific records can steer supported clients to the proxy:

```text
_http._tcp.at.archive.ubuntu.com. 3600 IN SRV 0 0 8090 cache.example.com.
_https._tcp.download.docker.com. 3600 IN SRV 0 0 8091 cache.example.com.
```

The HTTPS form requires interception and the direct TLS listener. Add records for every repository hostname that should be redirected, and test client behavior because not every APT configuration or helper consumes these records.

## Validation

Query DNS directly:

```bash
dig +short SRV _apt_proxy._tcp.example.com
getent hosts cache.example.com
```

Check the proxy compatibility response:

```bash
curl -i http://cache.example.com:8090/
```

Then run:

```bash
sudo apt update
curl -s http://cache.example.com:8090/_goaptcacher/api/stats
```

A discovered HTTP repository should increment cache hits or misses. HTTPS in tunnel mode increments tunnel counters instead.

## Failure modes

### No SRV answer

Confirm that the client uses the intended DNS suffix and resolver. Split-DNS and VPN clients often receive a different search list.

### Target resolves but cannot connect

Check routing and host firewall rules for the SRV port. Alternative listener port `3142` must be listed in `alternative_ports` before it is advertised.

### Discovery works on one subnet only

DNS should work across routed networks; mDNS normally should not. Publish a unicast DNS SRV record for all required client zones.

### Proxy found, repository denied

Discovery authorizes no repository destinations. Add required hosts to `domains` or `passthrough_domains` and restart GoAPTCacher.

## Best practices

- Keep a static configuration for servers whose package availability is critical.
- Use a low DNS TTL while rolling out or moving a proxy, then increase it after validation.
- Advertise a stable DNS name rather than a container or host IP.
- Remove discovery records before decommissioning an instance.
- Do not use discovery to expose an unauthenticated proxy to untrusted clients.
