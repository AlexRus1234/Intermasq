[**English**](README.en.md) | **Русский**

<div align="center">

# 🛡️ Intermasq

**Веб-панель для управления dnsmasq**

Лёгкий, быстрый, один бинарник — всё в комплекте.

[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg)](https://vuejs.org/)

</div>

---

## 📸 Скриншоты

> *Скриншоты будут добавлены позже.*

---

## ✨ Ключевые фишки

- **Управление статическими DHCP-записями** — добавление, редактирование, удаление хостов через веб-интерфейс
- **Мониторинг устройств** — онлайн/офлайн статус через ARP-таблицу (автообновление каждые 30 сек)
- **Конвертация аренды в статику** — перенос DHCP-аренды в статическую запись одним кликом
- **Массовый импорт** — вставка списка устройств из текста (MAC IP Hostname)
- **Бэкап и откат** — скачивание ZIP-архива конфигов и откат изменений по файлам (`.bak`)
- **Мульти-init** — автоопределение и поддержка: systemd, systemd-user, OpenRC, runit, sysvinit
- **Плагинная система** — расширение функционала через Unix-сокеты
- **Один бинарник** — фронтенд встроен в Go-бинарник через `go:embed`
- **Двуязычный интерфейс** — русский / английский, переключение в один клик
- **Тёмная / светлая тема** — с сохранением выбора в localStorage
- **Двойная аутентификация** — JWT для браузера, `X-API-Key` для скриптов и плагинов
- **Swagger UI** — интерактивная API-документация из коробки (`/swagger/index.html`)
- **Массовое удаление** — выбор нескольких хостов чекбоксами и удаление разом
- **Сортировка и поиск** — по MAC, IP, Hostname с умной сортировкой IP-адресов
- **Безопасность путей** — защита от path traversal, записи только внутри `conf-dir`

---

## 🚀 Быстрый старт

### Зависимости

- Go 1.25+
- Node.js 22+ (для сборки фронтенда)
- dnsmasq (на целевой машине)

### Сборка

```bash
# Сборка фронтенда
cd frontend && npm ci && npm run build && cd ..

# Сборка бинарника
go build -o intermasq .
```

### Запуск

```bash
sudo ./intermasq -port 8080 -conf-dir /etc/dnsmasq.d -leases /var/lib/misc/dnsmasq.leases
```

При первом запуске откроется экран настройки администратора. После создания аккаунта — полный доступ к панели.

### Сборка для production (с версией)

```bash
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=3.0.0" -o intermasq .
```

---

## ⚙️ Флаги командной строки

| Флаг | По умолчанию | Описание |
|---|---|---|
| `-port` | `8080` | Порт для прослушивания |
| `-db` | `/etc/intermasq/users.json` | Путь к базе пользователей |
| `-conf-dir` | `/etc/dnsmasq.d` | Директория конфигов dnsmasq |
| `-leases` | `/var/lib/misc/dnsmasq.leases` | Путь к файлу аренд dnsmasq |
| `-arp-file` | `/proc/net/arp` | Путь к ARP-таблице |
| `-init-system` | `auto` | Init-система: `auto`, `systemd`, `systemd-user`, `openrc`, `runit`, `sysvinit`, `none` |
| `-systemd-scope` | — | *(устаревший)* `auto`, `system`, `user`, `none` |
| `-ci-mode` | `false` | Отключает саморестарт (для CI) |

### Переменные окружения

| Переменная | Описание |
|---|---|
| `INTERMASQ_SECRET` | Секретный ключ для JWT и API-Key. Если не задан — используется дефолтный |

---

## 🔌 API

После запуска доступна Swagger-документация: `http://<host>:<port>/swagger/index.html`

### Основные эндпоинты

| Метод | Путь | Описание | Auth |
|---|---|---|---|
| `GET` | `/api/status` | Статус dnsmasq + необходимость setup | Нет |
| `POST` | `/api/setup` | Первичная настройка администратора | Нет |
| `POST` | `/api/login` | Вход, получение JWT | Нет |
| `GET` | `/api/hosts` | Список статических хостов | Да |
| `POST` | `/api/hosts` | Добавить/обновить хост | Да |
| `POST` | `/api/hosts/bulk` | Массовый импорт хостов | Да |
| `DELETE` | `/api/hosts/:mac` | Удалить хост | Да |
| `GET` | `/api/leases` | DHCP-аренды | Да |
| `GET` | `/api/arp` | ARP-таблица (онлайн MAC) | Да |
| `POST` | `/api/reload` | Проверка конфига + перезапуск dnsmasq | Да |
| `POST` | `/api/rollback` | Откат файла до `.bak` версии | Да |
| `GET` | `/api/backup` | Скачать ZIP-архив всех `.conf` | Да |
| `POST` | `/api/restart-self` | Перезапуск сервиса Intermasq | Да |
| `GET` | `/api/plugins` | Список загруженных плагинов | Да |

### Аутентификация

- **Браузер**: JWT-токен в заголовке `Authorization: Bearer <token>`
- **Скрипты/плагины**: статический ключ в заголовке `X-API-Key: <INTERMASQ_SECRET>`

---

## 🧩 Плагинная система

Intermasq поддерживает расширения через Unix-сокеты. Каждый плагин — это каталог в `/etc/intermasq/plugins/` с `manifest.json`:

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "bin": "./plugin-binary"
}
```

При старте Intermasq:
1. Читает `/etc/intermasq/plugins/<id>/manifest.json`
2. Запускает бинарник с переменными окружения:
   - `INTERMASQ_KEY` — секрет для API-запросов
   - `PLUGIN_SOCKET` — путь к Unix-сокету (`/run/intermasq/sockets/<id>.sock`)
3. Проксирует все запросы `/plugins/<id>/*` на Unix-сокет плагина
4. UI отображает плагин в iframe

---

## 📁 Структура проекта

```
.
├── main.go              # Точка входа, роутинг Gin, загрузка плагинов
├── auth.go              # JWT, аутентификация, пользователи (bcrypt)
├── handlers.go          # HTTP-обработчики API
├── models.go            # Структуры данных (HostEntry, LeaseEntry и др.)
├── dnsmasq.go           # Работа с конфигами dnsmasq, ARP, бэкапы, откат
├── system.go            # Абстракция init-систем (SystemCaller interface)
├── dnsmasq_test.go      # Unit-тесты (ARP, пути, init-системы)
├── go.mod / go.sum      # Зависимости Go
├── docs/
│   ├── swagger.yaml     # Спецификация OpenAPI
│   ├── swagger.json     # Спецификация OpenAPI (JSON)
│   └── docs.go          # Генерированный код для gin-swagger
├── frontend/
│   ├── package.json     # Зависимости Node.js
│   ├── vite.config.js   # Конфигурация Vite
│   ├── index.html       # Точка входа HTML
│   └── src/
│       ├── main.js          # Инициализация Vue + i18n
│       ├── App.vue          # Корневой компонент (навбар, табы, темы)
│       ├── store.js         # Реактивное хранилище + API-клиент (axios)
│       ├── i18n.js          # Настройка vue-i18n + перевод ошибок API
│       ├── style.css        # Глобальные стили
│       ├── locales/
│       │   ├── ru.json      # Русская локаль
│       │   └── en.json      # Английская локаль
│       └── components/
│           ├── AuthScreen.vue       # Экран входа / настройки
│           ├── static/
│           │   ├── StaticView.vue  # Вкладка статических хостов
│           │   ├── HostForm.vue     # Форма добавления/редактирования + bulk
│           │   ├── HostTable.vue    # Таблица хостов (сортировка, выбор)
│           │   └── BulkImport.vue   # Компонент массового импорта
│           └── leases/
│               └── LeasesTab.vue    # Вкладка DHCP-аренд
├── .forgejo/
│   └── workflows/
│       ├── build.yml       # CI: сборка, тесты, smoke-тест
│       └── release.yml     # Release: сборка, sha256, загрузка в Forgejo Packages
├── LICENSE               # GNU AGPL v3
└── README.md             # Документация
```

---

## 🛠 Стек технологий

### Бэкенд
- **Go 1.25** — язык, один бинарник
- **Gin** — HTTP-фреймворк
- **jwt/v5** — JWT-токены
- **bcrypt** (golang.org/x/crypto) — хеширование паролей
- **gin-swagger** — Swagger UI
- **go:embed** — встраивание фронтенда в бинарник

### Фронтенд
- **Vue 3** — Composition API, `<script setup>`
- **Vite 7** — сборка
- **Bootstrap 5** — UI-компоненты и темы
- **vue-i18n 9** — локализация (RU / EN)
- **Axios** — HTTP-клиент

### Инфраструктура
- **Forgejo Actions** — CI/CD
- **Go vet + gofmt** — статический анализ и форматирование
- **go test** — unit-тесты

---

## 📄 Лицензия

Проект распространяется под лицензией [GNU Affero General Public License v3.0](LICENSE).

Copyright (C) 2026 AlexRus1234
