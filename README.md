**Русский** | [English](README.en.md) |

<div align="center">

<h1>Intermasq</h1>

**Веб-панель управления dnsmasq**

Intermasq представляет собой автономное веб-приложение для администрирования
`dnsmasq`. Фронтенд, серверная логика и API объединены в одном исполняемом
файле. Для хранения данных используется файловая система; внешняя СУБД и
контейнерная инфраструктура не требуются.

[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg?style=flat-square)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg?style=flat-square)](https://vuejs.org/)
[![Bootstrap](https://img.shields.io/badge/Bootstrap-5-7952B3.svg?style=flat-square)](https://getbootstrap.com/)
[![Platform](https://img.shields.io/badge/Linux-any-1793D1.svg?style=flat-square)](#быстрый-старт)

</div>

---

## Содержание

- [Демонстрация](#демонстрация)
- [Возможности](#возможности)
- [Быстрый старт](#быстрый-старт)
- [Конфигурация](#конфигурация)
- [Права доступа](#права-доступа)
- [API, плагины и метрики](#api-плагины-и-метрики)
- [Структура проекта](#структура-проекта)
- [Технологический стек](#технологический-стек)
- [Лицензия](#лицензия)

> Расширенная документация по API, правам доступа, системным службам, плагинам
> и метрикам приведена в каталоге [`docs/func/ru/`](docs/func/ru/README.md).
> Настоящий файл содержит обзор системы и инструкцию по первоначальному запуску.

Проект разработан в соответствии с заранее определённой архитектурой; при
подготовке исходного кода использовался ИИ-ассистент.[^1]

---

## Демонстрация

Несколько экранов веб-панели в русской локализации:

<p>
  <img src="скрин/ru/Снимок%20экрана%202026-08-09%20151907.png" alt="Панель Intermasq" width="49%">
  <img src="скрин/ru/Снимок%20экрана%202026-08-09%20152016.png" alt="Настройки Intermasq" width="49%">
</p>
<p>
  <img src="скрин/ru/Снимок%20экрана%202026-08-09%20152034.png" alt="Конфигурация dnsmasq" width="49%">
  <img src="скрин/ru/Снимок%20экрана%202026-08-09%20152123.png" alt="Управление файлами" width="49%">
</p>
<p>
  <img src="скрин/ru/Снимок%20экрана%202026-08-09%20152257.png" alt="Список устройств" width="49%">
</p>

---

## Возможности

### DHCP и DNS
- Операции с `dhcp-host=` и валидация MAC/IP/hostname, тегов `set:` и `lease-time`
- Подсказка следующего свободного IP из `dhcp-range`
- Шаблоны хостов (ip-диапазон + hostname-паттерн + target-файл)
- DNS-записи `A` / `CNAME` / `PTR` / `TXT` + CSV импорт/экспорт
- Просмотр аренд, ARP-онлайн, конвертация lease → static (массово)
- Обнаружение неизвестных ARP-устройств с **определением вендора** (OUI)

### Конфигурация dnsmasq
- Визуальный редактор `dhcp-range`, `dhcp-option` (пресеты RFC 2132),
  `server=`, PXE/сетевая загрузка
- Raw-редактор произвольного `.conf` с проверкой `dnsmasq --test`
- Многофайловость: создание / удаление / пресеты конфигов (`basic-dhcp`,
  `forwarder`, `pxe`, `aliases`)

### Безопасность и история
- Многоуровневая история (N версий/файл) с diff и восстановлением
- Откат по `.bak`, резервное копирование и восстановление ZIP с предварительной валидацией
- Аудит-лог: кто/что/когда, с цветными метками
- Защита от path traversal: запись только внутри `-conf-dir`

### Эксплуатация и пользовательский интерфейс
- Один бинарник (`go:embed`), мульти-init: systemd / systemd-user / OpenRC /
  runit / sysvinit — автоопределение
- SSE-пуш ARP и статуса dnsmasq в реальном времени (без опроса)
- Двойная аутентификация: JWT для браузера, `X-API-Key` для скриптов/плагинов
- **RBAC**: роли `admin` / `user`, destructive-операции только для admin
- Rate-limit на `/api/login`, отзыв JWT при logout, отзыв всех токенов при
  смене пароля / удалении пользователя
- Плагины через Unix-сокеты, `/metrics` для Prometheus, документация Swagger
- Русский и английский языки интерфейса, тёмная и светлая темы

Подробное описание приведено в [`docs/func/ru/features.md`](docs/func/ru/features.md).

---

## Быстрый старт

### Требования

| Компонент | Версия | Назначение |
|---|---|---|
| **Go** | 1.25+ | Сборка бинарника |
| **Node.js** | 22+ | Сборка фронтенда |
| **dnsmasq** | любой | На целевой машине |

### Сборка

```bash
# Сборка фронтенда и серверной части в порядке, используемом CI:
make build

# Альтернативная ручная сборка:
cd frontend && npm ci && npm run build && cd ..
go build -o intermasq .
```

Сборка для рабочей среды (статическая компоновка, без таблиц символов, с версией):

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w \
  -X intermask/internal/version.Version=1.0.0" -o intermasq .
```

> Предварительно собранные бинарные файлы в публичном registry не публикуются.
> Сборка выполняется из исходного кода.

### Запуск

```bash
# Обязательно: секретный ключ (без него процесс упадёт при старте)
export INTERMASQ_SECRET="$(openssl rand -hex 32)"

sudo ./intermasq \
  -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

При первом запуске отображается форма создания учётной записи администратора.
После её заполнения становятся доступны функции панели.

> В рабочей среде рекомендуется задавать `INTERMASQ_SECRET` через drop-in
> systemd-юнита с правами `0600`. Полный пример юнита и порядок запуска от
> выделенного пользователя приведены в
> [`docs/func/ru/os-setup.md`](docs/func/ru/os-setup.md).

---

## Конфигурация

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

### Переменные окружения

| Переменная | Обязательная | Описание |
|---|---|---|
| `INTERMASQ_SECRET` | **Да** | Секрет для подписи JWT и значение `X-API-Key`. Сгенерируйте: `openssl rand -hex 32` |

---

## Права доступа

Способ выполнения системных команд определяется значением `getuid()`:

- **Запуск от `root`**: `systemctl` и `dnsmasq --test` вызываются напрямую.
- **Запуск от обычного пользователя**: управление сервисом выполняется через
  `sudo -n`. Требуется разрешить конкретные команды и предоставить права на
  чтение и запись `conf-dir`, а также на чтение файла аренд.

Пример `/etc/sudoers.d/intermasq` (systemd, пользователь `intermasq`):

```sudoers
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl is-active dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl restart dnsmasq
intermasq ALL=(root) NOPASSWD: /usr/bin/systemctl restart intermasq
```

В логе стартовая строка подскажет выбранный режим: `[INIT] System: systemd (root)`
или `[INIT] System: systemd (via sudo)`.

Полное руководство по sudo для всех поддерживаемых init-систем, файловым правам,
примеру systemd-юнита и запуску от выделенного пользователя приведено в
[`docs/func/ru/os-setup.md`](docs/func/ru/os-setup.md).

---

## API, плагины и метрики

После запуска доступна интерактивная документация:

```
http://<host>:<port>/swagger/index.html
```

| Что | Кратко | Подробности |
|---|---|---|
| **Аутентификация** | `Authorization: Bearer <JWT>` (браузер) или `X-API-Key: <INTERMASQ_SECRET>` (скрипты) | [`docs/func/ru/api.md`](docs/func/ru/api.md) |
| **Эндпоинты** | `/api/hosts`, `/api/aliases`, `/api/config`, `/api/files/:name`, `/api/history`, `/api/backup`, `/api/reload`, `/api/events`, … | полный список + RBAC в [`api.md`](docs/func/ru/api.md) |
| **RBAC** | роль `admin` (reload/rollback/raw-запись/users/restart) и роль `user` (чтение и добавление) | [`api.md`](docs/func/ru/api.md) |
| **Плагины** | sidecar-процессы через Unix-сокеты, манифест в `/etc/intermasq/plugins/`, проксирование в iframe | [`docs/func/ru/plugins.md`](docs/func/ru/plugins.md) |
| **Метрики** | `/metrics` для Prometheus: хосты/аренды/ARP/статус dnsmasq/health-check доменов | [`docs/func/ru/metrics.md`](docs/func/ru/metrics.md) |

---

## Структура проекта

```
.
├── main.go                 # Точка входа: флаги, инициализация, Gin, статика и Swagger
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
├── docs/                   # OpenAPI и пользовательская документация
├── frontend/               # Vue 3 SPA (Vite, Bootstrap 5, vue-i18n)
├── .forgejo/workflows/     # CI: сборка, тесты, smoke, опц. fuzz/e2e/L5-ВМ
├── tests/                  # Smoke-сьюты, Playwright E2E, perf, L5 (живые ВМ)
├── LICENSE                 # GNU AGPL v3
└── README.md               # Основная документация
```

Файл `main.go` расположен в корне, поскольку директива
`//go:embed frontend/dist/*` не поддерживает обращение к родительским каталогам.
Это позволяет сохранять сборку командой `go build -o intermasq .`.

---

## Технологический стек

**Бэкенд:** Go 1.25 · Gin · golang-jwt/v5 · golang.org/x/crypto (bcrypt) ·
gin-swagger · `go:embed`.

**Фронтенд:** Vue 3 (Composition API) · Vite 7 · Bootstrap 5 (dark/light) ·
vue-i18n 9 (RU/EN) · Axios · event-source-polyfill (SSE).

**Инфраструктура и качество:** Forgejo Actions (CI) · `go vet` / `gofmt` ·
`go test` (включая `-race`) · fuzz-таргеты · Playwright E2E · smoke-сьюты ·
L5-тесты на живых ВМ (systemd + OpenRC).

---

## Лицензия

Проект распространяется под лицензией **[GNU Affero General Public License v3.0](LICENSE)**.

```
Intermasq - Web panel for dnsmasq
Copyright (C) 2026  AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
```

[^1]: Разработка исходного кода выполнялась с использованием ИИ-ассистента в
соответствии с заранее определённой архитектурой проекта; архитектурные решения,
проверка результатов и итоговая интеграция осуществлялись автором.
