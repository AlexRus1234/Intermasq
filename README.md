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
- [📸 Скриншоты](#-скриншоты)
- [🚀 Быстрый старт](#-быстрый-старт)
- [⚙️ Конфигурация](#️-конфигурация)
- [🔌 API и Swagger](#-api-и-swagger)
- [🧩 Плагины](#-плагины)
- [📊 Метрики для Prometheus](#-метрики-для-prometheus)
- [📁 Структура проекта](#-структура-проекта)
- [🛠 Стек технологий](#-стек-технологий)
- [📄 Лицензия](#-лицензия)

---

## ✨ Возможности

<table>
<tr>
<td width="50%" valign="top">

### 🖥 DHCP и DNS
- **Статические хосты** — CRUD `dhcp-host=` с валидацией MAC/IP/hostname
- **DHCP-теги** — `set:` на хостах для таргетинга `dhcp-option`
- **DNS-записи** — `A` / `CNAME` / `PTR` / `TXT` в отдельной вкладке
- **Аренды** — просмотр DHCP-leases, ARP-онлайн, конвертация в статику
- **Обнаружение** — неизвестные ARP-устройства с **определением вендора** (OUI)

</td>
<td width="50%" valign="top">

### ⚙️ Конфигурация
- **Визуальный редактор** — `dhcp-range`, `dhcp-option` (с пресетами RFC 2132),
  `server=` форвардинг, PXE/сетевая загрузка
- **Raw-редактор** — прямое редактирование любого `.conf` с проверкой `dnsmasq --test`
- **Многофайловость** — работа с несколькими `.conf` одновременно
- **Шаблоны** — пресеты hostname/IP-диапазона для быстрого добавления

</td>
</tr>
<tr>
<td width="50%" valign="top">

### 🛡 Безопасность и надёжность
- **Многоуровневая история** — N версий каждого файла с **diff** и восстановлением
- **Быстрый откат** — `.bak` на одно действие назад
- **ZIP backup/restore** — архив всех `.conf` с pre-flight валидацией
- **Аудит-лог** — все действия с цветными метками (кто, что, когда)
- **Защита путей** — path traversal невозможен, запись только внутри `conf-dir`

</td>
<td width="50%" valign="top">

### 🚀 Массовые операции
- **Bulk-импорт** — вставка списка устройств текстом или CSV
- **Bulk-редактирование** — массовое изменение и перемещение хостов
- **CSV-экспорт** — выгрузка хостов и DNS-записей
- **Lease → static** — массовый перенос аренд в статику одним кликом
- **Массовое удаление** — выбор чекбоксами

</td>
</tr>
<tr>
<td colspan="2" valign="top">

### 🔧 Эксплуатация и UX
- **Один бинарник** — фронтенд встроен через `go:embed`, ничего ставить не нужно
- **Мульти-init** — автоопределение `systemd` / `systemd-user` / `OpenRC` / `runit` / `sysvinit`
- **Мониторинг в реальном времени** — SSE-пуш ARP и статуса dnsmasq (без опроса)
- **Метрики Prometheus** — `/metrics` с health-check'ом DNS-доменов
- **Двойная аутентификация** — JWT для браузера, `X-API-Key` для скриптов и плагинов
- **Rate-limit** на вход + сброс счётчика при успехе, **отзыв JWT** при logout
- **Обязательный `INTERMASQ_SECRET`** — процесс не стартует с дефолтным ключом
- **Плагины** — расширения через Unix-сокеты, проксируются в iframe
- **Swagger UI** — интерактивная API-документация из коробки
- **Двуязычный UI** — 🇷🇺 Русский / 🇬🇧 English в один клик
- **🌙 Тёмная / ☀️ светлая тема** с сохранением выбора

</td>
</tr>
</table>

---

## 📸 Скриншоты

> *Скриншоты интерфейса будут добавлены позже.*

---

## 🚀 Быстрый старт

### Требования

| Компонент | Версия | Назначение |
|---|---|---|
| **Go** | 1.25+ | Сборка бинарника |
| **Node.js** | 22+ | Сборка фронтенда |
| **dnsmasq** | любой | На целевой машине |

### Сборка из исходников

```bash
# 1. Сборка фронтенда (результат → frontend/dist/)
cd frontend && npm ci && npm run build && cd ..

# 2. Сборка бинарника
go build -o intermasq .
```

### Production-сборка (оптимизированная, с версией)

```bash
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=3.1.0" -o intermasq .
```

### Запуск

```bash
# Обязательно: задайте секретный ключ (иначе процесс упадёт при старте)
export INTERMASQ_SECRET="$(openssl rand -hex 32)"

sudo ./intermasq \
  -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

При первом запуске откроется экран **настройки администратора**. После создания
учётной записи — полный доступ ко всем вкладкам панели.

> 💡 **Совет:** для production пропишите `INTERMASQ_SECRET` в systemd-unit через
> drop-in: `systemctl edit intermasq` → `Environment="INTERMASQ_SECRET=<hex>"`.

---

## ⚙️ Конфигурация

### Флаги командной строки

| Флаг | По умолчанию | Описание |
|---|---|---|
| `-port` | `8081` | Порт для прослушивания |
| `-db` | `/etc/intermasq/users.json` | Путь к базе пользователей |
| `-conf-dir` | `/etc/dnsmasq.d` | Директория конфигов dnsmasq |
| `-leases` | `/var/lib/misc/dnsmasq.leases` | Путь к файлу аренд dnsmasq |
| `-arp-file` | `/proc/net/arp` | Путь к ARP-таблице |
| `-init-system` | `auto` | Init-система: `auto`, `systemd`, `systemd-user`, `openrc`, `runit`, `sysvinit`, `none` |
| `-systemd-scope` | — | *(устаревший)* `auto`, `system`, `user`, `none` |
| `-ci-mode` | `false` | Отключает саморестарт (для CI/тестов) |
| `-audit-log` | `/etc/intermasq/audit.log` | Путь к файлу аудита |
| `-templates` | `/etc/intermasq/templates.json` | Путь к файлу шаблонов |
| `-history-dir` | `/etc/intermasq/history` | Директория для версий конфигов |
| `-history-depth` | `10` | Сколько версий каждого файла хранить |
| `-dnsmasq-bin` | *(авто)* | Путь к `dnsmasq` (авто-resolve через `$PATH`) |
| `-sudo-bin` | *(авто)* | Путь к `sudo` |
| `-systemctl-bin` | *(авто)* | Путь к `systemctl` |
| `-service-bin` | *(авто)* | Путь к `service` (sysvinit) |
| `-rc-service-bin` | *(авто)* | Путь к `rc-service` (OpenRC) |
| `-sv-bin` | *(авто)* | Путь к `sv` (runit) |

> **Почему порт `8081`?** Начиная с v3.0 порт по умолчанию изменён с `8080`.
> Порт `8080` часто занят другими сервисами (например, Crowdsec слушает на
> `127.0.0.1:8080`), что приводило к молчаливому падению службы. Для старого
> порта указывайте его явно: `-port 8080`.

### Переменные окружения

| Переменная | Обязательная | Описание |
|---|---|---|
| `INTERMASQ_SECRET` | ✅ **Да** | Секретный ключ для подписи JWT и `X-API-Key`. Сгенерируйте: `openssl rand -hex 32` |

---

## 🔌 API и Swagger

После запуска доступна интерактивная документация:

```
http://<host>:<port>/swagger/index.html
```

<details>
<summary><b>📋 Основные эндпоинты (нажмите, чтобы развернуть)</b></summary>

| Метод | Путь | Описание | Auth |
|---|---|---|---|
| `GET` | `/api/status` | Статус dnsmasq + флаг первичной настройки | — |
| `POST` | `/api/setup` | Создание администратора | — |
| `POST` | `/api/login` | Вход, получение JWT (с rate-limit) | — |
| `GET` | `/api/hosts` | Список статических хостов | ✅ |
| `POST` | `/api/hosts` | Добавить / обновить хост | ✅ |
| `POST` | `/api/hosts/bulk` | Массовый импорт хостов | ✅ |
| `POST` | `/api/hosts/bulk-move` | Переместить хосты в другой файл | ✅ |
| `POST` | `/api/hosts/bulk-edit` | Массовое редактирование | ✅ |
| `GET` / `POST` | `/api/hosts/csv` | Экспорт / импорт CSV | ✅ |
| `DELETE` | `/api/hosts/:mac` | Удалить хост | ✅ |
| `GET` | `/api/aliases` | DNS-записи (A/CNAME/PTR/TXT) | ✅ |
| `POST` | `/api/aliases` | Добавить DNS-запись | ✅ |
| `POST` | `/api/aliases/bulk` | Массовый импорт DNS-записей | ✅ |
| `GET` / `POST` | `/api/aliases/csv` | Экспорт / импорт DNS в CSV | ✅ |
| `GET` | `/api/leases` | DHCP-аренды | ✅ |
| `GET` | `/api/arp` | ARP-таблица (онлайн MAC) | ✅ |
| `POST` | `/api/leases/to-static` | Массовый перенос аренд в статику | ✅ |
| `GET` | `/api/new-devices` | Неизвестные устройства (ARP + OUI) | ✅ |
| `GET` / `PUT` | `/api/config` | Снимок конфигурации dnsmasq | ✅ |
| `POST` / `DELETE` | `/api/config/file` | Создать / удалить `.conf`-файл | ✅ |
| `GET` / `PUT` | `/api/files/:name` | Raw-чтение / запись `.conf`-файла | ✅ |
| `GET` | `/api/templates` | Шаблоны хостов | ✅ |
| `POST` | `/api/rollback` | Быстрый откат файла до `.bak` | ✅ |
| `GET` | `/api/history` | Список версий файла | ✅ |
| `GET` | `/api/history/diff` | Diff между версиями | ✅ |
| `POST` | `/api/history/restore` | Восстановить файл из версии | ✅ |
| `GET` | `/api/backup` | Скачать ZIP-архив всех `.conf` | ✅ |
| `POST` | `/api/backup/restore` | Восстановить из ZIP | ✅ |
| `GET` | `/api/audit` | Журнал аудита | ✅ |
| `GET` | `/api/users` | Список пользователей | ✅ |
| `POST` | `/api/users` | Создать пользователя | ✅ |
| `DELETE` | `/api/users/:name` | Удалить пользователя | ✅ |
| `POST` | `/api/users/password` | Смена пароля | ✅ |
| `POST` | `/api/reload` | Проверка конфига + перезапуск dnsmasq | ✅ |
| `POST` | `/api/restart-self` | Перезапуск сервиса Intermasq | ✅ |
| `POST` | `/api/logout` | Выход + отзыв JWT | ✅ |
| `GET` | `/api/events` | SSE-стрим (ARP, статус dnsmasq) | ✅ |
| `GET` | `/api/plugins` | Список загруженных плагинов | ✅ |

</details>

### Аутентификация

| Сценарий | Способ |
|---|---|
| 🌐 **Браузер** | `Authorization: Bearer <JWT>` |
| 🤖 **Скрипты / плагины** | `X-API-Key: <INTERMASQ_SECRET>` |
| 📊 **Prometheus / SSE** | `?token=<JWT-или-SECRET>` в URL |

---

## 🧩 Плагины

Intermasq расширяется через **Unix-сокеты**. Каждый плагин — это каталог в
`/etc/intermasq/plugins/` с `manifest.json`:

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "bin": "./plugin-binary"
}
```

При старте Intermasq:

1. Читает `/etc/intermasq/plugins/<id>/manifest.json`
2. Запускает бинарник, передавая переменные окружения:
   - `INTERMASQ_KEY` — секрет для API-запросов
   - `PLUGIN_SOCKET` — путь к Unix-сокету (`/run/intermasq/sockets/<id>.sock`)
3. Проксирует все запросы `/plugins/<id>/*` на Unix-сокет плагина
4. Отображает плагин в **iframe** в полноэкранном overlay

> 🔄 Добавление плагина «на лету» → `Меню → 🔄 Рестарт Intermasq`.

---

## 📊 Метрики для Prometheus

Эндпоинт `/metrics` отдаёт operational-метрики в exposition-формате:

| Метрика | Описание |
|---|---|
| `intermasq_hosts_total` | Кол-во управляемых dhcp-host записей |
| `intermasq_leases_active` | Текущее кол-во активных DHCP-аренд |
| `intermasq_arp_online_total` | Устройств онлайн по ARP |
| `intermasq_dnsmasq_active` | `1` если dnsmasq активен, иначе `0` |
| `intermasq_reloads_total` | Успешных reload'ов через панель |
| `intermasq_dnsmasq_test_failures_total` | Сколько раз `dnsmasq --test` отклонил изменение |
| `intermasq_uptime_seconds` | Аптайм процесса |
| `intermasq_domain_up{domain=…}` | Резолвится ли домен (health-check каждые 60с) |
| `intermasq_domain_resolve_seconds{domain=…}` | Latency последнего резолва |

Пример `scrape_config`:

```yaml
scrape_configs:
  - job_name: intermasq
    scrape_interval: 30s
    metrics_path: /metrics
    params:
      token: ['<INTERMASQ_SECRET>']
    static_configs:
      - targets: ['172.20.0.1:8081']
```

Пример алерта в Grafana/Alertmanager:

```promql
intermasq_domain_up{domain="wiki.lan"} == 0
```

---

## 📁 Структура проекта

```
.
├── main.go                 # Точка входа: флаги, bootstrap, gin engine, статика/swagger (тонкий)
├── internal/               # Вся бизнес-логика (раньше — плоский package main)
│   ├── models/             # Типы данных (HostEntry, DnsAliasEntry, …)
│   ├── validate/           # Валидаторы MAC/IP/hostname/tag + нормализаторы
│   ├── oui/                # Таблица OUI (определение вендора по MAC)
│   ├── stats/              # Счётчики stats
│   ├── bins/               # Авто-resolve путей к системным бинарникам
│   ├── initd/              # SystemCaller — детект и управление init-системами
│   ├── dnsmasq/            # Ядро dhcp-host: парсинг/запись, алиасы, конфиг, история, backup
│   ├── netstate/           # ARP, leases, обнаружение устройств
│   ├── templates/          # Шаблоны хостов (создание/применение)
│   ├── auth/               # Пользователи, JWT, rate-limit, middleware (bcrypt)
│   ├── audit/              # Журнал аудита
│   ├── control/            # SSE broadcaster, статус/reload dnsmasq
│   ├── metrics/            # /metrics для Prometheus + DNS health-check
│   ├── plugins/            # Загрузка/проксирование плагинов (Unix-сокеты)
│   └── webapi/             # HTTP-обработчики + регистрация роутов (/api/*)
├── docs/                   # OpenAPI-спецификация + документация фич
│   ├── swagger.yaml / swagger.json
│   └── docs.go
├── frontend/               # Vue 3 SPA
│   ├── src/
│   │   ├── App.vue             # Корневой компонент (навбар, табы, темы, меню)
│   │   ├── store.js            # Реактивное хранилище + axios
│   │   ├── api/                # API-клиенты по доменам (hosts, dns, config, system)
│   │   ├── i18n.js             # vue-i18n (RU/EN)
│   │   ├── locales/            # ru.json, en.json
│   │   └── components/         # static/ dns/ config/ safety/ history/ audit/ …
│   └── vite.config.js
├── .forgejo/workflows/     # CI: сборка, тесты, smoke
├── tests/                  # Smoke-сьюты, Playwright E2E, L5 (живые ВМ)
├── LICENSE                 # GNU AGPL v3
└── README.md               # Этот файл
```

> 💡 **Почему `main.go` в корне, а не в `cmd/intermasq/`?** Директива
> `//go:embed frontend/dist/*` не умеет подниматься по дереву (`../`), поэтому
> точка входа обязана жить рядом с `frontend/`. Сборка остаётся прежней:
> `go build -o intermasq .`

---

## 🛠 Стек технологий

<details open>
<summary><b>⚙️ Бэкенд</b></summary>

- **Go 1.25** — язык, один статический бинарник
- **Gin** — HTTP-фреймворк
- **golang-jwt/v5** — JWT-токены
- **golang.org/x/crypto** — bcrypt для паролей
- **gin-swagger** — Swagger UI из OpenAPI-спецификации
- **go:embed** — встраивание фронтенда в бинарник

</details>

<details open>
<summary><b>🎨 Фронтенд</b></summary>

- **Vue 3** — Composition API, `<script setup>`
- **Vite 7** — дев-сервер и сборка
- **Bootstrap 5** — UI-компоненты, темизация (dark/light)
- **vue-i18n 9** — локализация (🇷🇺 RU / 🇬🇧 EN)
- **Axios** — HTTP-клиент
- **event-source-polyfill** — SSE-клиент

</details>

<details open>
<summary><b>🔧 Инфраструктура и качество</b></summary>

- **Forgejo Actions** — CI/CD (сборка, линтеры, тесты)
- **go vet / gofmt** — статический анализ и форматирование Go
- **go test** — unit-тесты (включая `-race`)
- **Playwright** — E2E-тесты фронтенда

</details>

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
