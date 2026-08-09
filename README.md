[**English**](README.en.md) | **Русский**

<div align="center">

<h1>🛡️ Intermasq</h1>

**Веб-панель для управления dnsmasq**

Лёгкая, быстрая и самодостаточная панель для домашнего сервера и HomeLab:
один бинарник — фронтенд, бэкенд и API встроены вместе. Никаких контейнеров,
баз данных и зависимостей — только вы и ваш `dnsmasq`.

[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg?style=flat-square)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg?style=flat-square)](https://vuejs.org/)
[![Bootstrap](https://img.shields.io/badge/Bootstrap-5-7952B3.svg?style=flat-square)](https://getbootstrap.com/)
[![Platform](https://img.shields.io/badge/Linux-any-1793D1.svg?style=flat-square)](#-быстрый-старт)

</div>

---

## 📑 Содержание

- [✨ Возможности](#-возможности)
- [🚀 Быстрый старт](#-быстрый-старт)
- [⚙️ Конфигурация](#️-конфигурация)
- [🔑 Sudo и права](#-sudo-и-права)
- [🔌 API, плагины, метрики](#-api-плагины-метрики)
- [📁 Структура проекта](#-структура-проекта)
- [🛠 Стек технологий](#-стек-технологий)
- [📄 Лицензия](#-лицензия)

> 📚 **Подробная документация** (по API, sudo/init, плагинам, метрикам,
> фичам) — в каталоге [`docs/func/ru/`](docs/func/ru/README.md). Этот README —
> только выжимка и быстрый старт.

---

## ✨ Возможности

**DHCP и DNS**
- CRUD `dhcp-host=` с валидацией MAC/IP/hostname, теги `set:` и `lease-time`
- Подсказка следующего свободного IP из `dhcp-range`
- Шаблоны хостов (ip-диапазон + hostname-паттерн + target-файл)
- DNS-записи `A` / `CNAME` / `PTR` / `TXT` + CSV импорт/экспорт
- Просмотр аренд, ARP-онлайн, конвертация lease → static (массово)
- Обнаружение неизвестных ARP-устройств с **определением вендора** (OUI)

**Конфигурация dnsmasq**
- Визуальный редактор `dhcp-range`, `dhcp-option` (пресеты RFC 2132),
  `server=`, PXE/сетевая загрузка
- Raw-редактор произвольного `.conf` с проверкой `dnsmasq --test`
- Многофайловость: создание / удаление / пресеты конфигов (`basic-dhcp`,
  `forwarder`, `pxe`, `aliases`)

**Безопасность и история**
- Многоуровневая история (N версий/файл) с diff и восстановлением
- Быстрый откат по `.bak`, ZIP backup/restore с pre-flight валидацией
- Аудит-лог: кто/что/когда, с цветными метками
- Защита от path traversal: запись только внутри `-conf-dir`

**Эксплуатация и UX**
- Один бинарник (`go:embed`), мульти-init: systemd / systemd-user / OpenRC /
  runit / sysvinit — автоопределение
- SSE-пуш ARP и статуса dnsmasq в реальном времени (без опроса)
- Двойная аутентификация: JWT для браузера, `X-API-Key` для скриптов/плагинов
- **RBAC**: роли `admin` / `user`, destructive-операции только для admin
- Rate-limit на `/api/login`, отзыв JWT при logout, отзыв всех токенов при
  смене пароля / удалении пользователя
- Плагины через Unix-сокеты, `/metrics` для Prometheus, Swagger UI из коробки
- Двуязычный UI 🇷🇺/🇬🇧, тёмная/светлая тема

Подробнее — в [`docs/func/ru/features.md`](docs/func/ru/features.md).

---

## 🚀 Быстрый старт

### Требования

| Компонент | Версия | Назначение |
|---|---|---|
| **Go** | 1.25+ | Сборка бинарника |
| **Node.js** | 22+ | Сборка фронтенда |
| **dnsmasq** | любой | На целевой машине |

### Сборка

```bash
# Простой путь — пересобирает фронтенд, потом бэкенд (зеркалит порядок CI):
make build

# …или вручную:
cd frontend && npm ci && npm run build && cd ..
go build -o intermasq .
```

Production-сборка (статический линк, без symbols, с версией):

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w \
  -X intermask/internal/version.Version=1.0.0" -o intermasq .
```

> Готовых pre-built бинарников в публичном registry нет — собирайте из исходников.

### Запуск

```bash
# Обязательно: секретный ключ (без него процесс упадёт при старте)
export INTERMASQ_SECRET="$(openssl rand -hex 32)"

sudo ./intermasq \
  -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

При первом запуске откроется экран **настройки администратора**. После создания
учётной записи — полный доступ ко всем вкладкам панели.

> 💡 Для production задавайте `INTERMASQ_SECRET` через drop-in к systemd-юниту
> (права `0600`, не попадает в git): `systemctl edit intermasq`.
> Полный пример юнита и запуск от выделенного пользователя — в
> [`docs/func/ru/os-setup.md`](docs/func/ru/os-setup.md).

---

## ⚙️ Конфигурация

### Флаги командной строки

| Флаг | По умолчанию | Описание |
|---|---|---|
| `-port` | `8081` | Порт для прослушивания |
| `-conf-dir` | `/etc/dnsmasq.d` | Директория конфигов dnsmasq |
| `-leases` | `/var/lib/misc/dnsmasq.leases` | Путь к файлу аренд dnsmasq |
| `-arp-file` | `/proc/net/arp` | Путь к ARP-таблице |
| `-db` | `/etc/intermasq/users.json` | База пользователей |
| `-audit-log` | `/etc/intermasq/audit.log` | Файл аудита |
| `-templates` | `/etc/intermasq/templates.json` | Файл шаблонов хостов |
| `-history-dir` | `/etc/intermasq/history` | Директория версий конфигов |
| `-history-depth` | `10` | Сколько версий каждого файла хранить |
| `-init-system` | `auto` | `auto` / `systemd` / `systemd-user` / `openrc` / `runit` / `sysvinit` / `none` |
| `-ci-mode` | `false` | Отключает self-restart (для CI/тестов) |
| `-dnsmasq-bin`<br>`-sudo-bin`<br>`-systemctl-bin`<br>`-service-bin`<br>`-rc-service-bin`<br>`-sv-bin` | *(авто)* | Переопределение путей к системным бинарникам (`dnsmasq`, `sudo`, `systemctl`, `service`, `rc-service`, `sv`). Пусто = resolve через `$PATH` + well-known абсолютные пути (Alpine/Debian). См. `internal/bins`. |
| `-systemd-scope` | — | *(устаревший)* `auto`/`system`/`user`/`none` → мапится в `-init-system` |

> **Почему порт `8081`?** С v3.0 порт по умолчанию изменён с `8080` (часто занят
> другими сервисами, напр. Crowdsec). Для старого порта указывайте `-port 8080`.

### Переменные окружения

| Переменная | Обязательная | Описание |
|---|---|---|
| `INTERMASQ_SECRET` | ✅ **Да** | Секрет для подписи JWT и значение `X-API-Key`. Сгенерируйте: `openssl rand -hex 32` |

---

## 🔑 Sudo и права

Панель **сама решает**, нужен ли `sudo`, по `getuid()`:

- **Запуск от `root`** → `systemctl` / `dnsmasq --test` вызываются напрямую, sudo **не нужен**. Так работает `sudo ./intermasq` из быстрого старта.
- **Запуск от обычного пользователя** → управление сервисом идёт через `sudo -n` (non-interactive). Нужно настроить passwordless-sudo на конкретные команды `systemctl` / `rc-service` / `sv` / `service` **и** дать права на чтение/запись `conf-dir` и файла аренд.

Пример `/etc/sudoers.d/intermasq` (systemd, пользователь `intermasq`):

```sudoers
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl is-active dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl restart dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl restart intermasq
```

В логе стартовая строка подскажет выбранный режим: `[INIT] System: systemd (root)`
или `[INIT] System: systemd (via sudo)`.

**Полное руководство** (sudo для всех init-систем, файловые права, пример
systemd-юнита, выделенный пользователь) — в
[`docs/func/ru/os-setup.md`](docs/func/ru/os-setup.md).

---

## 🔌 API, плагины, метрики

После запуска доступна интерактивная документация:

```
http://<host>:<port>/swagger/index.html
```

| Что | Кратко | Подробности |
|---|---|---|
| **Аутентификация** | `Authorization: Bearer <JWT>` (браузер) или `X-API-Key: <INTERMASQ_SECRET>` (скрипты) | [`docs/func/ru/api.md`](docs/func/ru/api.md) |
| **Эндпоинты** | `/api/hosts`, `/api/aliases`, `/api/config`, `/api/files/:name`, `/api/history`, `/api/backup`, `/api/reload`, `/api/events`, … | полный список + RBAC в [`api.md`](docs/func/ru/api.md) |
| **RBAC** | роль `admin` (reload/rollback/raw-запись/users/restart) vs `user` (чтение + добавление) | [`api.md`](docs/func/ru/api.md) |
| **Плагины** | sidecar-процессы через Unix-сокеты, manifest в `/etc/intermasq/plugins/`, проксируются в iframe | [`docs/func/ru/plugins.md`](docs/func/ru/plugins.md) |
| **Метрики** | `/metrics` для Prometheus: хосты/аренды/ARP/статус dnsmasq/health-check доменов | [`docs/func/ru/metrics.md`](docs/func/ru/metrics.md) |

---

## 📁 Структура проекта

```
.
├── main.go                 # Точка входа: флаги, bootstrap, gin engine, статика/swagger (тонкий)
├── internal/
│   ├── models/             # Типы данных (HostEntry, DnsAliasEntry, …)
│   ├── validate/           # Валидаторы MAC/IP/hostname/tag + нормализаторы
│   ├── oui/                # Таблица OUI (определение вендора по MAC)
│   ├── stats/              # Счётчики для /metrics
│   ├── bins/               # Авто-resolve путей к системным бинарникам
│   ├── initd/              # SystemCaller — детект и управление init-системами
│   ├── dnsmasq/            # Ядро dhcp-host: парсинг/запись, алиасы, конфиг, история, backup
│   ├── netstate/           # ARP, leases, обнаружение устройств
│   ├── templates/          # Шаблоны хостов (создание/применение)
│   ├── auth/               # Пользователи, JWT, rate-limit, RBAC middleware (bcrypt)
│   ├── audit/              # Журнал аудита
│   ├── control/            # SSE broadcaster, статус/reload dnsmasq
│   ├── metrics/            # /metrics для Prometheus + DNS health-check
│   ├── plugins/            # Загрузка/проксирование плагинов (Unix-сокеты)
│   ├── version/            # Версия сборки (ldflags)
│   └── webapi/             # HTTP-обработчики + регистрация роутов (/api/*)
├── docs/                   # OpenAPI + docs/func/ru/ (пользовательская документация)
├── frontend/               # Vue 3 SPA (Vite, Bootstrap 5, vue-i18n)
├── .forgejo/workflows/     # CI: сборка, тесты, smoke, опц. fuzz/e2e/L5-ВМ
├── tests/                  # Smoke-сьюты, Playwright E2E, perf, L5 (живые ВМ)
├── LICENSE                 # GNU AGPL v3
└── README.md               # Этот файл
```

> 💡 **Почему `main.go` в корне, а не в `cmd/intermasq/`?** Директива
> `//go:embed frontend/dist/*` не умеет подниматься по дереву (`../`), поэтому
> точка входа обязана жить рядом с `frontend/`. Сборка остаётся прежней:
> `go build -o intermasq .`

---

## 🛠 Стек технологий

**Бэкенд:** Go 1.25 · Gin · golang-jwt/v5 · golang.org/x/crypto (bcrypt) ·
gin-swagger · `go:embed`.

**Фронтенд:** Vue 3 (Composition API) · Vite 7 · Bootstrap 5 (dark/light) ·
vue-i18n 9 (RU/EN) · Axios · event-source-polyfill (SSE).

**Инфраструктура и качество:** Forgejo Actions (CI) · `go vet` / `gofmt` ·
`go test` (включая `-race`) · fuzz-таргеты · Playwright E2E · smoke-сьюты ·
L5-тесты на живых ВМ (systemd + OpenRC).

---

## 📄 Лицензия

Проект распространяется под лицензией **[GNU Affero General Public License v3.0](LICENSE)**.

```
Intermasq - Web panel for dnsmasq
Copyright (C) 2026  AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
```

<div align="center">

<sub>Сделано для HomeLab. Если проект оказался полезным — ⭐ звезда репозиторию приветствуется.</sub>

</div>
