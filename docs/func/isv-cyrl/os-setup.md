# Развєртивање в Linux-у

Intermasq управјаје `dnsmasq` чез systemd, OpenRC, runit или sysvinit и чита
конфигурационе фајли и фајл аренди. Панел можни јест запускати од `root` или од
посебного уживатеља.

Од `root` команде се вызывајут непосредно. Од обичнога уживатеља управјенје
сервисом иде чез `sudo -n`; потребно дати чтение/запис `conf-dir`, чтение фајла
аренди и безпаролни приступ к конкретним командама supervisor-а.

```bash
sudo useradd -r -s /usr/sbin/nologin intermasq
sudo install -d -o intermasq -g intermasq -m 0750 /etc/intermasq
sudo install -d -o intermasq -g intermasq -m 0770 /run/intermasq/sockets
```

Init-систему можни јест изабрати флагом `-init-system systemd|systemd-user|openrc|runit|sysvinit|none`.
Подразумєвани пути сут `/etc/dnsmasq.d`, `/var/lib/misc/dnsmasq.leases`,
`/etc/intermasq/users.json`, `/etc/intermasq/history` и `/run/intermasq/sockets`.

Пример systemd:

```ini
[Service]
Type=simple
User=intermasq
Group=intermasq
ExecStart=/usr/local/bin/intermasq -port 8081 -conf-dir /etc/dnsmasq.d -leases /var/lib/misc/dnsmasq.leases
Environment="INTERMASQ_SECRET=CHANGE_ME"
Restart=on-failure
```

`INTERMASQ_SECRET` јест обавезни. При reverse proxy потребно правилно поставити
`X-Forwarded-For`; за SSE треба искључити buffering.
