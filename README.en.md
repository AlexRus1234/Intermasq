[English](README.en.md) | [Русский](README.md) | 

<div align="center">

<h1>Intermasq</h1>

**Web panel for dnsmasq management**

Intermasq is a self-contained web application for administering `dnsmasq`.
The frontend, server logic, and API are combined into a single executable.
Data is stored in the filesystem; no external database or container
infrastructure is required.

[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg?style=flat-square)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg?style=flat-square)](https://vuejs.org/)
[![Bootstrap](https://img.shields.io/badge/Bootstrap-5-7952B3.svg?style=flat-square)](https://getbootstrap.com/)
[![Platform](https://img.shields.io/badge/Linux-any-1793D1.svg?style=flat-square)](#quick-start)

</div>

---

## Contents

- [Screenshots](#screenshots)
- [Features](#features)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [Access control](#access-control)
- [API, plugins, and metrics](#api-plugins-and-metrics)
- [Project structure](#project-structure)
- [Technology stack](#technology-stack)
- [License](#license)

> Extended documentation for the API, access control, system services, plugins,
> and metrics is available in [`docs/func/EN/`](docs/func/EN/README.md).
> This file provides a system overview and initial setup instructions.

The project was developed according to a predefined architecture; an AI
assistant was used while preparing the source code.[^1]

---

## Screenshots

A few screens from the web panel in the English localization:

<p>
  <img src="скрин/en/Снимок%20экрана%202026-08-09%20152137.png" alt="Intermasq panel" width="49%">
  <img src="скрин/en/Снимок%20экрана%202026-08-09%20152151.png" alt="Intermasq settings" width="49%">
</p>
<p>
  <img src="скрин/en/Снимок%20экрана%202026-08-09%20152217.png" alt="dnsmasq configuration" width="49%">
  <img src="скрин/en/Снимок%20экрана%202026-08-09%20152228.png" alt="File management" width="49%">
</p>
<p>
  <img src="скрин/en/Снимок%20экрана%202026-08-09%20152241.png" alt="Device list" width="49%">
</p>

---

## Features

### DHCP and DNS
- Operations on `dhcp-host=` entries with MAC/IP/hostname, tag `set:`, and `lease-time` validation
- Suggestion of the next free IP from `dhcp-range`
- Host templates (IP range + hostname pattern + target file)
- `A` / `CNAME` / `PTR` / `TXT` DNS records with CSV import/export
- Lease viewer, online ARP devices, and bulk lease-to-static conversion
- Unknown ARP device detection with **vendor identification** (OUI)

### dnsmasq configuration
- Visual editor for `dhcp-range`, `dhcp-option` (RFC 2132 presets), `server=`, and PXE/network boot
- Raw `.conf` editor with `dnsmasq --test` validation
- Multiple files: create, delete, and use configuration presets (`basic-dhcp`, `forwarder`, `pxe`, `aliases`)

### Security and history
- Multi-level history (N versions per file) with diff and restore
- `.bak` rollback, ZIP backup, and restore with pre-validation
- Audit log: who did what and when, with colored labels
- Path traversal protection: writes are limited to `-conf-dir`

### Operations and user interface
- Single binary (`go:embed`), multi-init support: systemd / systemd-user / OpenRC / runit / sysvinit with auto-detection
- Real-time SSE updates for ARP and dnsmasq status without polling
- Dual authentication: JWT for browsers, `X-API-Key` for scripts and plugins
- **RBAC**: `admin` / `user` roles; destructive operations are admin-only
- Rate limiting on `/api/login`, JWT revocation on logout, and revocation of all tokens when a password changes or a user is deleted
- Unix-socket plugins, `/metrics` for Prometheus, and Swagger documentation
- Russian and English interface languages, dark and light themes

See [`docs/func/EN/features.md`](docs/func/EN/features.md) for details.

---

## Quick start

### Requirements

| Component | Version | Purpose |
|---|---|---|
| **Go** | 1.25+ | Build the binary |
| **Node.js** | 22+ | Build the frontend |
| **dnsmasq** | any | On the target machine |

### Build

```bash
# Build the frontend and server in the order used by CI:
make build

# Alternative manual build:
cd frontend && npm ci && npm run build && cd ..
go build -o intermasq .
```

Production build (static linking, without symbol tables, with a version):

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w \
  -X intermask/internal/version.Version=1.0.0" -o intermasq .
```

> Prebuilt binaries are not published in a public registry. Build from source.

### Run

```bash
# Required: the process exits at startup without this secret
export INTERMASQ_SECRET="$(openssl rand -hex 32)"

sudo ./intermasq \
  -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

On the first launch, the admin account creation form is displayed. After it is
completed, the panel becomes available.

> In production, set `INTERMASQ_SECRET` in a systemd unit drop-in with mode
> `0600`. A complete unit example and instructions for running as a dedicated
> user are available in [`docs/func/EN/os-setup.md`](docs/func/EN/os-setup.md).

---

## Configuration

### Command-line flags

| Flag | Default | Description |
|---|---|---|
| `-port` | `8081` | Listening port |
| `-conf-dir` | `/etc/dnsmasq.d` | dnsmasq configuration directory |
| `-leases` | `/var/lib/misc/dnsmasq.leases` | dnsmasq lease file |
| `-arp-file` | `/proc/net/arp` | ARP table path |
| `-db` | `/etc/intermasq/users.json` | User database |
| `-audit-log` | `/etc/intermasq/audit.log` | Audit log file |
| `-templates` | `/etc/intermasq/templates.json` | Host templates file |
| `-history-dir` | `/etc/intermasq/history` | Configuration version directory |
| `-history-depth` | `10` | Number of versions retained per file |
| `-init-system` | `auto` | `auto` / `systemd` / `systemd-user` / `openrc` / `runit` / `sysvinit` / `none` |
| `-ci-mode` | `false` | Disables self-restart (for CI/tests) |
| `-dnsmasq-bin`<br>`-sudo-bin`<br>`-systemctl-bin`<br>`-service-bin`<br>`-rc-service-bin`<br>`-sv-bin` | *(auto)* | Override paths to system binaries (`dnsmasq`, `sudo`, `systemctl`, `service`, `rc-service`, `sv`). Empty means resolve through `$PATH` and well-known absolute paths. See `internal/bins`. |
| `-systemd-scope` | -- | *(deprecated)* `auto`/`system`/`user`/`none`; mapped to `-init-system` |

### Environment variables

| Variable | Required | Description |
|---|---|---|
| `INTERMASQ_SECRET` | **Yes** | Secret used to sign JWTs and as the `X-API-Key` value. Generate it with `openssl rand -hex 32`. |

---

## Access control

System command execution is determined by `getuid()`:

- **Running as `root`**: `systemctl` and `dnsmasq --test` are called directly.
- **Running as a regular user**: service management uses `sudo -n`. Allow the
  required commands and grant read/write access to `conf-dir` and read access
  to the lease file.

Example `/etc/sudoers.d/intermasq` for systemd and user `intermasq`:

```sudoers
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl is-active dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl restart dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl restart intermasq
```

The startup log indicates the selected mode: `[INIT] System: systemd (root)`
or `[INIT] System: systemd (via sudo)`.

See [`docs/func/EN/os-setup.md`](docs/func/EN/os-setup.md) for sudo rules for
all supported init systems, filesystem permissions, a systemd unit example,
and dedicated-user deployment.

---

## API, plugins, and metrics

Interactive documentation is available after startup:

```
http://<host>:<port>/swagger/index.html
```

| Area | Summary | Details |
|---|---|---|
| **Authentication** | `Authorization: Bearer <JWT>` (browser) or `X-API-Key: <INTERMASQ_SECRET>` (scripts) | [`docs/func/EN/api.md`](docs/func/EN/api.md) |
| **Endpoints** | `/api/hosts`, `/api/aliases`, `/api/config`, `/api/files/:name`, `/api/history`, `/api/backup`, `/api/reload`, `/api/events`, ... | Full list and RBAC in [`api.md`](docs/func/EN/api.md) |
| **RBAC** | `admin` (reload/rollback/raw writes/users/restart) and `user` (read and add) | [`api.md`](docs/func/EN/api.md) |
| **Plugins** | Sidecar processes over Unix sockets, manifest in `/etc/intermasq/plugins/`, iframe proxying | [`docs/func/EN/plugins.md`](docs/func/EN/plugins.md) |
| **Metrics** | `/metrics` for Prometheus: hosts/leases/ARP/dnsmasq status/domain health checks | [`docs/func/EN/metrics.md`](docs/func/EN/metrics.md) |

---

## Project structure

```
.
├── main.go                 # Entry point: flags, initialization, Gin, static files, Swagger
├── internal/
│   ├── models/             # Data types (HostEntry, DnsAliasEntry, ...)
│   ├── validate/           # MAC/IP/hostname/tag validators and normalizers
│   ├── oui/                # OUI table (vendor lookup by MAC)
│   ├── stats/              # Counters for /metrics
│   ├── bins/               # Automatic system binary path resolution
│   ├── initd/              # SystemCaller: init detection and management
│   ├── dnsmasq/            # dhcp-host parsing/writing, aliases, config, history, backup
│   ├── netstate/           # ARP, leases, device discovery
│   ├── templates/          # Host template creation and application
│   ├── auth/               # Users, JWT, rate limiting, RBAC middleware (bcrypt)
│   ├── audit/              # Audit log
│   ├── control/            # SSE broadcaster, dnsmasq status/reload
│   ├── metrics/             # Prometheus /metrics and DNS health checks
│   ├── plugins/            # Plugin loading/proxying (Unix sockets)
│   ├── version/            # Build version (ldflags)
│   └── webapi/             # HTTP handlers and /api/* route registration
├── docs/                   # OpenAPI and user documentation
├── frontend/               # Vue 3 SPA (Vite, Bootstrap 5, vue-i18n)
├── .forgejo/workflows/     # CI: build, tests, smoke, optional fuzz/e2e/L5 VM
├── tests/                  # Smoke suites, Playwright E2E, performance, L5 VMs
├── LICENSE                 # GNU AGPL v3
└── README.md               # Main documentation
```

`main.go` is located in the root because `//go:embed frontend/dist/*` cannot
refer to parent directories. This keeps `go build -o intermasq .` working.

---

## Technology stack

**Backend:** Go 1.25, Gin, golang-jwt/v5, golang.org/x/crypto (bcrypt),
gin-swagger, `go:embed`.

**Frontend:** Vue 3 (Composition API), Vite 7, Bootstrap 5 (dark/light),
vue-i18n 9 (RU/EN), Axios, event-source-polyfill (SSE).

**Infrastructure and quality:** Forgejo Actions (CI), `go vet` / `gofmt`,
`go test` (including `-race`), fuzz targets, Playwright E2E, smoke suites,
and L5 tests on live VMs (systemd + OpenRC).

---

## License

The project is distributed under the **[GNU Affero General Public License v3.0](LICENSE)**.

```
Intermasq - Web panel for dnsmasq
Copyright (C) 2026  AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
```

[^1]: The source code was developed with an AI assistant according to a
predefined project architecture; the author made the architectural decisions,
verified the results, and performed the final integration.
