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
