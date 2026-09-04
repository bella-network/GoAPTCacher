---
title: Client configuration
description: Configure Debian, Ubuntu, automation, and CI clients to use GoAPTCacher.
outline: deep
---

# Client configuration

GoAPTCacher is an explicit proxy. APT clients must either receive static proxy directives or discover the proxy through `auto-apt-proxy`.

## Static APT configuration

Create `/etc/apt/apt.conf.d/10proxy`:

```text
Acquire::http::Proxy "http://cache.example.com:8090/";
Acquire::https::Proxy "http://cache.example.com:8090/";
```

Use the HTTP listener for both directives. For HTTPS repositories, APT sends a `CONNECT` request to that listener; GoAPTCacher then blocks, tunnels, or intercepts it according to the configured [HTTPS mode](/concepts/https-modes).

Validate the effective values:

```bash
apt-config dump | grep -E 'Acquire::(http|https)::Proxy'
sudo apt update
```

## Per-host exceptions

APT can bypass the proxy for selected origins:

```text
Acquire::http::Proxy "http://cache.example.com:8090/";
Acquire::https::Proxy "http://cache.example.com:8090/";
Acquire::https::Proxy::login.example.com "DIRECT";
```

Prefer a server-side `passthrough_domains` entry when the traffic should still traverse GoAPTCacher but must not be cached or intercepted. Use `DIRECT` when the client should connect to the origin without the proxy.

## Configuration management

Manage the proxy file as a complete file rather than appending on every run. An Ansible task can look like:

```yaml
- name: Configure APT cache proxy
  ansible.builtin.copy:
    dest: /etc/apt/apt.conf.d/10proxy
    owner: root
    group: root
    mode: "0644"
    content: |
      Acquire::http::Proxy "http://cache.example.com:8090/";
      Acquire::https::Proxy "http://cache.example.com:8090/";
```

## Ephemeral CI jobs

For a Debian-family build image:

```yaml
build:
  image: debian:stable-slim
  before_script:
    - printf '%s\n' 'Acquire::http::Proxy "http://goaptcacher.internal:8090/";' > /etc/apt/apt.conf.d/10proxy
    - printf '%s\n' 'Acquire::https::Proxy "http://goaptcacher.internal:8090/";' >> /etc/apt/apt.conf.d/10proxy
    - apt-get update
  script:
    - apt-get install -y --no-install-recommends build-essential
```

The runner network must resolve and reach the proxy. See the complete [CI runners example](/examples/ci-runners).

## Auto-discovery

Install `auto-apt-proxy` on clients and publish a DNS SRV record:

```bash
sudo apt install auto-apt-proxy
```

```text
_apt_proxy._tcp.example.com. 3600 IN SRV 0 0 8090 cache.example.com.
```

GoAPTCacher can also announce `_apt_proxy._tcp.local` through mDNS. DNS SRV behavior, mDNS scope, and validation are documented in [Automatic discovery](/features/discovery).

## Disable the proxy temporarily

Move the configuration out of APT's configuration directory, or override it for one command:

```bash
sudo apt-get -o Acquire::http::Proxy=DIRECT \
  -o Acquire::https::Proxy=DIRECT update
```

## Validation checklist

- `apt-config dump` shows the intended proxy URL.
- DNS resolves the proxy name from the client network.
- TCP port `8090` or the selected alternative port is reachable.
- The requested repository hostname is in `domains` or `passthrough_domains`.
- A repeated HTTP request increments the hit count.
- HTTPS traffic behaves as expected for the selected mode.
