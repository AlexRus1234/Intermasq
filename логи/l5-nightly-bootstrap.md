# Этап ВМ — L5 Real VM nightly (Gap 4) — bootstrap

**Дата:** 2026-08-01. **Статус:** инфраструктура поднята, Arch/systemd доказан
вживую (PASS=2/2), Alpine/OpenRC — конфиг частично готов, правки ушли в
`tests/l5/provision.sh` (см. ниже). Nightly workflow + vm-check написаны.

## Решения (из плана, подтверждены оператором)

- **2 ВМ:** `l5-systemd` (Arch, 172.20.5.18) + `l5-openrc` (Alpine 3.24.1,
  172.20.5.19). runit/sysvinit — post-v1.0.
- **Транспорт:** SSH + SCP из текущего fedora:44 Forgejo runner'а (Model A:
  smoke/vm-check гоняются **внутри** ВМ, через границу идёт только SSH:22;
  API-трафик не покидает ВМ).
- **Rollback:** пока без snapshot — `provision.sh` idempotent и чистит per-run
  state (`/etc/intermasq/conf/*`, `/etc/intermasq/users.json`, history).
- **Сеть:** изолированный bridge `br-l5` (`10.5.0.0/24`, без uplink, без
  IP-forward). dnsmasq слушает ТОЛЬКО на `10.5.0.1:53` + DHCP на `br-l5`
  (`interface=br-l5`+`bind-interfaces`+`no-resolv`+`local=/l5.test/` → DNS не
  форвардится наружу). intermasq API на `127.0.0.1:18081`.
- **Opt-in:** отдельный `.forgejo/workflows/l5-nightly.yml` (`schedule.cron` +
  `workflow_dispatch`), НЕ в `build.yml` (требование задания).

## Что проверено на живых ВМ

### `l5-systemd` (Arch, 172.20.5.18) — ДОКАЗАНО

`detectInitSystem()` → `"systemd"` (`/proc/1/comm`=`systemd`,
`system.go:258`). intermasq под root → `SystemdSystemCaller{UseSudo:false}`.
Ручной прогон `tests/l5/_scratch/l5test.sh`:

| Ассерт | Результат |
|---|---|
| `[INIT] System: systemd (root)` в логе | ✓ |
| `GET /api/status` → `dnsmasq_active:true` (real `systemctl is-active`) | ✓ |
| `POST /api/reload` → dnsmasq `ActiveEnterTimestamp` `16:31:53→16:36:02` | ✓ реально рестартовал |
| `POST /api/restart-self` → intermasq `MainPID` `1393→1467`, back in 1s | ✓ реально self-restart |

`-ci-mode` НЕ передан → дефолт `false` → `/api/restart-self` зовёт
`sysCaller.RestartSelf()` (`main.go:264` `if !*CiMode`).

### `l5-openrc` (Alpine, 172.20.5.19) — конфиг частично готов

`detectInitSystem()` → `"openrc"` (`/proc/1/comm`=`init` + `/sbin/rc-service`
есть, `system.go:258-262`). `br-l5` поднят (openrc-сервис оператором). Найдены
баги сетапа (правит `provision.sh`):
- **dnsmasq утекал в глобальную сеть:** слушал `0.0.0.0:53` (процесс стартовал
  до дозревания конфига) → `rc-service dnsmasq restart` + корректный l5.conf.
- **`/etc/init.d/intermasq` недописан:** нет `INTERMASQ_SECRET`, нет
  `command_args`, stdout не залогирован (нельзя assert `[INIT] System:`), имя
  бинарника `intermasq` vs `intermasq-ci`.

## Баги найдены и пофиксены (Arch, во время bootstrap)

1. **dnsmasq unit `Type=dbus` + мой drop-in убрал `--enable-dbus`** → systemd
   ждал readiness-сигнал вечно (`activating`, `systemctl restart` блокировал).
   Фикс: drop-in с `Type=simple` (`/etc/systemd/system/dnsmasq.service.d/l5.conf`).
2. **l5.conf не грузился:** `/etc/dnsmasq.conf` без активного `conf-dir`, а мой
   `grep -q conf-dir` матчил закомментированные строки → `|| echo` не сработал.
   dnsmasq биндил wildcard `0.0.0.0:53` → конфликт с `systemd-resolved`
   (`127.0.0.53:53`, "Address already in use"). Фикс: drop-in пинит
   `--conf-file=/etc/dnsmasq.d/l5.conf` явно.
3. (PS-quicks, не баг проекта) PowerShell 5.1 сгрызает `\n` и double-quotes в
   args native-команд → писал файлы через scp/printf-on-remote, не инлайн.

## Артефакты в репо (этот этап)

- `.forgejo/workflows/l5-nightly.yml` — nightly cron + workflow_dispatch,
  matrix [systemd, openrc], SSH/SCP, вызов provision.sh + vm-check.sh.
