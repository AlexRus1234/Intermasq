[**English**](README.en.md) | **Русский**

# Intermasq

Веб-панель для управления [dnsmasq](https://thekelleys.org.uk/dnsmasq/doc.html).
Intermasq собирается в один Go-бинарник: Vue-интерфейс встраивается внутрь
бинарника, отдельная база данных не требуется.

[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8.svg)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg)](https://vuejs.org/)

## Возможности

- Управление статическими DHCP-хостами (`dhcp-host=`): CRUD, теги, lease-time,
  поиск, сортировка, CSV и массовый импорт/перенос/редактирование.
- Просмотр DHCP-аренд и ARP-таблицы, обнаружение новых устройств и
  определение производителя по OUI.
- DNS-записи `address=`, `cname=`, `ptr-record=` и `txt-record=`, включая CSV
  и массовые операции.
- Визуальный редактор конфигурации dnsmasq и raw-редактор `.conf`.
- Шаблоны хостов и конфигурационных файлов, автоматический подбор свободного
  IPv4-адреса.
- Проверка изменяемых конфигураций через `dnsmasq --test` с откатом при ошибке.
- Быстрый откат через `.bak`, многоуровневая история с diff/restore и ZIP
  backup/restore.
- Аудит операций в JSON Lines.
- JWT-аутентификация для браузера, `X-API-Key` для скриптов и RBAC для
  административных операций.
- SSE-события для ARP и состояния dnsmasq, Prometheus-метрики на `/metrics`.
- Управление dnsmasq и перезапуск Intermasq через systemd, systemd-user,
  OpenRC, runit или sysvinit.
- Плагины через Unix-сокеты с проксированием в интерфейс.
- Русский и английский интерфейс, светлая и тёмная темы.

## Быстрый старт

### Требования

- Linux на целевой машине.
- Go 1.25+ для сборки backend.
- Node.js/npm для сборки frontend.
- `dnsmasq` для работы с конфигурацией и управления сервисом.

### Сборка

Рекомендуемый способ пересобирает frontend перед встраиванием его в бинарник:

```bash
make build
```

По текущему Makefile результат называется `intermasq.exe`; при ручной сборке
имя можно выбрать через `-o`.

Вручную:

```bash
cd frontend
npm ci
npm run build
cd ..
go build -o intermasq .
```

Не используйте голый `go build` после изменения frontend без предварительного
`npm run build`: Go встроит имеющееся содержимое `frontend/dist`.

### Запуск

Перед запуском задайте секрет. Процесс завершается, если `INTERMASQ_SECRET` не
задан:

```bash
export INTERMASQ_SECRET="$(openssl rand -hex 32)"

sudo ./intermasq \
  -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

Сервер по умолчанию слушает `:8081`. При первом запуске откройте
`http://<host>:8081` и создайте администратора. Первый пользователь получает
роль `admin`, последующие пользователи по умолчанию получают роль `user`.

Для production храните `INTERMASQ_SECRET` в конфигурации менеджера сервисов,
например в systemd drop-in, а не в командной строке или репозитории.

## Конфигурация

### Флаги

| Флаг | По умолчанию | Назначение |
|---|---|---|
| `-port` | `8081` | Порт HTTP-сервера |
| `-db` | `/etc/intermasq/users.json` | Файл пользователей |
| `-conf-dir` | `/etc/dnsmasq.d` | Каталог конфигураций dnsmasq |
| `-leases` | `/var/lib/misc/dnsmasq.leases` | Файл DHCP-аренд |
| `-arp-file` | `/proc/net/arp` | Файл ARP-таблицы |
| `-init-system` | `auto` | `auto`, `systemd`, `systemd-user`, `openrc`, `runit`, `sysvinit` или `none` |
| `-systemd-scope` | пусто | Устаревший способ выбора systemd scope |
| `-ci-mode` | `false` | Не выполнять фактический self-restart |
| `-audit-log` | `/etc/intermasq/audit.log` | Файл аудита |
| `-templates` | `/etc/intermasq/templates.json` | Файл шаблонов |
| `-history-dir` | `/etc/intermasq/history` | Каталог истории конфигураций |
| `-history-depth` | `10` | Число версий каждого файла |
| `-dnsmasq-bin` | auto | Явный путь к `dnsmasq` |
| `-sudo-bin` | auto | Явный путь к `sudo` |
| `-systemctl-bin` | auto | Явный путь к `systemctl` |
| `-service-bin` | auto | Явный путь к `service` |
| `-rc-service-bin` | auto | Явный путь к `rc-service` |
| `-sv-bin` | auto | Явный путь к `sv` |

Пути внешних программ сначала ищутся в `$PATH`, затем в стандартных каталогах.
Для запуска от непривилегированного пользователя операции с сервисами требуют
настроенного `sudo -n`.

### Переменные окружения

| Переменная | Обязательна | Назначение |
|---|---|---|
| `INTERMASQ_SECRET` | да | Подпись JWT, API-ключ и секрет для плагинов |

Приложение проверяет наличие значения, но не проверяет его формат и длину.
Используйте криптографически случайный секрет достаточной длины.

## Безопасность и роли

API принимает один из вариантов авторизации:

```http
Authorization: Bearer <JWT>
```

```http
X-API-Key: <INTERMASQ_SECRET>
```

`GET /metrics` также поддерживает `?token=` для Prometheus, если скрейпер не
может передать пользовательский заголовок. Для `/api/*` токен в query string
не используется.

Обычные аутентифицированные пользователи могут просматривать данные, работать
с хостами, DNS-записями, шаблонами и экспортом. Администратор дополнительно
может изменять raw-конфиги, применять и откатывать конфигурацию, восстанавливать
backup/history, управлять пользователями и перезапускать сервисы. Серверная
проверка роли является обязательной; скрытие кнопок в UI не является механизмом
безопасности.

Изменения raw/visual-конфигурации проходят проверку `dnsmasq --test` и при
ошибке откатываются. После операции «аренды в статику» нужно отдельно нажать
«Применить»: сама операция не перезапускает dnsmasq и не выполняет проверку.

JWT blacklist и rate limit хранятся в памяти процесса. Blacklist теряется после
перезапуска, а `/metrics` не учитывает отзыв JWT в blacklist.

## API и документация

- Swagger UI после запуска: `http://<host>:<port>/swagger/index.html`.
- OpenAPI: [`docs/swagger.yaml`](docs/swagger.yaml) и
  [`docs/swagger.json`](docs/swagger.json).
- API сгруппирован под `/api`: setup/login, hosts, aliases, leases, ARP,
  config/files, templates, history, backup, audit, users, reload, SSE и plugins.
- Метрики: `GET /metrics`.

Swagger UI доступен из коробки, но спецификация OpenAPI покрывает не все
маршруты текущего API. Для поведения отдельных функций используйте документы
в `docs/` и исходную регистрацию маршрутов в `internal/webapi`.

## Документация

- [`docs/dnsmasq-config.md`](docs/dnsmasq-config.md) — визуальный редактор.
- [`docs/raw-editor-and-rbac.md`](docs/raw-editor-and-rbac.md) — raw-редактор и RBAC.
- [`docs/dns-aliases.md`](docs/dns-aliases.md) — DNS aliases.
- [`docs/bulk-ops-and-templates.md`](docs/bulk-ops-and-templates.md) — bulk-операции и шаблоны.
- [`docs/config-templates.md`](docs/config-templates.md) — шаблоны `.conf`.
- [`docs/version-history.md`](docs/version-history.md) — history, diff и restore.
- [`docs/portability-and-validation.md`](docs/portability-and-validation.md) — portability и валидация.
- [`docs/v3.1-features.md`](docs/v3.1-features.md) — возможности v3.1.
- [`docs/testing-v1.md`](docs/testing-v1.md) — тестирование.

## Тестирование

Go-тесты и статические проверки:

```bash
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```

Smoke-тесты требуют Unix shell и запущенный экземпляр Intermasq:

```bash
BASE=http://localhost:8081 CONF_DIR=/etc/dnsmasq.d ./tests/smoke.sh
```

Playwright E2E:

```bash
cd tests/e2e
npm ci
npx playwright install chromium
npm run test:e2e
```

Дополнительные perf, fuzz, compatibility и real-VM проверки запускаются
отдельно и описаны в [тестовом roadmap](tests/ROADMAP.md).

## Структура проекта

```text
internal/             backend: auth, dnsmasq, webapi, metrics, plugins и другое
frontend/             Vue 3 SPA и её сборка
docs/                 OpenAPI и пользовательская документация
tests/                unit/integration, smoke, Playwright и L5
.forgejo/workflows/   CI: сборка, проверки и опциональные расширенные тесты
main.go               bootstrap и встраивание frontend
Makefile              локальная сборка
```

## Лицензия

Проект распространяется по лицензии [GNU AGPL v3.0](LICENSE).

Copyright (C) 2026 AlexRus1234
