# L5 — точные настройки каждой ВМ

Конфигурация, которую `provision.sh` приводит к этому состоянию идемпотентно на
каждом прогоне. Базовый bootstrap (ОС+сеть+SSH-ключ) делает оператор вручную
(см. ниже); всё остальное — `provision.sh`.

---

## Общее для обеих ВМ

- **Бинарник:** `/usr/local/bin/intermasq-ci` (runner scp'ит в `/tmp/intermasq-ci.upload`,
  provision стопает сервисы и `mv`).
- **Изолированный bridge `br-l5`:** `10.5.0.1/24`, без uplink, без IP-forward.
  dnsmasq и intermasq работают только в этой изолированной сети.
- **dnsmasq-конфиг `/etc/dnsmasq.d/l5.conf`** (одинаковый):
  ```
  interface=br-l5
  bind-interfaces
  dhcp-range=10.5.0.10,10.5.0.50,255.255.255.0,12h
  domain=l5.test
  local=/l5.test/
  address=/probe.l5.test/10.5.0.9
  no-resolv
  conf-dir=/etc/intermasq/conf/,*.conf
  ```
  → слушает **только** `10.5.0.1:53` + DHCP на br-l5; `no-resolv` = DNS не
  форвардится в реальную сеть. `dig @10.5.0.1 probe.l5.test → 10.5.0.9` — функциональный проб.
- **nftables** restrictive (policy drop + явные allow): lo / established /
  invalid-drop / br-l5 / **SSH:22** / icmp; sensitive-порты (`18081/18082/53/67`)
  дропаются на внешнем iface; forward policy drop.
- **Юзер `intermasq`** (system, `/sbin/nologin`) — для rootless-инстанса.
- **Sudoers `/etc/sudoers.d/intermasq`** (NOPASSWD только нужные init-команды,
  пути резолвятся из реального `$PATH`).
- **2 инстанса intermasq:**
  - `intermasq:18081` — root, `UseSudo=false`. Canonical-имя `intermasq` (т.к.
    `RestartSelf()` хардкодит его, `system.go:61`) → restart-self = self.
  - `intermasq-rootless:18082` — user `intermasq` + sudoers, `UseSudo=true`.
    Restart-self бьёт по root-sibling (PID-change доказывает выполнение sudo-ветки).
- **Секрет** `INTERMASQ_SECRET` (из Forgejo-секрета `L5_INTERMASQ_SECRET`).
- **`-ci-mode` НЕ передаётся** → дефолт `false` → `/api/restart-self` зовёт
  `RestartSelf()` (`main.go:264` `if !*CiMode`).
- **Флаги intermasq:** `-init-system=auto` (тестирует detect), `-port 18081/18082`,
  `-conf-dir /etc/intermasq/conf` (root) / `/var/lib/intermasq-rw/conf` (rootless),
  свои `-db`/`-history-dir`/`-leases`/`-arp-file /proc/net/arp`.

---

## ВМ-1: `l5-systemd` (Arch Linux, 172.20.5.18)

- **ОС:** Arch Linux, standard install (BIOS/UEFI). systemd как PID 1
  (`/proc/1/comm` = `systemd`).
