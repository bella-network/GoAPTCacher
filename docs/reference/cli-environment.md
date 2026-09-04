---
title: CLI and environment reference
description: GoAPTCacher command-line options, commands, environment variables, and exit behavior.
outline: deep
---

# CLI and environment reference

## Synopsis

```text
goaptcacher [options] [command]
```

Place options before the command:

```bash
goaptcacher --config /etc/goaptcacher/config.yaml verify-repos
```

## Options

| Short | Long | Argument | Description |
| --- | --- | --- | --- |
| `-h` | `--help` | none | Print usage and exit. |
| `-v` | `--version` | none | Print version, commit, and build timestamp, then exit. |
| `-c` | `--config` | file path | Select the YAML configuration file. |

Example version output:

```text
GoAPTCacher version 1.2.3, commit abcdef12, built at 2026-09-04T12:00:00Z
```

Exact values are injected by release builds. A local development build may show placeholder build information.

## Commands

### Default server mode

With no command, GoAPTCacher:

1. loads configuration;
2. initializes optional debugging and interception;
3. opens the cache and starts persistence loops;
4. starts cache expiration when configured;
5. starts HTTP, alternative, and optional TLS listeners;
6. announces mDNS when enabled;
7. runs until the process is terminated.

```bash
goaptcacher --config ./config.yaml
```

### `verify-repos`

```bash
goaptcacher --config ./config.yaml verify-repos
```

Scans cached repositories and reports metadata or package checksum mismatches. This mode does not start listeners. See [Repository verification](/operations/repository-verification).

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `CONFIG` | empty | Configuration path used when `--config`/`-c` is absent. |
| `CACHE_DIR` | empty | Overrides `cache_directory` after YAML parsing. |
| `INVOCATION_ID` | normally set by systemd | When non-empty, suppresses Go log timestamps and source-file prefixes. |

Configuration path precedence is:

1. `--config` or `-c`;
2. `CONFIG`;
3. `./config.yaml`.

`CACHE_DIR` is independent and always overrides the parsed field when non-empty.

## Package service environment

The systemd unit uses:

```ini
Environment="CONFIG=/etc/goaptcacher/config.yaml"
WorkingDirectory=/etc/goaptcacher
User=goaptcacher
Group=goaptcacher
```

The one-shot repository-verification service uses the same configuration and user.

## Container defaults

The image uses:

```text
USER goaptcacher:goaptcacher
WORKDIR /config
CMD ["/goaptcacher"]
```

Therefore `/config/config.yaml` is selected by the default relative path. Use `CONFIG` for another mounted path and `CACHE_DIR` for another writable volume.

## Exit behavior

Help, version, a successful no-op verification, and a completed verification without internal repository failures exit successfully. Configuration read failures, invalid commands, interception initialization errors, and repository processing failures exit non-zero.

Checksum mismatches are currently warnings and do not by themselves produce a non-zero exit. Parse verification output in monitoring automation.
