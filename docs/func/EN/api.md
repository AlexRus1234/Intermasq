# API, Authentication, and Access Control

Intermasq provides a Gin-based REST API. The base path is `/api`; Swagger UI
is available at `http://<host>:<port>/swagger/index.html`, and the
specification is in `docs/swagger.yaml` / `docs/swagger.json`.

---

## Authentication

Protected endpoints support two authentication methods:

| Scenario | Method | Actor |
|---|---|---|
| Browser | `Authorization: Bearer <JWT>` | User logged in through `/api/login` |
| Scripts / plugins / Prometheus | `X-API-Key: <INTERMASQ_SECRET>` | Programmatic access |

`X-API-Key` is the same value as `INTERMASQ_SECRET` (the secret that signs
JWTs). A successful request runs as the virtual `api-key` user with the
`admin` role.

### JWT

- Algorithm: **HS256**, key: `INTERMASQ_SECRET`.
- Token lifetime: **72 hours**.
- Claims: `sub` (username), `exp`, `jti`, `ver` (revocation version), and `role` (`admin`/`user`).

### Token revocation

- **Logout** (`POST /api/logout`) stores the `jti` in an in-memory blacklist until `exp`.
- **Password change / user deletion** increments `ver` for that user; all previously issued tokens become invalid immediately.
- **Process restart** clears the in-memory blacklist. Tokens revoked through logout become valid again until `exp`. This is intentional simplification.

### `/api/login` rate limiting

Ten attempts per IP per minute are allowed; subsequent requests receive
`429 too_many_attempts`. The counter is reset after a **successful** login, so
an attacker without the password cannot reset it. Storage is in-memory and
cleanup is lazy (every five minutes).

---

## Access control

Roles are stored in `users.json`. The first user created through `/api/setup`
becomes `admin`; later users created through `/api/users` become `user`.

| Level | Middleware | Permissions |
|---|---|---|
| `auth` | `auth.Middleware` | Any authenticated user: read and add hosts/DNS, templates, history (view/diff), backup download, audit, and CSV. |
| `admin` | `auth.AdminMiddleware` | Also destructive operations: reload, rollback, restore from history/ZIP, raw file writes, user management, and restart-self. |

The server is the source of truth for permissions. The UI hides administrative
elements for `user` based on the JWT `role` claim, but every admin request is
checked again on the server. Forging a client-side role can only change the UI,
not grant privileges.

---

## Endpoints

### Public endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/status` | dnsmasq status and initial-setup flag (whether any user exists) |
| `POST` | `/api/setup` | Create the first admin (only when the database is empty) |
| `POST` | `/api/login` | Log in and return a JWT; rate-limited |

### Hosts (`dhcp-host`), `auth` level

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/hosts` | List static hosts |
| `POST` | `/api/hosts` | Add or update a host |
| `DELETE` | `/api/hosts/:mac` | Delete a host |
| `GET` | `/api/hosts/next-ip` | Suggest the next free IP in a range |
| `POST` | `/api/hosts/apply-template` | Apply an IP/hostname-pattern template to a MAC |
| `POST` | `/api/hosts/bulk` | Bulk import hosts |
| `POST` | `/api/hosts/bulk-move` | Move hosts to another `.conf` |
| `POST` | `/api/hosts/bulk-edit` | Bulk edit (IP prefix, hostname suffix) |
| `GET` | `/api/hosts/csv` | Export hosts to CSV |
| `POST` | `/api/hosts/csv` | Import hosts from CSV |

### DNS records (A/CNAME/PTR/TXT), `auth` level

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/aliases` | List DNS records |
| `POST` | `/api/aliases` | Add a DNS record |
| `POST` | `/api/aliases/bulk` | Bulk import DNS records |
| `POST` | `/api/aliases/delete` | Delete a DNS record |
| `GET` | `/api/aliases/csv` | Export DNS to CSV |
| `POST` | `/api/aliases/csv` | Import DNS from CSV |