- `tests/l5/provision.sh` — idempotent-настройка обеих ВМ (br-l5, dnsmasq c
  изоляцией, intermasq unit/openrc, INTERMASQ_SECRET, binary install).
- `tests/l5/vm-check.sh` — assert detect (`[INIT] System:`) + реальный restart
  dnsmasq (`ActiveEnterTimestamp`) + RestartSelf (`MainPID`).
- `tests/l5/intermasq.service` (systemd) / `tests/l5/intermasq.openrc` (Alpine).
- `tests/l5/dnsmasq-l5.conf` — шаблон изолированного dnsmasq-конфига.
- `tests/l5/README.md` — как поднять ВМ руками + соответствие секретов.

## Секреты Forgejo (для оператора)

`L5_SSH_KEY`, `L5_SYSTEMD_HOST` (`root@172.20.5.18`),
`L5_OPENRC_HOST` (`root@172.20.5.19`), `L5_INTERMASQ_SECRET`, и опубликованный
`intermasq-ci` в Forgejo Packages (`L5_BINARY_VERSION`).

## Держит nightly

Оператор (ВМ — persistent на гипервизоре пользователя; nightly гоняет
fedora:44 runner по SSH). 7 дней green → тикнуть `tests/ROADMAP.md` метрику
«L5 nightly: 7 дней без красноты».

---

## Update (2026-08-01): 2 инстанса (root + rootless/sudo) + nftables

По просьбе оператора добавлены:
- **nftables** на обе ВМ (`/etc/nftables.conf`): на внешнем iface гасятся
  intermasq API `18081/18082` и dnsmasq `:53/:67`; lo/br-l5/SSH свободны,
  forward policy drop. Defense-in-depth к `bind-interfaces`.
- **rootless-инстанс** (`intermasq-rootless:18082`, user `intermasq` + sudoers
  `/etc/sudoers.d/intermasq` с резолв. путями) → покрывает `UseSudo:true`
  ветки: `system.go:40` (is-active), `:51` (restart dnsmasq), `:61` (restart-self).
- Корневой инстанс оставлен canonical именем `intermasq` (т.к. `RestartSelf`
  хардкодит это имя, `system.go:61`) → root restart-self=self; rootless
  restart-self бьёт по root-sibling (PID-change доказывает выполнение sudo-ветки).

**Архив валидации:**

| ВМ | init | root (UseSudo=false) | rootless (UseSudo=true) | итог |
|---|---|---|---|---|
| Arch 172.20.5.18 | systemd | detect `systemd (root)`, reload/restart-self PID-change | detect `systemd (via sudo)`, reload PID-change (sudo `systemctl restart dnsmasq`), restart-self PID-change (sudo `systemctl restart intermasq`) | **PASS=16 FAIL=0** |
| Alpine 172.20.5.19 | openrc | detect `openrc (root)`, reload/restart-self PID-change | detect `openrc (via sudo)`, reload PID-change (sudo `rc-service dnsmasq restart`), restart-self PID-change (sudo `rc-service intermasq restart`) | **PASS=16 FAIL=0** |

**Баг найден при rootless на Alpine:** openrc source'ит `/etc/conf.d/<svcname>`
per-service, а секрет писался только в `/etc/conf.d/intermasq` →
`intermasq-rootless` стартовал без `INTERMASQ_SECRET` → падал. Фикс в
`provision.sh`: секрет пишется и в `/etc/conf.d/intermasq-rootless`. Alpine ВМ
ушла в даун (network/power) до re-run'а — вернётся, прогон добьёт rootless.

**Баг rootless-log на Alpine:** busybox `start-stop-daemon` открывает
`output_log` **после** `command_user` (drop privs), а `/var/log/intermasq-rw.log`
был root-owned → permission denied → сервис крутился на буте. Фикс в
`provision.sh`: `chown intermasq:intermasq /var/log/intermasq-rw.log`.

**Баг nft на Alpine (стоил SSH-доступа):** Alpine nftables-сервис грузит
**`/etc/nftables.nft`** (init.d default `${rules_file}`), а НЕ `.conf`. provision.sh
писал `.conf` → сервис не видел его и грузил дефолтный `table inet filter`
(policy drop, **без правила SSH:22**) → на `rc-service nftables restart`/ребуте
SSH дропался. Фикс: provision.sh определяет rules-файл per-init (systemd
`/etc/nftables.conf`, openrc `/etc/nftables.nft`) и пишет туда **restrictive**
ruleset (policy drop + явные allow: lo/established/br-l5/**SSH:22**/icmp +
дроп sensitive-портов на ext iface). Теперь restrictive и reboot-safe на обеих ВМ.

**PS-напоминание:** PowerShell 5.1 сгрызает `\n` и double-quotes в args native
(ssh) — файлы на ВМ пишутся через `scp`/heredoc, не инлайн-`printf`. nft-конфиг
пишется из heredoc в provision.sh (на ВМ, не через PS).
