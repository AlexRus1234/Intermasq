**English** | [**Русский**](README.md)

<div align="center">

# 🛡️ Intermasq

**Web panel for dnsmasq management**

Lightweight, fast, single binary — everything included.

[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg)](https://vuejs.org/)

</div>

---

## 📸 Screenshots

> *Screenshots will be added later.*

---

## ✨ Key Features

- **Static DHCP entry management** — add, edit, delete hosts via web interface
- **Device monitoring** — online/offline status via ARP table (auto-refresh every 30 sec)
- **Lease-to-static conversion** — promote a DHCP lease to a static entry in one click
- **Bulk import** — paste a list of devices from text (MAC IP Hostname)
- **Backup & rollback** — download ZIP archive of configs and rollback changes per file (`.bak`)
- **Multi-init** — auto-detection and support: systemd, systemd-user, OpenRC, runit, sysvinit
- **Plugin system** — extend functionality via Unix sockets
- **Single binary** — frontend embedded into Go binary via `go:embed`
- **Bilingual interface** — Russian / English, switch in one click
- **Dark / light theme** — choice persisted in localStorage
- **Dual authentication** — JWT for browsers, `X-API-Key` for scripts and plugins
- **Swagger UI** — interactive API documentation out of the box (`/swagger/index.html`)
- **Bulk deletion** — select multiple hosts with checkboxes and delete at once
- **Sorting & search** — by MAC, IP, Hostname with smart IP-address sorting
- **Path safety** — path traversal protection, writes only inside `conf-dir`

---

## 🚀 Quick Start

### Requirements

- Go 1.25+
- Node.js 22+ (for frontend build)
- dnsmasq (on the target machine)

### Build

```bash
# Build frontend
cd frontend && npm ci && npm run build && cd ..

# Build binary
go build -o intermasq .
```

### Run

```bash
sudo ./intermasq -port 8081 -conf-dir /etc/dnsmasq.d -leases /var/lib/misc/dnsmasq.leases
```

On first launch, the admin setup screen appears. After creating an account — full access to the panel.

### Production build (with version)

```bash
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=3.0.0" -o intermasq .
```

---

## ⚙️ CLI Flags

| Flag | Default | Description |
|---|---|---|
| `-port` | `8081` | Port to listen on |
| `-db` | `/etc/intermasq/users.json` | Path to user database |
| `-conf-dir` | `/etc/dnsmasq.d` | Directory with dnsmasq configs |
| `-leases` | `/var/lib/misc/dnsmasq.leases` | Path to dnsmasq leases file |
| `-arp-file` | `/proc/net/arp` | Path to ARP table file |
| `-init-system` | `auto` | Init system: `auto`, `systemd`, `systemd-user`, `openrc`, `runit`, `sysvinit`, `none` |
| `-systemd-scope` | — | *(deprecated)* `auto`, `system`, `user`, `none` |
| `-ci-mode` | `false` | Disables self-restart (for CI) |
| `-audit-log` | `/etc/intermasq/audit.log` | Path to audit log file |
| `-templates` | `/etc/intermasq/templates.json` | Path to templates file |

> **Default port note:** As of v3.0 the default port changed from `8080` to `8081`. Port `8080` is frequently occupied by other services (e.g. Crowdsec listens on `127.0.0.1:8080`), which caused binding conflicts and silent service crashes. To use the old port, pass it explicitly via `-port 8080`.

### Environment Variables

| Variable | Description |
|---|---|
| `INTERMASQ_SECRET` | Secret key for JWT and API-Key. If not set — default key is used |

---

## 🔌 API

After launch, Swagger documentation is available at: `http://<host>:<port>/swagger/index.html`

### Core Endpoints

| Method | Path | Description | Auth |
|---|---|---|---|
| `GET` | `/api/status` | dnsmasq status + setup requirement | No |
| `POST` | `/api/setup` | Initial admin setup | No |
| `POST` | `/api/login` | Login, get JWT | No |
| `GET` | `/api/hosts` | List static hosts | Yes |
| `POST` | `/api/hosts` | Add/update a host | Yes |
| `POST` | `/api/hosts/bulk` | Bulk import hosts | Yes |
| `DELETE` | `/api/hosts/:mac` | Delete a host | Yes |
| `GET` | `/api/leases` | DHCP leases | Yes |
| `GET` | `/api/arp` | ARP table (online MACs) | Yes |
| `POST` | `/api/reload` | Validate config + restart dnsmasq | Yes |
| `POST` | `/api/rollback` | Rollback file to `.bak` version | Yes |
| `GET` | `/api/backup` | Download ZIP archive of all `.conf` | Yes |
| `POST` | `/api/restart-self` | Restart Intermasq service | Yes |
| `GET` | `/api/plugins` | List loaded plugins | Yes |

### Authentication

- **Browser**: JWT token in `Authorization: Bearer <token>` header
- **Scripts/plugins**: static key in `X-API-Key: <INTERMASQ_SECRET>` header

---

## 🧩 Plugin System

Intermasq supports extensions via Unix sockets. Each plugin is a directory in `/etc/intermasq/plugins/` with a `manifest.json`:

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "bin": "./plugin-binary"
}
```

On startup, Intermasq:
1. Reads `/etc/intermasq/plugins/<id>/manifest.json`
2. Launches the binary with environment variables:
   - `INTERMASQ_KEY` — secret for API requests
   - `PLUGIN_SOCKET` — path to Unix socket (`/run/intermasq/sockets/<id>.sock`)
3. Proxies all requests to `/plugins/<id>/*` to the plugin's Unix socket
4. UI renders the plugin in an iframe

---

## 📁 Project Structure

```
.
├── main.go              # Entry point, Gin routing, plugin loading
├── auth.go              # JWT, authentication, users (bcrypt)
├── handlers.go          # HTTP API handlers
├── models.go            # Data structures (HostEntry, LeaseEntry, etc.)
├── dnsmasq.go           # dnsmasq config handling, ARP, backups, rollback
├── system.go            # Init system abstraction (SystemCaller interface)
├── dnsmasq_test.go      # Unit tests (ARP, paths, init systems)
├── go.mod / go.sum      # Go dependencies
├── docs/
│   ├── swagger.yaml     # OpenAPI specification
│   ├── swagger.json     # OpenAPI specification (JSON)
│   └── docs.go          # Generated code for gin-swagger
├── frontend/
│   ├── package.json     # Node.js dependencies
│   ├── vite.config.js   # Vite configuration
│   ├── index.html       # HTML entry point
│   └── src/
│       ├── main.js          # Vue + i18n initialization
│       ├── App.vue          # Root component (navbar, tabs, themes)
│       ├── store.js         # Reactive store + API client (axios)
│       ├── i18n.js          # vue-i18n setup + API error translation
│       ├── style.css        # Global styles
│       ├── locales/
│       │   ├── ru.json      # Russian locale
│       │   └── en.json      # English locale
│       └── components/
│           ├── AuthScreen.vue       # Login / setup screen
│           ├── static/
│           │   ├── StaticView.vue  # Static hosts tab
│           │   ├── HostForm.vue     # Add/edit form + bulk import
│           │   ├── HostTable.vue    # Hosts table (sorting, selection)
│           │   └── BulkImport.vue   # Bulk import component
│           └── leases/
│               └── LeasesTab.vue    # DHCP leases tab
├── .forgejo/
│   └── workflows/
│       ├── build.yml       # CI: build, tests, smoke test
│       └── release.yml     # Release: build, sha256, upload to Forgejo Packages
├── LICENSE               # GNU AGPL v3
└── README.md             # Documentation
```

---

## 🛠 Tech Stack

### Backend
- **Go 1.25** — language, single binary
- **Gin** — HTTP framework
- **jwt/v5** — JWT tokens
- **bcrypt** (golang.org/x/crypto) — password hashing
- **gin-swagger** — Swagger UI
- **go:embed** — frontend embedding into binary

### Frontend
- **Vue 3** — Composition API, `<script setup>`
- **Vite 7** — build tool
- **Bootstrap 5** — UI components and theming
- **vue-i18n 9** — localization (RU / EN)
- **Axios** — HTTP client

### Infrastructure
- **Forgejo Actions** — CI/CD
- **Go vet + gofmt** — static analysis and formatting
- **go test** — unit tests

---

## 📄 License

This project is licensed under the [GNU Affero General Public License v3.0](LICENSE).

Copyright (C) 2026 AlexRus1234