### Leases, ARP, and device discovery, `auth` level

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/leases` | DHCP leases from the `-leases` file |
| `GET` | `/api/arp` | ARP table (online MAC -> bool) |
| `GET` | `/api/new-devices` | Unknown MACs with vendor identification (OUI) |
| `POST` | `/api/leases/to-static` | Bulk lease-to-static conversion (without `dnsmasq --test`!) |

> `POST /api/leases/to-static` writes lines directly to the file without
> running `dnsmasq --test` for bulk-operation speed. Activate the changes with
> the regular Apply action (`POST /api/reload`).

### dnsmasq configuration, `auth` and `admin` levels

| Method | Path | Level | Description |
|---|---|---|---|
| `GET` | `/api/config` | auth | Snapshot of all directives (dhcp-range/option/server/PXE/...) |
| `PUT` | `/api/config` | auth | Update file directives (visual editor) |
| `POST` | `/api/config/file` | auth | Create a new `.conf` (optionally from a template) |
| `DELETE` | `/api/config/file` | auth | Delete a `.conf` physically |
| `GET` | `/api/config/templates` | auth | List configuration presets (basic-dhcp, forwarder, pxe, ...) |
| `GET` | `/api/files/:name` | auth | Read a raw `.conf` |
| `PUT` | `/api/files/:name` | **admin** | Write a raw `.conf` (plain-text editor) |

### Host templates, `auth` level

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/templates` | List templates (IP range + hostname pattern + target file) |
| `POST` | `/api/templates` | Create a template |
| `DELETE` | `/api/templates/:id` | Delete a template |
| `GET` | `/api/templates/ranges` | Known `dhcp-range=` values for target-range selection |

### History, backup, and reload

| Method | Path | Level | Description |
|---|---|---|---|
| `GET` | `/api/history` | auth | List file versions |
| `GET` | `/api/history/diff` | auth | Diff between versions or a version and the current file |
| `POST` | `/api/history/restore` | **admin** | Restore a file from a version |
| `POST` | `/api/rollback` | **admin** | Quickly roll a file back to its `.bak` (one step) |
| `GET` | `/api/backup` | auth | Download a ZIP archive of all `.conf` files |
| `POST` | `/api/backup/restore` | **admin** | Restore from ZIP (with `dnsmasq --test` pre-flight) |
| `POST` | `/api/reload` | **admin** | `dnsmasq --test` plus service restart |

### Users and sessions

| Method | Path | Level | Description |
|---|---|---|---|
| `GET` | `/api/users` | **admin** | List users |
| `POST` | `/api/users` | **admin** | Create a user (role `user`) |
| `DELETE` | `/api/users/:name` | **admin** | Delete a user (cannot delete yourself) |
| `POST` | `/api/users/password` | auth | Change your password (revokes your tokens) |
| `POST` | `/api/logout` | auth | Log out and revoke the current JWT |

### Operational endpoints

| Method | Path | Level | Description |
|---|---|---|---|
| `GET` | `/api/events` | auth | SSE stream: `arp` and `dnsmasq_status` events |
| `GET` | `/api/audit` | auth | Audit log (who/what/when, with colored labels) |
| `GET` | `/api/plugins` | auth | List loaded plugins |
| `POST` | `/api/restart-self` | **admin** | Restart the Intermasq service through the supervisor |

### Endpoints outside `/api`

| Method | Path | Description |
|---|---|---|
| `GET` | `/metrics` | Prometheus metrics. Handler auth: `Authorization: Bearer <JWT>` or `X-API-Key: <SECRET>` |
| `GET` | `/plugins/<id>/*` | Reverse proxy to a plugin Unix socket, protected by `auth.Middleware` |
| `GET` | `/swagger/*any` | Swagger UI (no auth) |

---

## Examples

### Login and access a host through JWT

```bash
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"secret"}' | jq -r .token)

curl -s http://localhost:8081/api/hosts \
  -H "Authorization: Bearer $TOKEN"
```

### The same through `X-API-Key` (for scripts)

```bash
curl -s http://localhost:8081/api/hosts \
  -H "X-API-Key: $INTERMASQ_SECRET"
```

### Add a host

```bash
curl -s -X POST http://localhost:8081/api/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.50","hostname":"iot","file":"/etc/dnsmasq.d/hosts.conf"}'
```
