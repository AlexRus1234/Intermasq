# L5 — как проходит тест

L5 — opt-in слой в основном CI (`build.yml`), не отдельный nightly. Запускается
**только** ручным `workflow_dispatch` с галочкой `run_l5_vm_tests`. Никакого
автозапуска по push/cron.

## Триггер

Forgejo UI → `build.yml` → `Run workflow` → отметить **`Run L5 Real VM...`**.
Остальные галочки (`run_e2e_tests`/`run_perf_tests`/…) — по желанию. Прогон
идёт под root в контейнере `fedora:44`.

## Шаг «L5 — Real VM» в build.yml

`if: success() && github.event.inputs.run_l5_vm_tests == 'true'`. Бинарник
`./intermasq-ci` уже собран в этом прогоне (тем же шагом «Build binary»,
что и для L1–L4) — **никакой зависимости от Forgejo Packages / артефактов**.

Шаг циклом по `[systemd, openrc]` (выбирает хост из секрета
`L5_SYSTEMD_HOST` / `L5_OPENRC_HOST`):

1. `dnf install openssh-clients`; кладёт приватный ключ `L5_SSH_KEY` в `~/.ssh/l5_key`.
2. Дописывает `Host l5vm-<init>` в `~/.ssh/config` (`StrictHostKeyChecking accept-new`).
3. `scp ./intermasq-ci → l5vm-<init>:/tmp/intermasq-ci.upload`
   (в `/tmp/`, а не в `/usr/local/bin/` — нельзя писать в работающий binary, ETXTBSY).
4. `scp tests/l5/{provision,vm-check}.sh → l5vm-<init>:/tmp/`.
5. `ssh … "INTERMASQ_SECRET=$L5_INTERMASQ_SECRET bash /tmp/provision.sh"` —
   идемпотентная настройка ВМ (см. `vm-setup.md`).
6. `ssh … "EXPECTED_INIT=<init> bash /tmp/vm-check.sh"` — ассерты.
7. Если RC≠0 на любой ВМ → `::error::` и шаг падает.

## provision.sh (коротко)

Авто-detect init по `/proc/1/comm`+`command -v systemctl/rc-service`. Ставит
пакеты (dnsmasq/bind-tools/nftables), юзера `intermasq`, bridge `br-l5`,
изолированный dnsmasq, nft restrictive, **2 инстанса** intermasq (root +
rootless), sudoers, секрет. Если есть `/tmp/intermasq-ci.upload` — стопает
сервисы и `mv` в `/usr/local/bin/intermasq-ci`. Чистит per-run state
(`users.json`, `conf/*`). Ждёт оба API (`:18081`, `:18082`).

Подробно: `vm-setup.md`.

## vm-check.sh — что проверяется

Для **каждого** из двух инстансов (`intermasq:18081` root, `intermasq-rootless:18082`
sudo) — 8 ассертов:

| # | Ассерт | Что реально проверяет |
|---|---|---|
| 1 | `[INIT] System:` в логе | `detectInitSystem()` отработал на живом PID 1 |
| 2 | init совпадает с `EXPECTED_INIT` | `systemd`/`openrc` детектирован верно |
| 3 | role = `root` / `via sudo` | ветка `UseSudo` выбрана верно (`os.Getuid`, `internal/initd/system.go`) |
| 4 | `/api/setup` или `/api/login` → JWT | сервер жив, auth работает |
| 5 | `GET /api/status` → `dnsmasq_active:true` | `IsActive()` реально дёрнул init-команду (`systemctl/rc-service … is-active/status`) |
| 6 | `POST /api/reload` → dnsmasq PID сменился | `Restart("dnsmasq")` реально рестартанул dnsmasq (прямая или `sudo`) |
| 7 | `dig @10.5.0.1 probe.l5.test → 10.5.0.9` | рестартнувший dnsmasq функционален |
| 8 | `POST /api/restart-self` → PID svc `intermasq` сменился | `RestartSelf()` выполнился (прямая или `sudo systemctl/rc-service restart intermasq`) |

Порядок: сначала root-инстанс (его restart-self = self), затем rootless (его
restart-self бьёт по hardcoded `intermasq` = root-sibling — доказывает
выполнение sudo-ветки). Итого **16 ассертов на ВМ**.

## Ожидаемый результат

```
=== RESULT: PASS=16 FAIL=0 SKIP=0 ===   (на каждой ВМ)
L5: both VMs PASS (root + rootless for systemd + openrc)
```

## Что НЕ проверяет L5 (намеренно)

- `runit`/`sysvinit` callers — post-v1.0 (ниша).
- UI/frontend — это L4 (Playwright); L5 тестирует backend init-plumbing.
- statement-coverage — L5 это функциональный слой, не coverage-метрика.
  Быстрые regression-guards на `system.go` (fake-бинари Coverage sweep D) остаются.

## Гоччи (важные)

- `-ci-mode` НЕ ставить — иначе `/api/restart-self` не зовёт `RestartSelf()`.
- Корневой инстанс = canonical имя `intermasq` (из-за хардкода в `internal/initd/system.go:61`).
- Alpine nft грузит `.nft`, не `.conf`; Arch — `.conf` (provision различает).
- scp бинарника только в `/tmp/` + `mv` (ETXTBSY).
