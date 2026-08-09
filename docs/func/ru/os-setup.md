# Запуск под Linux: права, sudo, init-системы

Intermasq управляет `dnsmasq` через системного супервизора (systemd / OpenRC /
runit / sysvinit) и читает/пишет конфиги и аренды на диске. Отсюда два вопроса:
**от какого пользователя запускать** и **как панель дотягивается до systemctl /
rc-service**. Ниже — оба режима и точные правила.

---

## TL;DR

- **Самый простой путь — запустить от `root`.** Тогда sudo не нужен вообще,
  панель вызывает `systemctl` / `dnsmasq --test` напрямую.
- **Запуск от отдельного пользователя** возможен, но требует:
  1. прав на чтение/запись `conf-dir` и чтение файла аренд;
  2. passwordless-sudo на команды управления сервисом (см. ниже).

Панель **сама решает**, нужен ли `sudo`: если `getuid() == 0` — вызовы идут
напрямую, иначе — через `sudo -n`. Флаг `-sudo-bin` нужен только чтобы
переопределить путь к `sudo` на дистрибутивах, где он лежит не в `$PATH`.

---

## Как панель выбирает способ вызова

Логика живёт в `internal/initd/system.go` (`detectSystemCaller`,
`ResolveSystemCaller`):

| Условие | Поведение |
|---|---|
| Процесс запущен от `root` (`getuid()==0`) | `systemctl` / `rc-service` / `sv` / `service` вызываются **напрямую**, без `sudo`. |
| Процесс запущен от обычного пользователя | те же команды вызываются через `sudo -n` (non-interactive). |
| `systemd-user` (явно через `-init-system systemd-user`) | всегда `systemctl --user`, без `sudo`. |
| `-init-system none` | управление сервисом отключено (`/api/reload` и `/api/restart-self` становятся no-op/error). |

При `auto` (по умолчанию) для systemd дополнительно проверяется, что
`sudo -n systemctl is-active dnsmasq` реально работает — иначе пробуется
`systemctl --user` и только потом fallback на `sudo`.

Команды, которые панель выполняет через супервизор (для примера — systemd):

```
systemctl is-active dnsmasq
systemctl restart  dnsmasq
systemctl restart  intermasq      # только для POST /api/restart-self
```

Внимание: `dnsmasq --test` (превалидация конфига) **всегда** запускается без
`sudo` — см. раздел прав ниже.

---

## Режим 1. Запуск от `root` (рекомендуется для HomeLab)

Никакой настройки sudo не требуется. Так работает команда из быстрого старта:

```bash
sudo ./intermasq -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

В логе появится `[INIT] System: systemd (root)`.

`systemd-юнит` в этом случае тоже запускается от root — см. пример ниже.

---

## Режим 2. Запуск от выделенного пользователя

Если вы не хотите держать панель под root, создайте пользователя и дайте ему
минимум необходимых прав.

### 2.1. Файловые права

```bash
# пользователь и группа
sudo useradd -r -s /usr/sbin/nologin intermasq

