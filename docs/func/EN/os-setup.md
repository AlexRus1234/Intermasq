# Linux Deployment

Intermasq manages `dnsmasq` through a system supervisor (systemd, OpenRC,
runit, or sysvinit), and reads and changes configuration and lease files.
Deployment therefore requires choosing the process user and the way supervisor
commands are invoked.

---

## Main modes

- **Run as `root`.** `sudo` is not used; the panel calls `systemctl` and
  `dnsmasq --test` directly.
- **Run as a dedicated user.** This requires:
  1. read/write access to `conf-dir` and read access to the lease file;
  2. passwordless sudo for service-management commands (see below).

The invocation method is detected automatically: when `getuid() == 0`, calls
are direct; otherwise they use `sudo -n`. The `-sudo-bin` flag is only needed
to override the sudo path on distributions where it is not in `$PATH`.

---

## Choosing the invocation method

The logic is in `internal/initd/system.go`
(`detectSystemCaller`, `ResolveSystemCaller`):

| Condition | Behavior |
|---|---|
| Process runs as `root` (`getuid()==0`) | `systemctl` / `rc-service` / `sv` / `service` are called **directly**, without `sudo`. |
| Process runs as a regular user | The same commands are called through `sudo -n` (non-interactive). |
| `systemd-user` explicitly selected with `-init-system systemd-user` | Always `systemctl --user`, without `sudo`. |
| `-init-system none` | Service management is disabled (`/api/reload` and `/api/restart-self` become no-op/error). |

With `auto` (the default), systemd additionally checks whether `sudo -n
systemctl is-active dnsmasq` works, then tries `systemctl --user` and finally
falls back to sudo.

Commands run through the supervisor (systemd example):

```
systemctl is-active dnsmasq
systemctl restart  dnsmasq
systemctl restart  intermasq      # only for POST /api/restart-self
```

`dnsmasq --test` is always run without sudo; the required permissions are
described below.

---

## Running as `root`

No sudo configuration is required:

```bash
sudo ./intermasq -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

The log will contain `[INIT] System: systemd (root)`. A systemd unit also runs
as root; see the example below.

---

## Running as a dedicated user

Create a separate account and grant it only the required permissions.

### 2.1. File permissions

```bash
# user and group
sudo useradd -r -s /usr/sbin/nologin intermasq

