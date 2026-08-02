# L5 — Real VM (Gap 4)

Единственный функциональный слой, требующий **живых init-систем** (systemd /
openrc) как PID 1. Контейнеры не годятся: `detectInitSystem()` читает
`/proc/1/comm` (`system.go:247`), а в контейнере PID 1 — bash/entrypoint.
Fake-бинари из Coverage sweep D дали statement-процент, но не реальную
уверенность; L5 закрывает именно функциональную дыру.

**Где живёт:** opt-in галочка `run_l5_vm_tests` в основном
`.forgejo/workflows/build.yml` (рядом с `run_e2e_tests`/`run_perf_tests`).
**НЕ отдельный файл, НЕ cron, НЕ автозапуск** — только ручной `workflow_dispatch`
с поставленной галочкой. Бинарник берётся из того же прогона (`./intermasq-ci`,
уже собран) — **никакой зависимости от Forgejo Packages/артефактов**.
Подробности: `логи/l5-nightly-bootstrap.md`.

## Что проверяет `vm-check.sh` (2 инстанса на ВМ)

На каждой ВМ подняты **два** инстанса — чтобы покрыть обе ветки `SystemCaller`:
- `intermasq:18081` (`User=root`) → `UseSudo=false` (прямые `systemctl`/`rc-service`).
- `intermasq-rootless:18082` (`User=intermasq` + sudoers) → `UseSudo=true`
  (`sudo -n systemctl/rc-service …`, `system.go:40,51,61` / `:104,115,125`).

Для каждого:
1. `detectInitSystem()` → ожидаемый init + role (`root` / `via sudo`) из лога.
2. `GET /api/status` → `dnsmasq_active:true` (real `IsActive`).
3. `POST /api/reload` → **реальный** рестарт dnsmasq (смена PID, не exit-0) + `dig`-проб.
4. `POST /api/restart-self` → рестарт svc `intermasq` (смена PID).

`-ci-mode` НЕ ставить (дефолт `false`); `main.go:264` `if !*CiMode`.

> Нюанс продукта: `RestartSelf()` хардкодит имя `intermasq` (`system.go:61`).
> Поэтому root — canonical `intermasq` (restart-self = self), а rootless
> restart-self бьёт по root-sibling (PID-change доказывает выполнение sudo-ветки).

## Архитектура

- **2 persistent ВМ:** `l5-systemd` (Arch), `l5-openrc` (Alpine 3.24). runit/
  sysvinit — post-v1.0.
- **Транспорт:** SSH+SCP из fedora:44 runner'а. smoke/vm-check гоняются **внутри**
  ВМ (Model A); через границу — только SSH:22, API-трафик не покидает ВМ.
- **Rollback:** пока без snapshot — `provision.sh` idempotent и чистит per-run
  state (`users.json`, `conf/*`).

## Изоляция сети

- bridge `br-l5` (`10.5.0.0/24`, без uplink). dnsmasq слушает только на
  `10.5.0.1:53`+DHCP/br-l5 (`interface=br-l5`+`bind-interfaces`+`no-resolv`).
- **nftables** (`provision.sh` пишет restrictive ruleset: policy drop + явные
  allow lo/established/br-l5/**SSH:22**/icmp + дроп `18081/18082/53/67` на ext).
  Файл per-init: systemd → `/etc/nftables.conf`, openrc → `/etc/nftables.nft`
  (init.d default `${rules_file}`; если написать не туда — Alpine на ребуте
  вернёт дефолтный `table inet filter` без SSH:22 и доступ пропадёт).
- intermasq API на `127.0.0.1:18081/18082` (vm-check гоняется внутри ВМ).

## Сеть (важный дизайн)

`intermasq -conf-dir` указывает на **отдельный** `/etc/intermasq/conf/`, а НЕ
на `/etc/dnsmasq.d/`. Иначе smoke (который делает `rm -rf $CONF_DIR`) снёс бы
`/etc/dnsmasq.d/l5.conf` и сломал бы изоляцию dnsmasq. Цепочка:
`/etc/dnsmasq.d/l5.conf` (dnsmasq interface config, protected) → внутри него
`conf-dir=/etc/intermasq/conf/,*.conf` (host-конфиги, которыми управляет intermasq).

## Ручной bootstrap ВМ (один раз, до первого прогона)

`provision.sh` сделает почти всё, но базовый доступ ставит оператор:

1. Поднять ВМ (Arch standard install / Alpine standard `setup-alpine`).
2. `root` SSH-доступ; публичный ключ runner'а → `~/.ssh/authorized_keys`.
3. На Arch: убедиться что `pacman -Sy` работает. На Alpine: `apk update`.
4. Статичный IP в сети, видимой с fedora:44 runner'а.

Бинарник `intermasq-ci` кладётся runner'ом из того же прогона `build.yml`
(никаких отдельных публикаций). `provision.sh` дальше сам: поставит
dnsmasq/bind-tools/nftables, поднимет `br-l5`, изолированный dnsmasq,
intermasq unit/openrc (2 инстанса), секрет, старт.

## Секреты Forgejo

| Имя | Пример |
|---|---|
| `L5_SSH_KEY` | приватный ключ (ed25519), парный к `authorized_keys` на ВМ |
| `L5_SYSTEMD_HOST` | `root@172.20.5.18` |
| `L5_OPENRC_HOST` | `root@172.20.5.19` |
| `L5_INTERMASQ_SECRET` | `openssl rand -hex 32` (≥32 байта) |

(Packages/`UPLOAD_TOKEN`/`version_tag` — **не нужны**; L5 не зависит от артефактов.)

## Известные гоччи (найдены на bootstrap)

- **Arch dnsmasq `Type=dbus`**: дефолтный unit ждёт dbus readiness-сигнал. Без
  `--enable-dbus` (или с `Type=simple` в drop-in) → `activating` вечно, `systemctl
  restart` блокирует. `provision.sh` ставит drop-in с `Type=simple`.
- **Alpine `/etc/dnsmasq.conf` пустой**: без активного `conf-dir` dnsmasq биндит
  wildcard `0.0.0.0:53` → конфликт с resolver'ом / утечка в сеть. `provision.sh`
  дописывает активный `conf-dir=/etc/dnsmasq.d/,*.conf`.
- **`-ci-mode` НЕ ставить**: иначе `/api/restart-self` не зовёт `RestartSelf()`.
