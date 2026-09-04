---
layout: home

hero:
  name: GoAPTCacher
  text: Faster APT downloads from a shared local cache
  tagline: A pull-through proxy for Debian and Ubuntu repositories, designed for networks, build systems, and CI runners.
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Configuration reference
      link: /reference/configuration
    - theme: alt
      text: View on GitLab
      link: https://gitlab.com/bella.network/goaptcacher

features:
  - icon: 📦
    title: Pull-through cache
    details: Download each requested package or metadata file once, stream it to the first client, and reuse it for later clients.
  - icon: 🔒
    title: Three HTTPS modes
    details: Tunnel HTTPS end to end, reject it, or intercept approved repositories so their content can be cached.
  - icon: 🧭
    title: Controlled routing
    details: Allow only selected repository domains, configure explicit passthrough domains, and consolidate Debian or Ubuntu mirrors.
  - icon: 📊
    title: Built-in visibility
    details: Inspect cache size, hit rate, traffic, routing, and client setup through the web interface and JSON statistics API.
  - icon: 🧹
    title: Cache lifecycle
    details: Refresh mutable repository metadata conditionally and expire files that have not been used for a configured period.
  - icon: ✅
    title: Repository verification
    details: Validate cached indexes and packages against the strongest supported checksums from cached InRelease metadata.
---

## What is GoAPTCacher?

GoAPTCacher is an explicit HTTP proxy for APT. Clients continue to request their normal Debian, Ubuntu, or third-party repository URLs, while the proxy enforces a domain policy and stores approved responses below a shared cache directory.

```text
APT clients ──HTTP/CONNECT──▶ GoAPTCacher ──HTTP/HTTPS──▶ Repository mirrors
                                  │
                                  ├── package and metadata cache
                                  ├── access metadata and statistics
                                  └── web interface and diagnostics
```

The cache is demand-driven: GoAPTCacher downloads only files requested by a client. It does not mirror an entire repository and does not prefetch packages.

## Where to begin

Follow [Getting started](/guide/getting-started) to install the package, configure a safe domain allowlist, connect a client, and confirm the first cache hit. Container users can start with [Container deployment](/guide/container).

Before production use, choose the appropriate [HTTPS mode](/concepts/https-modes), review the [security best practices](/security/best-practices), and establish procedures for [cache management](/operations/cache-management), [repository verification](/operations/repository-verification), and [troubleshooting](/operations/troubleshooting).
