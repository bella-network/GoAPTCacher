# GoAPTCacher

GoAPTCacher is a pull-through caching proxy for Debian and Ubuntu APT repositories. It stores requested repository metadata and packages on local disk, streams cache misses to clients, and serves later requests from the cache.

**[Documentation](https://goaptcacher.docs.bella.network/)** · **[Getting started](https://goaptcacher.docs.bella.network/guide/getting-started)** · **[Configuration reference](https://goaptcacher.docs.bella.network/reference/configuration)** · **[Releases](https://gitlab.com/bella.network/goaptcacher/-/releases)**

## Project status

`main` is the development branch and may contain breaking changes. Use a tagged release for production deployments.

## Features

- Pull-through cache for APT `GET` and `HEAD` requests
- Streaming cache misses and concurrent-download protection
- HTTP proxying plus HTTPS tunnel, block, or interception modes
- Cache and passthrough domain policies
- Ubuntu/Debian mirror overrides and exact path remapping
- Conditional refresh of repository metadata
- Automatic expiration of unused files
- Persistent traffic statistics and a built-in web interface
- APT proxy discovery through mDNS and DNS SRV records
- Cached repository checksum verification with systemd timer support
- Optional JSON diagnostics and pprof profiles

## Quick start

Install the Debian package from the bella.network repository, copy and review the supplied configuration, then start the service:

```bash
curl -fsSL https://repo.bella.network/_static/bella-archive-keyring.gpg \
  | sudo tee /usr/share/keyrings/bella-archive-keyring.gpg >/dev/null

sudo tee /etc/apt/sources.list.d/repo.bella.network.sources >/dev/null <<'EOF'
Types: deb
URIs: https://repo.bella.network/deb
Suites: stable
Components: main
Architectures: amd64 arm64
Signed-By: /usr/share/keyrings/bella-archive-keyring.gpg
EOF

sudo apt update
sudo apt install goaptcacher
sudo editor /etc/goaptcacher/config.yaml
sudo systemctl enable --now goaptcacher
```

Configure an APT client in `/etc/apt/apt.conf.d/10proxy`:

```text
Acquire::http::Proxy "http://cache.example.com:8090/";
Acquire::https::Proxy "http://cache.example.com:8090/";
```

Run `sudo apt update`, then open `http://cache.example.com:8090/_goaptcacher/` to inspect the instance.

The complete setup, including containers, HTTPS interception, discovery, CI runners, security guidance, and troubleshooting, is covered in the **[documentation](https://goaptcacher.docs.bella.network/)**.

## Development

```bash
go test ./...
npm ci
npm run docs:build
```

Use `npm run docs:dev` for a local documentation server.

## License

GoAPTCacher is released under the [MIT License](LICENCE.md).
