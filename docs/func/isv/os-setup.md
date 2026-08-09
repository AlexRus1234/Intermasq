# Razvêrtyvanje v Linuxu

Intermasq upravlja `dnsmasq` črez systemd, OpenRC, runit ili sysvinit i čita
konfiguracione fajly i fajl arend. Možno jest zapuskati panel od `root` ili od
posebnogo użytkateľa.

Od `root` komandy se vyzyvajut neposredno. Od obyčnogo użytkateľa upravljenje
servisom idet črez `sudo -n`; potrebno dati čtenje/zapis `conf-dir`, čtenje fajla
arend i bezparolny pristup k konkretnym komandam supervisor-a.

```bash
sudo useradd -r -s /usr/sbin/nologin intermasq
sudo install -d -o intermasq -g intermasq -m 0750 /etc/intermasq
sudo install -d -o intermasq -g intermasq -m 0770 /run/intermasq/sockets
```

Init-sistemu možno izbrati flagom `-init-system systemd|systemd-user|openrc|runit|sysvinit|none`.
Default puti sut `/etc/dnsmasq.d`, `/var/lib/misc/dnsmasq.leases`,
`/etc/intermasq/users.json`, `/etc/intermasq/history` i `/run/intermasq/sockets`.

Za systemd primer:

```ini
[Service]
Type=simple
User=intermasq
Group=intermasq
ExecStart=/usr/local/bin/intermasq -port 8081 -conf-dir /etc/dnsmasq.d -leases /var/lib/misc/dnsmasq.leases
Environment="INTERMASQ_SECRET=CHANGE_ME"
Restart=on-failure
```

`INTERMASQ_SECRET` jest obavezny. Pri reverse proxy potrebno pravilno nastaviti
`X-Forwarded-For`; za SSE treba isključiti buffering.
