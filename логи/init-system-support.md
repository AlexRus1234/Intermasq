# Сессия: поддержка нескольких init-систем (systemd / OpenRC / runit / sysvinit)

## Контекст

Проект был жёстко привязан к systemd: `system.go` содержал только `SystemdSystemCaller` и `SystemdUserCaller`, автоопределение искало только systemd, а в `main.go:163` был хардкод `systemctl restart intermasq`. На системах без systemd (Alpine/OpenRC, Void/runit, Devuan/sysvinit) перезапуск dnsmasq и самоперезапуск intermasq не работали.

## Изменения

### `system.go` — полная переработка

- **Интерфейс `SystemCaller`** — добавлен метод `RestartSelf() error` для самоперезапуска intermasq через ту же init-систему
- **`SystemdSystemCaller`** — добавлен `RestartSelf()` (systemctl restart intermasq, с/без sudo)
- **`SystemdUserCaller`** — добавлен `RestartSelf()` (systemctl --user restart intermasq)
- **`OpenRCCaller`** — новый caller для Alpine/Gentoo/Artix:
  - `IsActive`: `rc-service <svc> status` → парсит "started"
  - `Restart`: `rc-service <svc> restart`
  - `RestartSelf`: `rc-service intermasq restart`
  - Поддержка `UseSudo`
- **`RunitCaller`** — новый caller для Void/Artix:
  - `IsActive`: `sv status <svc_path>` → парсит "run"
  - `Restart`: `sv restart <svc_path>`
  - `RestartSelf`: `sv restart /etc/service/intermasq`
  - Поддержка `UseSudo` и `ServiceDir` (по умолчанию `/etc/service`)
- **`SysVinitCaller`** — новый caller для Devuan:
  - `IsActive`: `service <svc> status` → exit code 0
  - `Restart`: `service <svc> restart`
  - `RestartSelf`: `service intermasq restart`
  - Поддержка `UseSudo`
- **`NoneCaller.RestartSelf()`** — возвращает ошибку "self-restart not supported without init system"
- **`detectInitSystem()`** — новая функция автоопределения:
  1. Читает `/proc/1/comm` → systemd / runit / init (+ openrc по rc-service / sysvinit)
  2. Проверяет наличие утилит: systemctl, rc-service, sv, service
  3. Fallback → "none"
- **`detectSystemCaller()`** — переписана: вызывает `detectInitSystem()` и возвращает соответствующий caller
- **`resolveSystemCaller()`** — понимает новые значения: `systemd`, `systemd-user`, `openrc`, `runit`, `sysvinit`, `none` + маппит легаси через `mapLegacyScope()`
- **`mapLegacyScope()`** — маппинг старых значений: `system`→`systemd`, `user`→`systemd-user`, `none`→`none`
- Лог-префикс `[SYSTEMD]` → `[INIT]`

### `main.go` — флаги и самоперезапуск

- Новый флаг `-init-system` (auto/systemd/systemd-user/openrc/runit/sysvinit/none)
- Старый флаг `-systemd-scope` сохранён для обратной совместимости; если задан (не пустой), маппится через `mapLegacyScope()` с warning'ом в лог
- Если переданы оба флага — `-systemd-scope` при не-auto значении переопределяет `-init-system`
- Хардкод `exec.Command("/usr/bin/systemctl", "restart", "intermasq")` заменён на `sysCaller.RestartSelf()` с выводом ошибки при неудаче

### `dnsmasq_test.go` — обновлённые и новые тесты

- `TestResolveSystemCaller` — проверяет все новые значения флага
- `TestResolveSystemCallerLegacy` — проверяет что старые `system`/`user` маппятся корректно
- `TestMapLegacyScope` — тест маппинга легаси-значений
- `TestNoneCaller` — добавлена проверка `RestartSelf()` → error
- `TestOpenRCCaller` — String() с/без sudo
- `TestRunitCaller` — String() содержит "runit" и ServiceDir
- `TestSysVinitCaller` — String() с/без sudo
