---
title: Installation
description: Install GoAPTCacher from a Debian package or build it from source.
outline: deep
---

# Installation

## Debian and Ubuntu package

Released packages are available for Linux `amd64` and `arm64` from the bella.network APT repository.

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
```

The package provides:

| Path | Purpose |
| --- | --- |
| `/usr/bin/goaptcacher` | Server and repository-verification command |
| `/etc/goaptcacher/config.yaml` | Main YAML configuration |
| `/lib/systemd/system/goaptcacher.service` | Long-running proxy service |
| `/lib/systemd/system/goaptcacher-repoverify.service` | One-shot checksum verification |
| `/lib/systemd/system/goaptcacher-repoverify.timer` | Periodic verification schedule |
| `/var/cache/goaptcacher` | Recommended persistent cache directory |

The service runs as the dedicated `goaptcacher` user. Package installation enables both the proxy service and the verification timer.

After changing the configuration, restart and inspect the service:

```bash
sudo systemctl restart goaptcacher
sudo systemctl status goaptcacher
sudo journalctl -u goaptcacher -n 100 --no-pager
```

## Container image

Release builds publish an image to the GitLab container registry. See [Container deployment](/guide/container) for Docker and Compose examples, required mounts, ports, and permissions.

## Build from source

The module declares Go 1.26.3 and a Go 1.26.6 toolchain. From a clean checkout:

```bash
go build -o goaptcacher ./cmd/goaptcacher
./goaptcacher --version
```

Run the server with an explicit configuration:

```bash
sudo install -d -o goaptcacher -g goaptcacher /var/cache/goaptcacher
./goaptcacher --config ./config.yaml-example
```

A source build does not install a service account, systemd unit, configuration, or cache directory. Create those explicitly or adapt the files in `source/` from the repository.

## Upgrade

For package installations:

```bash
sudo apt update
sudo apt install --only-upgrade goaptcacher
sudo systemctl status goaptcacher goaptcacher-repoverify.timer
```

Back up the configuration before an upgrade. Cache files and persistent statistics reside below `cache_directory` and should survive package replacement.

## Uninstall

```bash
sudo apt remove goaptcacher
```

Removing the package disables the installed services. Treat the configured cache directory and local configuration as persistent data and review them separately if they are no longer needed.
