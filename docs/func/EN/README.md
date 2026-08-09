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

# Intermasq Documentation

This directory contains Intermasq functional documentation. The root
`README.en.md` provides a system overview and initial setup instructions;
details requiring separate treatment are documented here.

| File | Contents |
|---|---|
| [os-setup.md](os-setup.md) | Linux deployment, permissions, sudo, init systems, systemd unit, and directories. |
| [api.md](api.md) | Endpoints, authentication (JWT and X-API-Key), and access control (RBAC). |
| [features.md](features.md) | DHCP/DNS features, configuration editing, history, bulk operations, and security. |
| [plugins.md](plugins.md) | Plugin system, manifest, environment, reverse proxy, and lifecycle. |
| [metrics.md](metrics.md) | `/metrics`, DNS reachability checks, and Prometheus alert examples. |

Historical version documents are in [`docs/`](../../), including
`v1.0-features.md`, `v3.1-features.md`, `version-history.md`, and
`new-features.md`. This directory contains the current user documentation.
