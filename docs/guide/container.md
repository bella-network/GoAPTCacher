---
title: Container deployment
description: Run GoAPTCacher with Docker or Compose and keep the cache persistent.
outline: deep
---

# Container deployment

The release image is published as `registry.gitlab.com/bella.network/goaptcacher`. It is based on `scratch`, runs as the unprivileged `goaptcacher` user with UID/GID `65532`, and expects its configuration below `/config`.

## Prepare files

```bash
mkdir -p goaptcacher/cache
cd goaptcacher
curl -o config.yaml \
  https://gitlab.com/bella.network/goaptcacher/-/raw/main/config.yaml-example
sudo chown -R 65532:65532 cache
chmod 755 cache
chmod 644 config.yaml
```

Set `cache_directory: /var/cache/goaptcacher` in `config.yaml`. Review the domain policy before starting the container.

## Docker

```bash
docker run -d \
  --name goaptcacher \
  --restart unless-stopped \
  -p 8090:8090 \
  -p 8091:8091 \
  -p 3142:3142 \
  -v "$PWD/config.yaml:/config/config.yaml:ro" \
  -v "$PWD/cache:/var/cache/goaptcacher" \
  registry.gitlab.com/bella.network/goaptcacher:latest
```

Publish port `8091` only when HTTPS interception is enabled. Publish `3142` only when it appears in `alternative_ports`.

Inspect startup and the web interface:

```bash
docker logs goaptcacher
docker exec goaptcacher /goaptcacher --version
curl -i http://127.0.0.1:8090/_goaptcacher/
```

## Docker Compose

```yaml
services:
  goaptcacher:
    image: registry.gitlab.com/bella.network/goaptcacher:latest
    container_name: goaptcacher
    restart: unless-stopped
    ports:
      - "8090:8090"
      - "3142:3142"
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - ./cache:/var/cache/goaptcacher
    read_only: true
    tmpfs:
      - /tmp
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
```

GoAPTCacher writes only to `cache_directory`; a read-only container filesystem is therefore suitable when that directory is a writable mount.

## Configuration override

The image starts `/goaptcacher` with the default relative path `./config.yaml` from `/config`. To mount a differently named configuration, set `CONFIG`:

```yaml
environment:
  CONFIG: /config/production.yaml
```

`CACHE_DIR` overrides `cache_directory`, which is useful with a named volume:

```yaml
environment:
  CACHE_DIR: /data/cache
volumes:
  - cache-data:/data/cache
```

## Repository verification

The container image does not run systemd or the package timer. Execute verification from a scheduler on the host or a separate one-shot container using the same configuration and cache volume:

```bash
docker exec goaptcacher /goaptcacher verify-repos
```

The command reports mismatches but does not delete them. See [Repository verification](/operations/repository-verification).

## Upgrades

Pull and recreate the container while retaining both mounts:

```bash
docker pull registry.gitlab.com/bella.network/goaptcacher:latest
docker compose up -d
```

For reproducible environments, pin a release tag instead of `latest`. Check the logs after every upgrade and keep a backup of `config.yaml`.

## Common permission failure

If startup or the first download reports `permission denied`, verify the effective user and mount ownership:

```bash
docker inspect --format '{{.Config.User}}' goaptcacher
ls -ld cache
sudo chown -R 65532:65532 cache
```

Do not solve mount permissions by running the container as root.
