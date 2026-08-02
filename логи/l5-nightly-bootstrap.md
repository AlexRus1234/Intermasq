# Этап ВМ — L5 Real VM (Gap 4) — ВЫПОЛНЕНО

**Дата:** 2026-08-02. **Статус:** ✅ реализовано и валидировано end-to-end через
реальный Forgejo runner (build.yml, галочка `run_l5_vm_tests`): обе ВМ —
**PASS=16/16** (root + rootless/sudo), **post-reboot stable** (обе ВМ пережили
рестарт — nft/services/binary корректно восстанавливаются).

## Что сделано

Единственный нулевой слой (L5) закрыт **функционально**, не цифрой: `detectInitSystem()`
и реальные init-рестарты проверены на живых systemd и openrc (PID 1), в обеих ветках
каждого caller'а (`UseSudo:false` — root, `UseSudo:true` — rootless через sudoers).
Fake-бинари Coverage sweep D давали statement-%; L5 даёт реальную уверенность для
тех же `system.go` путей.

## Решения

- **2 persistent ВМ:** `l5-systemd` (Arch, 172.20.5.18) + `l5-openrc` (Alpine 3.24.1,
  172.20.5.19). runit/sysvinit — post-v1.0.
- **Транспорт:** SSH+SCP из fedora:44 runner'а; smoke/vm-check гоняются **внутри** ВМ
  (через границу — только SSH:22, API-трафик не покидает ВМ).
- **Бинарник:** `./intermasq-ci` из того же прогона `build.yml` (scp в `/tmp/` + `mv`,
  т.к. нельзя писать в работающий binary). **Никаких Packages / артефактов.**
- **Opt-in:** галочка `run_l5_vm_tests` в `build.yml` (как `run_e2e_tests`). **НЕ
  отдельный файл, НЕ cron, НЕ автозапуск** — только ручной `workflow_dispatch`.
- **Rollback:** без snapshot — `provision.sh` idempotent, чистит per-run state.
- **Сеть:** изолированный bridge `br-l5` (`10.5.0.0/24`, без uplink); dnsmasq только
  на `10.5.0.1:53`+DHCP/br-l5 (`bind-interfaces`+`no-resolv`); intermasq API на
  `127.0.0.1:18081/18082`; **nftables** restrictive (policy drop + SSH:22/lo/br-l5).
- **2 инстанса на ВМ:** `intermasq:18081` (root) + `intermasq-rootless:18082`
  (user `intermasq` + sudoers) — покрывают обе ветки `SystemCaller`.

## Результаты валидации (через реальный runner)

| ВМ | init | root (UseSudo=false) | rootless (UseSudo=true) | итог |
|---|---|---|---|---|
| Arch 172.20.5.18 | systemd | `systemd (root)`, reload `systemctl restart dnsmasq`, restart-self | `systemd (via sudo)`, reload/restart-self через `sudo systemctl …` | **PASS=16** |
| Alpine 172.20.5.19 | openrc | `openrc (root)`, reload `rc-service dnsmasq restart`, restart-self | `openrc (via sudo)`, reload/restart-self через `sudo rc-service …` | **PASS=16** |

Каждый ассерт = смена PID (реальный рестарт, не `exit 0`) + `dig @10.5.0.1 probe.l5.test → 10.5.0.9`.

## Баги найдены и пофиксены (все в `provision.sh`, продуктовый код не тронут)

1. **dnsmasq `Type=dbus` без `--enable-dbus`** (Arch) → `activating` вечно, `systemctl
   restart` блокировал. Фикс: drop-in `Type=simple` + `--conf-file=l5.conf` явно.
2. **l5.conf не грузился** (Arch) — `/etc/dnsmasq.conf` без активного `conf-dir` →
   wildcard `0.0.0.0:53` → конфликт с `systemd-resolved`. Фикс: drop-in пинит conf-file.
3. **Alpine dnsmasq утекал в сеть** — слушал `0.0.0.0:53` (стартовал до конфига). Фикс:
   `rc-service dnsmasq restart` после корректного l5.conf.
4. **Alpine rootless падал** — openrc source'ит `/etc/conf.d/<svcname>` per-service, секрет
   был только в `intermasq`. Фикс: секрет пишется в оба conf.d.
5. **Alpine rootless лог** — busybox `start-stop-daemon` открывает `output_log` после
   drop-privs → root-owned лог → permission denied. Фикс: `chown intermasq:intermasq`.
6. **Alpine nft ломал SSH** — сервис грузит `/etc/nftables.nft` (не `.conf`); дефолтный
   `table inet filter` без SSH:22 → на ребуте доступ пропадал. Фикс: rules-файл per-init,
   restrictive ruleset с явным `tcp dport 22 accept`.
7. **scp поверх работающего binary** → ETXTBSY (`dest open: Failure`). Фикс: scp в
   `/tmp/intermasq-ci.upload` + `mv` после стопа сервисов.

## Артефакты в репо

- `.forgejo/workflows/build.yml` — input `run_l5_vm_tests` + шаг «L5 — Real VM».
- `tests/l5/provision.sh` — idempotent-настройка (br-l5, dnsmasq, nft, 2 инстанса,
  sudoers) + установка бинарника из `/tmp`.
- `tests/l5/vm-check.sh` — assert detect + реальный restart dnsmasq + RestartSelf
  (обоих инстансов).
- `tests/l5/vm-setup.md` — точные настройки каждой ВМ по пунктам.
- `tests/l5/test-flow.md` — как проходит тест L5.
- `tests/l5/README.md` — краткий индекс.

## Секреты Forgejo

`L5_SSH_KEY`, `L5_SYSTEMD_HOST` (`root@172.20.5.18`), `L5_OPENRC_HOST`
(`root@172.20.5.19`), `L5_INTERMASQ_SECRET`. Packages/`UPLOAD_TOKEN` — НЕ нужны.

## Что осталось

Soak-метрика («7 дней без красноты») переформулирована под opt-in-модель: **L5
гоняется по факту правок в init-путях** (`system.go`/`bins.go`/`main.go`), не по
календарю. Post-reboot stable подтверждено (обе ВМ пережили рестарт, PASS=16/16).
Метрика в `tests/ROADMAP.md` тикнута.
