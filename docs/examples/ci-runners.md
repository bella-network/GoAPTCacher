---
title: CI runners example
description: Accelerate Debian-family CI jobs with a shared GoAPTCacher instance.
outline: deep
---

# CI runners example

A shared proxy is most effective when many ephemeral jobs install overlapping package sets. Place GoAPTCacher close to the runners and keep its cache on persistent, low-latency storage.

## Cache server policy

Use an explicit allowlist for all base images and third-party build repositories:

```yaml
cache_directory: "/var/cache/goaptcacher"
listen_port: 8090

domains:
  - "deb.debian.org"
  - "security.debian.org"
  - "archive.ubuntu.com"
  - "security.ubuntu.com"
  - "ports.ubuntu.com"
  - "packages.microsoft.com"
  - "download.docker.com"
  - "repo.bella.network"

passthrough_domains: []

https:
  prevent: false
  intercept: false

index:
  enable: true
  hostnames:
    - "goaptcacher.ci.internal"

expiration:
  unused_days: 45
```

Tunnel mode means modern HTTPS repositories do not benefit from content caching. Enable interception only if the runner trust store can be managed safely and the performance benefit is measured.

## GitLab CI variable

Define a group or project variable:

```text
APT_PROXY_URL=http://goaptcacher.ci.internal:8090/
```

It contains no secret but centralizes the endpoint.

## Reusable job template

```yaml
.apt-proxy:
  before_script:
    - test -n "$APT_PROXY_URL"
    - printf 'Acquire::http::Proxy "%s";\n' "$APT_PROXY_URL" > /etc/apt/apt.conf.d/10proxy
    - printf 'Acquire::https::Proxy "%s";\n' "$APT_PROXY_URL" >> /etc/apt/apt.conf.d/10proxy
    - apt-get update

build:
  extends: .apt-proxy
  image: debian:stable-slim
  script:
    - apt-get install -y --no-install-recommends build-essential ca-certificates
    - make
```

This assumes the job runs as root, as official Debian and Ubuntu images normally do. For an unprivileged image, bake the proxy file into a controlled base image or use an entrypoint with the required permissions.

## Docker executor networking

The proxy DNS name must resolve inside job containers, not only on the runner host. Avoid `localhost`: inside the job it refers to that container.

For a proxy running beside a self-managed Docker runner, use a routable internal address or attach both to a managed Docker network with stable DNS. Test from the exact job image:

```bash
docker run --rm debian:stable-slim \
  sh -c 'getent hosts goaptcacher.ci.internal'
```

## Concurrency

GoAPTCacher serializes simultaneous misses for the same object so one writer populates the cache. Waiting requests retry for roughly 25 seconds. Very slow origins or disks can cause later jobs to fail before the first download completes.

Reduce this risk by:

- using local SSD-backed storage;
- keeping the proxy near runners;
- avoiding excessive parallel cold-cache fan-out;
- warming common packages with one controlled job after a cache reset;
- retaining enough filesystem headroom for concurrent downloads.

Warming remains demand-driven: run a normal representative APT install rather than attempting to mirror whole repositories.

## Metrics for CI

Collect the JSON API after representative pipelines:

```bash
curl -s http://goaptcacher.ci.internal:8090/_goaptcacher/api/stats \
  | jq '.totals'
```

Track cache hit rate, upstream bytes, and job package-install duration together. A low hit rate can be normal when images use HTTPS tunnel mode, repositories publish unique paths, or jobs use many distributions and architectures.

## Security

- Allow the proxy port only from runner networks.
- Keep project workloads from changing proxy configuration or reading interception keys.
- Do not leave both domain lists empty.
- Treat repositories used by untrusted jobs as explicit policy entries.
- Separate caches for trust zones if one runner group must not influence another.
- Continue to rely on APT signature and package verification in each job.
