# API, аутентификация и разграничение прав

Intermasq предоставляет REST API на базе Gin. Базовый путь `/api`; Swagger UI доступен по
адресу `http://<host>:<port>/swagger/index.html`, спецификация —
`docs/swagger.yaml` / `docs/swagger.json`.

---

## Аутентификация

На защищённых эндпоинтах поддерживаются два способа аутентификации:

| Сценарий | Способ | Кто |
|---|---|---|
| Браузер | `Authorization: Bearer <JWT>` | Пользователь, залогиненный через `/api/login` |
| Скрипты / плагины / Prometheus | `X-API-Key: <INTERMASQ_SECRET>` | Программный доступ |

`X-API-Key` — это и есть `INTERMASQ_SECRET` (тот же секрет, что подписывает JWT).
При успехе запрос выполняется от имени виртуального пользователя `api-key` с
ролью `admin`.

### JWT

- Алгоритм: **HS256**, ключ — `INTERMASQ_SECRET`.
- Срок жизни токена — **72 часа**.
- В токене: `sub` (имя пользователя), `exp`, `jti`, `ver` (версия отзыва),
  `role` (`admin`/`user`).

### Отзыв токенов

- **Logout** (`POST /api/logout`) кладёт `jti` в in-memory blacklist до `exp`.
- **Смена пароля / удаление пользователя** инкрементирует `ver` для этого
  пользователя — все ранее выданные токены моментально становятся невалидными
  (даже если не истёкли).
- **Рестарт процесса** сбрасывает blacklist (in-memory) — отозванные через
  logout токены снова валидны до `exp`. Это сознательное упрощение.

### Ограничение частоты запросов к `/api/login`

Допускается 10 попыток с одного IP за одну минуту; последующие запросы получают
`429 too_many_attempts`.
Счётчик сбрасывается при **успешном** входе (атакующий без пароля сброса не
получит). Хранилище in-memory, очистка ленивая (раз в 5 минут).

---

## Разграничение прав

Роли хранятся в `users.json`. Первый созданный (через `/api/setup`) пользователь
становится `admin`, все последующие (через `/api/users`) — `user`.

| Уровень | Middleware | Что разрешает |
|---|---|---|
| `auth` | `auth.Middleware` | Любой аутентифицированный пользователь: чтение, добавление хостов/DNS, шаблоны, история (просмотр/diff), backup (скачивание), аудит, CSV. |
| `admin` | `auth.AdminMiddleware` | Дополнительно: destructive-операции — reload, rollback, restore из истории/ZIP, raw-запись файлов, управление пользователями, restart-self. |

Серверная часть является источником данных о полномочиях. Интерфейс скрывает
административные элементы для `user` на основе
claim `role` в JWT, но каждый admin-запрос перепроверяется сервером. Подделка
client-side роли даёт только «неправильный» UI, но не привилегии.

---

## Эндпоинты

### Публичные эндпоинты

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/status` | Статус dnsmasq + флаг первичной настройки (есть ли хоть один пользователь) |
| `POST` | `/api/setup` | Создание первого администратора (работает только при пустой базе) |
| `POST` | `/api/login` | Вход, возвращает JWT. Под rate-limit'ом |

### Хосты (`dhcp-host`), уровень `auth`

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/hosts` | Список статических хостов |
| `POST` | `/api/hosts` | Добавить / обновить хост |
| `DELETE` | `/api/hosts/:mac` | Удалить хост |
| `GET` | `/api/hosts/next-ip` | Предложить следующий свободный IP из диапазона |
| `POST` | `/api/hosts/apply-template` | Применить шаблон (ip/hostname-паттерн) к MAC |
| `POST` | `/api/hosts/bulk` | Массовый импорт хостов |
| `POST` | `/api/hosts/bulk-move` | Переместить хосты в другой `.conf` |
| `POST` | `/api/hosts/bulk-edit` | Массовое редактирование (префикс IP, суффикс hostname) |
| `GET` | `/api/hosts/csv` | Экспорт хостов в CSV |
| `POST` | `/api/hosts/csv` | Импорт хостов из CSV |

### DNS-записи (A/CNAME/PTR/TXT), уровень `auth`

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/aliases` | Список DNS-записей |
| `POST` | `/api/aliases` | Добавить DNS-запись |
| `POST` | `/api/aliases/bulk` | Массовый импорт DNS-записей |
| `POST` | `/api/aliases/delete` | Удалить DNS-запись |
| `GET` | `/api/aliases/csv` | Экспорт DNS в CSV |
| `POST` | `/api/aliases/csv` | Импорт DNS из CSV |

### Аренды, ARP и обнаружение устройств, уровень `auth`

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/leases` | DHCP-аренды из файла `-leases` |
| `GET` | `/api/arp` | ARP-таблица (онлайн MAC → bool) |
| `GET` | `/api/new-devices` | Неизвестные MAC + определение вендора (OUI) |
| `POST` | `/api/leases/to-static` | Массовый перенос аренд в статику (без `dnsmasq --test`!) |

