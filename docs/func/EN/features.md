<!--
Intermasq - Web panel for dnsmasq
Copyright (C) 2026 AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
-->

# Features

This document describes the main functional areas of the system. The root
`README.en.md` contains a short overview.

---

## DHCP: static hosts

- Create, read, update, and delete `dhcp-host=` records with MAC, IP, and hostname validation (`internal/validate`).
- Additional host fields:
  - `tags`: qualifiers after the IP, including `set:<tag>` for assigning `dhcp-option`, `id:<client-id>`, and other `dhcp-host` fields.
  - `lease_time`: lease duration suffix (`12h`, `3600`, `infinite`), preserved during bulk moves and editing.
- `GET /api/hosts/next-ip` suggests a free address from a known `dhcp-range`.
- Host templates contain a name, IP range, and target file; one action applies them to a selected MAC (`POST /api/hosts/apply-template`).

MAC format: `^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$` (both `:` and `-` are accepted).

## DNS records

- Supported types are **A** (`address=/domain/IP`), **CNAME** (`cname=alias,target`), **PTR**, and **TXT**.
- Standard operations, bulk import, and CSV import/export are supported.
- Domains configured as A/CNAME are periodically resolved by a background health checker and exposed as `intermasq_domain_up{domain=...}` metrics (see [metrics.md](metrics.md)).

## dnsmasq configuration

- The visual editor supports `dhcp-range`, `dhcp-option` (RFC 2132 presets), `server=` / `server=/domain/upstream`, and PXE/network boot.
- The text editor provides direct file editing. It is available only to the `admin` role (`PUT /api/files/:name` is admin-only).
- Multiple `.conf` files are supported: create, delete, and switch between tabs.
- Configuration presets (`GET /api/config/templates`) include `basic-dhcp`, `forwarder`, `pxe`, `aliases`, and `empty` when creating a file.

## Security and history

- A `.bak` backup is created before every write, allowing a one-step rollback (`POST /api/rollback`, admin).
- Multi-level history (`-history-dir`, `-history-depth`, default 10 versions per file):
  - `GET /api/history` lists versions, newest first.
  - `GET /api/history/diff` returns a unified diff between versions or against the current file.
  - `POST /api/history/restore` restores a version (admin). The current state is also saved, so the restore can be undone.
  - Version names are `<sha256-path>_YYYYMMDD-HHMMSS[-N].bak`. Path traversal is impossible because the version and path are validated.
- Backup and restore of all `.conf` files uses ZIP with a `dnsmasq --test` pre-check and automatic rollback on failure.
- `IsSafePath` prevents operations outside `-conf-dir`; text-editor names must end in `.conf` and contain no separators.
- `dnsmasq --test` runs before every change is applied. On failure the file is restored and `intermasq_dnsmasq_test_failures_total` is incremented.

> Exception: `POST /api/leases/to-static` intentionally skips `dnsmasq --test`
> for bulk-operation speed. The UI displays a yellow warning and suggests
> clicking Apply.

## Bulk operations

- Bulk host and DNS import in text or CSV format.
- **Bulk move**: move hosts to another `.conf`.
- **Bulk edit**: replace an IP prefix, or add/remove a hostname suffix.
- **Lease to static**: bulk conversion of leases to static entries.
- **Bulk deletion** using checkboxes.
- **CSV import/export** for hosts and DNS records.

## Device discovery

- New devices are MAC addresses found in ARP but absent from static records and leases.
- **OUI lookup** using the built-in table (`internal/oui`) identifies vendors from the first three MAC octets (Apple, Cisco, Netgear, Raspberry Pi, QEMU/KVM, and others).
- The Add button opens the static form with the MAC pre-filled.

## Operations and user interface

- The frontend is embedded into the executable with `go:embed frontend/dist/*`; no extra components are required on the target machine.
- Automatic detection of systemd / systemd-user / OpenRC / runit / sysvinit is supported (see [os-setup.md](os-setup.md)).
- The SSE stream (`GET /api/events`) sends ARP and dnsmasq status changes only when they change. The broadcaster is non-blocking: a slow client skips an update and does not block other connections.
- JWT is supported for browsers and `X-API-Key` for scripts and plugins.
- RBAC hides administrative operations (reload, rollback, raw writes, users, restart) from the `user` role.
- Interactive Swagger documentation is available at `/swagger/index.html`.
- The interface supports Russian and English.
- Dark and light themes are available, with the selected mode persisted.
- The audit log records user actions (user creation and deletion, raw edits, restore, backup, and more).