- **Сеть:** `ens18` DHCP (статический или DHCP-reserve в подсети runner'а).
- **Packages (pacman):** `dnsmasq`, `bind-tools` (dig), `nftables`.
  (`openssh`, `sudo`, `bash` — из base.)
- **Bridge:** systemd-networkd — `/etc/systemd/network/20-br-l5.{netdev,network}`
  (`ConfigureWithoutCarrier=yes`, `IPForward=no`).
- **dnsmasq unit drop-in `/etc/systemd/system/dnsmasq.service.d/l5.conf`:**
  ```
  [Service]
  Type=simple            # оригинал Type=dbus без --enable-dbus вечно ждёт readiness
  ExecStart=
  ExecStart=/usr/bin/dnsmasq -k --user=dnsmasq --pid-file --conf-file=/etc/dnsmasq.d/l5.conf
  ```
- **nftables-файл:** `/etc/nftables.conf` (стандартный для systemd).
- **intermasq-юзер:** `useradd -r -s /usr/sbin/nologin intermasq`.
- **Sudoers:** `intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl is-active dnsmasq, /usr/bin/systemctl restart dnsmasq, /usr/bin/systemctl restart intermasq`.
- **intermasq unit `/etc/systemd/system/intermasq.service`** (root): `User=root`,
  `EnvironmentFile=/etc/intermasq/intermasq.env`, `StandardOutput=append:/var/log/intermasq.log`,
  `ExecStart=/usr/local/bin/intermasq-ci -port 18081 -conf-dir /etc/intermasq/conf …`.
- **intermasq unit `/etc/systemd/system/intermasq-rootless.service`**: `User=intermasq`,
  `ExecStart=… -port 18082 -conf-dir /var/lib/intermasq-rw/conf …`, `StandardOutput=append:/var/log/intermasq-rw.log`.
- **Секрет:** `/etc/intermasq/intermasq.env` → `INTERMASQ_SECRET=…` (0600 root).
- **Services enabled:** `systemd-networkd`, `dnsmasq`, `nftables`, `intermasq`, `intermasq-rootless`.
- **detect:** `systemd (root)` / `systemd (via sudo)`.

---

## ВМ-2: `l5-openrc` (Alpine Linux, 172.20.5.19)

- **ОС:** Alpine 3.24, standard `setup-alpine` (НЕ docker-образ). OpenRC как PID 1
  (`/proc/1/comm` = `init` + `/sbin/rc-service` → detect = `openrc`).
- **Сеть:** `eth0` DHCP.
- **Packages (apk):** `dnsmasq`, `bind-tools` (dig), `nftables`,
  `nftables-openrc`, `bash`, `iproute2`, `sudo`.
- **Bridge:** openrc-service `/etc/init.d/br-l5` (`ip link add … type bridge`;
  `depend: after net; before dnsmasq; before intermasq; before intermasq-rootless`).
- **dnsmasq:** грузит `/etc/dnsmasq.d/l5.conf` через активный `conf-dir=/etc/dnsmasq.d/,*.conf`
  в `/etc/dnsmasq.conf` (provision дописывает, если отсутствует).
- **nftables-файл:** `/etc/nftables.nft` (init.d default `${rules_file}` — НЕ `.conf`!).
- **intermasq-юзер:** `adduser -S -H -s /sbin/nologin -D intermasq`.
- **Sudoers:** `intermasq ALL=(root) NOPASSWD: /sbin/rc-service dnsmasq status, /sbin/rc-service dnsmasq restart, /sbin/rc-service intermasq restart`.
- **intermasq openrc `/etc/init.d/intermasq`** (root): `command_background=true`,
  `command_args="-port 18081 …"`, `output_log=/var/log/intermasq.log`.
- **intermasq openrc `/etc/init.d/intermasq-rootless`**: `command_user="intermasq:intermasq"`,
  `command_args="-port 18082 …"`, `output_log=/var/log/intermasq-rw.log`
  (этот лог `chown intermasq:intermasq` — busybox `start-stop-daemon` открывает
  его после drop-privs).
- **Секрет:** `/etc/conf.d/intermasq` И `/etc/conf.d/intermasq-rootless`
  (openrc source'ит per-svc; пишется в ОБА) → `export INTERMASQ_SECRET=…`.
- **rc-update add default:** `br-l5`, `dnsmasq`, `nftables`, `intermasq`, `intermasq-rootless`.
- **detect:** `openrc (root)` / `openrc (via sudo)`.

---

## Ручной bootstrap (оператор, один раз)

1. Поднять ВМ (Arch standard / Alpine `setup-alpine`), дать статический IP в
   подсети, видимой с fedora:44 runner'а.
2. Завести root SSH-доступ; публичный ключ runner'а → `/root/.ssh/authorized_keys`.
3. Arch: `pacman -Sy`; Alpine: `apk update`.
4. Forgejo-секреты: `L5_SSH_KEY` (приватный), `L5_SYSTEMD_HOST=root@172.20.5.18`,
   `L5_OPENRC_HOST=root@172.20.5.19`, `L5_INTERMASQ_SECRET`.

Дальше всё делает `provision.sh` при первом прогоне `run_l5_vm_tests`.