# конфиги dnsmasq — читать и писать
sudo chown -R root:intermasq /etc/dnsmasq.d
sudo chmod 2775 /etc/dnsmasq.d          # setgid — новые файлы наследуют группу
sudo chmod g+rw /etc/dnsmasq.d/*.conf

# собственное состояние панели
sudo install -d -o intermasq -g intermasq -m 0750 /etc/intermasq
sudo install -d -o intermasq -g intermasq -m 0750 /etc/intermasq/history
sudo install -d -o intermasq -g intermasq -m 0770 /run/intermasq/sockets

# файл аренд — только чтение
sudo chmod g+r /var/lib/misc/dnsmasq.leases
sudo chgrp intermasq /var/lib/misc/dnsmasq.leases
```

`/proc/net/arp` доступен всем на чтение — прав не требует.

### 2.2. Passwordless sudo

Панель вызывает супервизор через `sudo -n` (без возможности ввести пароль).
Поэтому для пользователя `intermasq` нужно разрешить ровно те команды, которые
он вызывает, без пароля.

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

> Проверьте реальный путь: на Debian/Ubuntu обычно `/usr/bin/systemctl`,
> на Alpine/старых системах — `/bin/systemctl`. Панель сама резолвит бинарник
> через `$PATH` + well-known пути (`-systemctl-bin` для явного указания).
> В sudoers указывайте **абсолютный** путь, который покажет `which systemctl`.

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

После настройки в логе должно появиться `[INIT] System: systemd (via sudo)`.

### 2.3. Проверка вручную

```bash
sudo -u intermasq sudo -n systemctl is-active dnsmasq
# ожидается: active/inactive (не запрос пароля, не Permission denied)
```

---

## Явный выбор init-системы: флаг `-init-system`

При `auto` панель читает `/proc/1/comm` и проверяет наличие бинарников. Если
автоопределение не подходит — задайте явно:

```bash
-init-system systemd      # systemd (system scope), sudo если не root
-init-system systemd-user # systemctl --user (для user-session демонов)
-init-system openrc       # Alpine/Artix: rc-service
-init-system runit        # Void Linux: sv /etc/service/<name>
-init-system sysvinit     # Devuan/старые: service <name> ...
-init-system none         # отключить управление сервисом
```

Устаревший флаг `-systemd-scope` (`auto`/`system`/`user`/`none`) ещё работает и
мапится на новые значения (`system`→`systemd`, `user`→`systemd-user`), но при
использовании печатает warning.

---

## Пути по умолчанию и каталоги

| Назначение | Путь по умолчанию | Флаг |
|---|---|---|
| Конфиги dnsmasq | `/etc/dnsmasq.d` | `-conf-dir` |
| DHCP-аренды | `/var/lib/misc/dnsmasq.leases` | `-leases` |
| ARP-таблица | `/proc/net/arp` | `-arp-file` |
| База пользователей | `/etc/intermasq/users.json` | `-db` |
| Аудит-лог | `/etc/intermasq/audit.log` | `-audit-log` |
| Шаблоны хостов | `/etc/intermasq/templates.json` | `-templates` |
| История версий конфигов | `/etc/intermasq/history` | `-history-dir` |
| Плагины | `/etc/intermasq/plugins` | (в коде `internal/plugins`) |
| Сокеты плагинов | `/run/intermasq/sockets` | (в коде `internal/plugins`) |

Каталоги истории и сокетов создаются автоматически при старте.

---

## Пример systemd-юнита

### От root

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
# Привилегированный порт — через AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
```

### От пользователя `intermasq` (с sudo)

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

> **Секрет через drop-in безопаснее, чем в основном файле:**
> ```bash
> sudo systemctl edit intermasq
> # в открывшемся редакторе:
> # [Service]
> # Environment="INTERMASQ_SECRET=<openssl rand -hex 32>"
> ```
> У drop-in'а права `0600` и он не попадает в git.

Управление:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now intermasq
sudo journalctl -u intermasq -f
```

---

## Обязательная переменная `INTERMASQ_SECRET`

Без неё процесс **упадёт при старте**:

```
[FATAL] INTERMASQ_SECRET environment variable is not set.
        Generate one with:  openssl rand -hex 32
```

Секрет используется и для подписи JWT, и как значение `X-API-Key` для скриптов
и плагинов. Сгенерируйте и задайте через среду (см. юнит выше):

```bash
export INTERMASQ_SECRET="$(openssl rand -hex 32)"
```

---

## Reverse proxy (опционально)

За nginx/caddy панель работает без донастройки. Для корректного определения
реального IP в rate-limit (`/api/login`) и аудите — настрой доверенные прокси:
Gin читает `X-Forwarded-For`, и `c.RemoteIP()` используется лимитером. SSE
(`EventSource`) ходит через обычный HTTP и проксируется прозрачно.

```nginx
location / {
    proxy_pass http://127.0.0.1:8081;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    # для SSE /api/events
    proxy_buffering off;
    proxy_read_timeout 1h;
}
```