# dnsmasq configs: read and write
sudo chown -R root:intermasq /etc/dnsmasq.d
sudo chmod 2775 /etc/dnsmasq.d          # setgid: new files inherit the group
sudo chmod g+rw /etc/dnsmasq.d/*.conf

# panel state
sudo install -d -o intermasq -g intermasq -m 0750 /etc/intermasq
sudo install -d -o intermasq -g intermasq -m 0750 /etc/intermasq/history
sudo install -d -o intermasq -g intermasq -m 0770 /run/intermasq/sockets

# lease file: read-only
sudo chmod g+r /var/lib/misc/dnsmasq.leases
sudo chgrp intermasq /var/lib/misc/dnsmasq.leases
```

`/proc/net/arp` is readable by everyone and requires no additional permissions.

### 2.2. Passwordless sudo

The panel calls the supervisor through `sudo -n`, without an interactive
password prompt. For user `intermasq`, allow exactly the commands it invokes,
without a password.

#### systemd

```bash
sudo visudo -f /etc/sudoers.d/intermasq
```

```sudoers
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl is-active dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl restart dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl restart intermasq
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl is-active intermasq
```

> Check the actual path: Debian/Ubuntu usually use `/usr/bin/systemctl`, while
> Alpine and older systems may use `/bin/systemctl`. The panel resolves the
> binary through `$PATH` and well-known paths; use `-systemctl-bin` to override
> it. In sudoers, use the absolute path reported by `which systemctl`.

#### OpenRC

```sudoers
intermasq ALL=(root) NOPASSWD: /usr/bin/rc-service dnsmasq status
intermasq ALL=(root) NOPASSWD: /usr/bin/rc-service dnsmasq restart
intermasq ALL=(root) NOPASSWD: /usr/bin/rc-service intermasq status
intermasq ALL=(root) NOPASSWD: /usr/bin/rc-service intermasq restart
```

#### runit

```sudoers
intermasq ALL=(root) NOPASSWD: /usr/bin/sv status /etc/service/dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/sv restart /etc/service/dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/sv status /etc/service/intermasq
intermasq ALL=(root) NOPASSWD: /usr/bin/sv restart /etc/service/intermasq
```

#### sysvinit

```sudoers
intermasq ALL=(root) NOPASSWD: /usr/sbin/service dnsmasq status
intermasq ALL=(root) NOPASSWD: /usr/sbin/service dnsmasq restart
intermasq ALL=(root) NOPASSWD: /usr/sbin/service intermasq status
intermasq ALL=(root) NOPASSWD: /usr/sbin/service intermasq restart
```

After setup, the log should contain `[INIT] System: systemd (via sudo)`.

### 2.3. Manual check

```bash
sudo -u intermasq sudo -n systemctl is-active dnsmasq
# expected: active/inactive (not a password prompt or Permission denied)
```

---

## Explicit init-system selection

With `auto`, the panel reads `/proc/1/comm` and checks for available binaries.
If auto-detection is unsuitable, select one explicitly:

```bash
-init-system systemd      # systemd (system scope), sudo if not root
-init-system systemd-user # systemctl --user (user-session daemons)
-init-system openrc       # Alpine/Artix: rc-service
-init-system runit        # Void Linux: sv /etc/service/<name>
-init-system sysvinit     # Devuan/older systems: service <name> ...
-init-system none         # disable service management
```

The deprecated `-systemd-scope` flag (`auto`/`system`/`user`/`none`) still
works and maps to the new values (`system` -> `systemd`, `user` ->
`systemd-user`), but prints a warning.

---

## Default paths and directories

| Purpose | Default path | Flag |
|---|---|---|
| dnsmasq configs | `/etc/dnsmasq.d` | `-conf-dir` |
| DHCP leases | `/var/lib/misc/dnsmasq.leases` | `-leases` |
| ARP table | `/proc/net/arp` | `-arp-file` |
| User database | `/etc/intermasq/users.json` | `-db` |
| Audit log | `/etc/intermasq/audit.log` | `-audit-log` |
| Host templates | `/etc/intermasq/templates.json` | `-templates` |
| Configuration history | `/etc/intermasq/history` | `-history-dir` |
| Plugins | `/etc/intermasq/plugins` | (`internal/plugins`) |
| Plugin sockets | `/run/intermasq/sockets` | (`internal/plugins`) |

History and socket directories are created automatically at startup.

---

## Example systemd unit

### As root

```ini
# /etc/systemd/system/intermasq.service
[Unit]
Description=Intermasq - Web panel for dnsmasq
After=network.target dnsmasq.service

[Service]
Type=simple
ExecStart=/usr/local/bin/intermasq \
  -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
Environment="INTERMASQ_SECRET=3f5c8a2bCHANGE_ME_3f5c8a2b"
Restart=on-failure
# Privileged port: use AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
```

### As user `intermasq` (with sudo)

```ini
[Service]
Type=simple
User=intermasq
Group=intermasq
ExecStart=/usr/local/bin/intermasq \
  -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
Environment="INTERMASQ_SECRET=3f5c8a2bCHANGE_ME_3f5c8a2b"
Restart=on-failure
```

> Set the secret in a drop-in rather than the main unit file:
> ```bash
> sudo systemctl edit intermasq
> # in the editor:
> # [Service]
> # Environment="INTERMASQ_SECRET=<openssl rand -hex 32>"
> ```
> Set the drop-in mode to `0600` and keep it out of git.

Manage the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now intermasq
sudo journalctl -u intermasq -f
```

---

## `INTERMASQ_SECRET`

Without this variable, the process exits at startup:

```
[FATAL] INTERMASQ_SECRET environment variable is not set.
        Generate one with:  openssl rand -hex 32
```

The secret is used to sign JWTs and as the `X-API-Key` value for scripts and
plugins. Generate and set it through the environment:

```bash
export INTERMASQ_SECRET="$(openssl rand -hex 32)"
```

---

## Reverse proxy

No additional panel configuration is needed with nginx or Caddy. To determine
the real IP correctly for rate limiting (`/api/login`) and auditing, configure
trusted proxies. Gin reads `X-Forwarded-For`, and `c.RemoteIP()` is used by the
limiter. SSE (`EventSource`) uses ordinary HTTP and is proxied transparently.

```nginx
location / {
    proxy_pass http://127.0.0.1:8081;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    # for SSE /api/events
    proxy_buffering off;
    proxy_read_timeout 1h;
}
```