> `POST /api/leases/to-static` пишет строки напрямую в файл, не запуская
> `dnsmasq --test` — ради скорости массовой операции. Активация изменений —
> через обычную кнопку «Применить» (`POST /api/reload`).

### Конфигурация dnsmasq, уровни `auth` и `admin`

| Метод | Путь | Уровень | Описание |
|---|---|---|---|
| `GET` | `/api/config` | auth | Снимок всех директив (dhcp-range/option/server/PXE/...) |
| `PUT` | `/api/config` | auth | Обновить директивы файла (визуальный редактор) |
| `POST` | `/api/config/file` | auth | Создать новый `.conf` (с опц. шаблоном) |
| `DELETE` | `/api/config/file` | auth | Удалить `.conf` (физически) |
| `GET` | `/api/config/templates` | auth | Список пресетов конфига (basic-dhcp, forwarder, pxe, ...) |
| `GET` | `/api/files/:name` | auth | Raw-чтение `.conf` |
| `PUT` | `/api/files/:name` | **admin** | Raw-запись `.conf` (plain-text редактор) |

### Шаблоны хостов, уровень `auth`

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/templates` | Список шаблонов (ip-диапазон + hostname-паттерн + target-файл) |
| `POST` | `/api/templates` | Создать шаблон |
| `DELETE` | `/api/templates/:id` | Удалить шаблон |
| `GET` | `/api/templates/ranges` | Известные `dhcp-range=` (для выбора target-диапазона) |

### История, резервное копирование и перезагрузка

| Метод | Путь | Уровень | Описание |
|---|---|---|---|
| `GET` | `/api/history` | auth | Список версий файла |
| `GET` | `/api/history/diff` | auth | Diff между версиями / версией и текущим файлом |
| `POST` | `/api/history/restore` | **admin** | Восстановить файл из версии |
| `POST` | `/api/rollback` | **admin** | Быстрый откат файла до `.bak` (один шаг) |
| `GET` | `/api/backup` | auth | Скачать ZIP-архив всех `.conf` |
| `POST` | `/api/backup/restore` | **admin** | Восстановить из ZIP (с pre-flight `dnsmasq --test`) |
| `POST` | `/api/reload` | **admin** | `dnsmasq --test` + рестарт сервиса |

### Пользователи и сессии

| Метод | Путь | Уровень | Описание |
|---|---|---|---|
| `GET` | `/api/users` | **admin** | Список пользователей |
| `POST` | `/api/users` | **admin** | Создать пользователя (роль `user`) |
| `DELETE` | `/api/users/:name` | **admin** | Удалить (нельзя удалить себя) |
| `POST` | `/api/users/password` | auth | Смена своего пароля (отзывает свои токены) |
| `POST` | `/api/logout` | auth | Выход + отзыв текущего JWT |

### Эксплуатационные эндпоинты

| Метод | Путь | Уровень | Описание |
|---|---|---|---|
| `GET` | `/api/events` | auth | SSE-стрим: события `arp` и `dnsmasq_status` |
| `GET` | `/api/audit` | auth | Журнал аудита (кто/что/когда, с цветными метками) |
| `GET` | `/api/plugins` | auth | Список загруженных плагинов |
| `POST` | `/api/restart-self` | **admin** | Перезапуск сервиса Intermasq через супервизор |

### Эндпоинты вне `/api`

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/metrics` | Метрики Prometheus. Auth в обработчике: `Authorization: Bearer <JWT>` или `X-API-Key: <SECRET>` |
| `GET` | `/plugins/<id>/*` | Reverse-proxy на Unix-сокет плагина. Под `auth.Middleware` |
| `GET` | `/swagger/*any` | Swagger UI (без auth) |

---

## Примеры

### Логин и работа с хостом через JWT

```bash
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"secret"}' | jq -r .token)

curl -s http://localhost:8081/api/hosts \
  -H "Authorization: Bearer $TOKEN"
```

### То же через `X-API-Key` (для скриптов)

```bash
curl -s http://localhost:8081/api/hosts \
  -H "X-API-Key: $INTERMASQ_SECRET"
```

### Добавить хост

```bash
curl -s -X POST http://localhost:8081/api/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.50","hostname":"iot","file":"/etc/dnsmasq.d/hosts.conf"}'
```
