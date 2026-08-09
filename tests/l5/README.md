<!--
Intermasq - Web panel for dnsmasq
Copyright (C) 2026 AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
-->

# L5 — Real VM (Gap 4)

Единственный функциональный слой, требующий **живых init-систем** (systemd /
openrc) как PID 1. Контейнеры не годятся: `detectInitSystem()` читает
`/proc/1/comm` (`internal/initd/system.go`), а в контейнере PID 1 — bash/entrypoint.
Fake-бинари из Coverage sweep D дали statement-процент, но не реальную
уверенность; L5 закрывает именно функциональную дыру.

## Документация

- **`vm-setup.md`** — точные настройки каждой ВМ по пунктам (Arch/systemd, Alpine/openrc).
- **`test-flow.md`** — как проходит тест (триггер, шаг в build.yml, provision, vm-check, ассерты).
- **`provision.sh`** — идемпотентная настройка ВМ (выполняется на ВМ по SSH).
- **`vm-check.sh`** — ассерты L5 (выполняется на ВМ после provision).
- `логи/l5-nightly-bootstrap.md` — сводка этапа, баг-таблица, результаты.

## Кратко

- **Триггер:** галочка `run_l5_vm_tests` в `.forgejo/workflows/build.yml`
  (как `run_e2e_tests`). Только ручной `workflow_dispatch` — без cron/автозапуска.
- **Транспорт:** SSH+SCP из fedora:44 runner'а; smoke/vm-check внутри ВМ.
- **Бинарник:** `./intermasq-ci` из того же прогона (без Packages/артефактов).
- **2 ВМ:** Arch/systemd (172.20.5.18) + Alpine/openrc (172.20.5.19). runit/sysvinit — post-v1.0.
- **2 инстанса на ВМ:** root (`UseSudo=false`) + rootless `intermasq`-user + sudoers
  (`UseSudo=true`) — покрывают обе ветки `SystemCaller`.
- **Результат:** PASS=16/16 на каждой ВМ ( validated end-to-end через реальный runner).

## Секреты Forgejo

`L5_SSH_KEY`, `L5_SYSTEMD_HOST` (`root@172.20.5.18`), `L5_OPENRC_HOST`
(`root@172.20.5.19`), `L5_INTERMASQ_SECRET`. Packages/`UPLOAD_TOKEN` — НЕ нужны.
