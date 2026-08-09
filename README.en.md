**English** | [**Русский**](README.md)

# Intermasq

Web panel for managing [dnsmasq](https://thekelleys.org.uk/dnsmasq/doc.html).
Intermasq is distributed as a single Go binary with the Vue frontend embedded
inside it. No external database is required.

[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8.svg)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg)](https://vuejs.org/)

## Features

- Static DHCP host management, tags, lease time, CSV and bulk operations.
- DHCP leases, ARP status, unknown-device discovery and OUI vendor lookup.
- DNS aliases: `address=`, `cname=`, `ptr-record=` and `txt-record=`.
- Visual and raw `.conf` editors, templates and free IPv4 selection.
- `dnsmasq --test` validation, `.bak` rollback, version history, diff and ZIP backup/restore.
- JSON Lines audit log, JWT/API-key authentication and RBAC.
- SSE status updates, Prometheus metrics and Unix-socket plugins.
- systemd, systemd-user, OpenRC, runit and sysvinit integration.
- Russian/English UI with light and dark themes.

## Quick Start

Requirements: Linux on the target host, Go 1.25+, Node.js/npm for building the
frontend, and `dnsmasq` for configuration and service operations.

```bash
make build
export INTERMASQ_SECRET="$(openssl rand -hex 32)"
sudo ./intermasq \
  -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

`INTERMASQ_SECRET` is mandatory. The default listener is `:8081`. Open
`http://<host>:8081` and create the first administrator account.

`make build` rebuilds the frontend before `go build`. If building manually:

```bash
cd frontend && npm ci && npm run build && cd ..
go build -o intermasq .
```

## Configuration

Important flags:

| Flag | Default |
|---|---|
| `-port` | `8081` |
| `-db` | `/etc/intermasq/users.json` |
| `-conf-dir` | `/etc/dnsmasq.d` |
| `-leases` | `/var/lib/misc/dnsmasq.leases` |
| `-arp-file` | `/proc/net/arp` |
| `-init-system` | `auto` |
| `-ci-mode` | `false` |
| `-audit-log` | `/etc/intermasq/audit.log` |
| `-templates` | `/etc/intermasq/templates.json` |
| `-history-dir` | `/etc/intermasq/history` |
| `-history-depth` | `10` |

Binary path overrides are available as `-dnsmasq-bin`, `-sudo-bin`,
`-systemctl-bin`, `-service-bin`, `-rc-service-bin` and `-sv-bin`.

## Authentication and API

Use `Authorization: Bearer <JWT>` for browser/API sessions or
`X-API-Key: <INTERMASQ_SECRET>` for scripts and plugins. `/metrics` also
accepts `?token=` for Prometheus clients that cannot set custom headers.

Swagger UI: `http://<host>:<port>/swagger/index.html`.

OpenAPI files: [`docs/swagger.yaml`](docs/swagger.yaml) and
[`docs/swagger.json`](docs/swagger.json). The generated specification does not
yet describe every current route; detailed feature documentation is in `docs/`.

Admin-only operations include raw config writes, reload, rollback, restore,
backup restore, user management and service restarts. The backend enforces RBAC.

## Testing

```bash
go test ./...
go test -race ./...
go vet ./...
```

See [`tests/ROADMAP.md`](tests/ROADMAP.md) for smoke, Playwright, fuzz,
compatibility, performance and real-VM test layers.

## Documentation

- [`docs/raw-editor-and-rbac.md`](docs/raw-editor-and-rbac.md)
- [`docs/dnsmasq-config.md`](docs/dnsmasq-config.md)
- [`docs/dns-aliases.md`](docs/dns-aliases.md)
- [`docs/bulk-ops-and-templates.md`](docs/bulk-ops-and-templates.md)
- [`docs/version-history.md`](docs/version-history.md)
- [`docs/portability-and-validation.md`](docs/portability-and-validation.md)

## License

[GNU Affero General Public License v3.0](LICENSE).

Copyright (C) 2026 AlexRus1234
