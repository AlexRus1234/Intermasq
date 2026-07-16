# Новые возможности Intermasq

Начиная с v3.1 в Intermasq добавлены 8 новых функций:
редактор конфигурационных файлов, SSE-события, управление пользователями,
определение вендора по MAC (OUI), восстановление из ZIP, rate-limit на логин,
logout с отзывом JWT, массовый перенос lease → static, и вкладка «Новые устройства».

---

## 1. Редактор конфигурационных файлов

Просмотр и редактирование любых `.conf` файлов в `-conf-dir` через веб-интерфейс.

### API

```
GET /api/files/:name
Authorization: Bearer <token>

→ 200 {"content":"..."}
→ 404 {"error":"file_not_found"}
→ 400 {"error":"unsafe_path"}
```

```
PUT /api/files/:name
Authorization: Bearer <token>
Content-Type: application/json

{"content":"новое содержимое\n"}

→ 200 {"message":"ok"}
→ 400 {"error":"unsafe_path"}
→ 400 {"error":"validation_error"}
→ 400 {"error":"dnsmasq_test_failed"}
```

### Механизм работы

1. PUT создаёт `.bak`-копию файла перед записью.
2. После записи запускает `dnsmasq --test` для проверки синтаксиса.
3. При неудаче теста автоматически откатывает изменения.
4. При успехе сохраняет версию в историю (см. `docs/version-history.md`).

---

## 2. SSE-события (Server-Sent Events)

Канал реального времени для уведомлений о событиях в системе.

### API

```
GET /api/events
Content-Type: text/event-stream

data: {"type":"config_updated","data":"hosts.conf"}

data: {"type":"user_changed","data":"created"}
```

### Использование

Подключение через `EventSource` в браузере:
```js
const es = new EventSource('/api/events');
es.onmessage = (e) => {
  const evt = JSON.parse(e.data);
  // evt.type, evt.data
};
```

### События

- `config_updated` — конфигурационный файл изменён через PUT /api/files/:name
- `user_changed` — пользователь создан/удалён
- В будущем: lease changes, device discovery, system alerts

---

## 3. Управление пользователями

CRUD для пользователей через веб-интерфейс (вкладка «Пользователи»).

### API

```
POST /api/users
Authorization: Bearer <token>
Content-Type: application/json

{"login":"newuser","password":"secret123"}

→ 201 {"message":"created"}
→ 400 {"error":"empty_fields"}
→ 409 {"error":"user_exists"}
```

```
DELETE /api/users/:login
Authorization: Bearer <token>

→ 200 {"message":"deleted"}
→ 400 {"error":"cannot_delete_self"}
→ 404 {"error":"user_not_found"}
```

```
PUT /api/users/me/password
Authorization: Bearer <token>
Content-Type: application/json

{"old_password":"old","new_password":"new"}

→ 200 {"message":"changed"}
→ 400 {"error":"wrong_password"}
→ 400 {"error":"empty_fields"}
```

### Безопасность

- Пароли хранятся в bcrypt.
- Нельзя удалить самого себя.
- Все руты под `authMiddleware`.

---

## 4. Вкладка «Новые устройства»

Автоматическое обнаружение устройств в ARP-таблице, которых ещё нет
в статических конфигурациях.

### API

```
GET /api/devices/new
Authorization: Bearer <token>

→ 200 [
  {"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.100","oui":"Apple Inc."},
  {"mac":"11:22:33:44:55:66","ip":"192.168.1.101","oui":""}
]
```

### Логика

1. Читает `arp.json` из `-dir`.
2. Парсит записи через `parseArpContent`.
3. Для каждого MAC проверяет наличие `dhcp-host=` в статических хостах
   и hosts-файлах.
4. Если MAC не найден — возвращает устройство как «новое».
5. Определяет вендора через OUI-таблицу (первые 3 октета MAC).

### UI

- Таблица с колонками: MAC, IP, Вендор (OUI).
- Кнопка «Добавить как статический» для каждого устройства.
- При добавлении вызывается bulkLeaseToStatic для одного хоста.

---

## 5. Массовый перенос lease → static

Перенос выбранных динамических аренд в статические `dhcp-host=` записи.

### API

```
POST /api/leases/to-static
Authorization: Bearer <token>
Content-Type: application/json

{
  "leases": [
    {"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.1.1","hostname":"myhost"}
  ],
  "file": "hosts.conf"
}

→ 200 {"message":"ok"}
→ 400 {"error":"invalid_data"}
→ 400 {"error":"invalid_mac"}
→ 409 {"error":"mac_conflict"}
```

### Особенности

- **MAC-конфликт:** если `dhcp-host=<mac>` уже есть в целевом файле —
  возвращается 409, запись не добавляется.
- **Авто-hostname:** если hostname == `*`, генерируется `device-{ip без точек}`
  (например `device-19216811`).
- **Валидация MAC:** строгий формат `xx:xx:xx:xx:xx:xx`.
- **Unsafe path:** пути с `..` блокируются.

### UI

- Чекбоксы для выбора lease в таблице аренд.
- Выпадающий список для выбора целевого файла.
- Кнопка «Перенести выбранные в статику».
- Обработка ошибок: конфликты MAC показываются в toast.

---

## 6. Восстановление из ZIP-архива

Загрузка ZIP-архива с конфигурационными файлами и восстановление
с созданием backup-копии.

### API

```
POST /api/files/restore
Authorization: Bearer <token>
Content-Type: multipart/form-data

file: backup.zip

→ 200 {"message":"ok"}
→ 400 {"error":"invalid_zip"}
→ 400 {"error":"no_conf_files"}
→ 400 {"error":"dnsmasq_test_failed"}
```

### Механизм работы

1. Принимает ZIP-архив через multipart/form-data.
2. Распаковывает in-memory, для каждого `.conf`-файла:
   - Проверяет path traversal (отклоняет `../` и абсолютные пути).
   - Создаёт `.restore.bak` из текущей версии файла.
   - Записывает содержимое из архива.
3. Запускает `dnsmasq --test` для проверки всей конфигурации.
4. При неудаче откатывает все изменённые файлы из `.restore.bak`.

### Безопасность

- Файлы за пределами `-conf-dir` игнорируются.
- Архивы без `.conf` файлов возвращают `no_conf_files`.
- Некорректные ZIP-архивы возвращают `invalid_zip`.

---

## 7. Rate-limit на `/api/login`

Защита от brute-force атак на endpoint логина.

### Конфигурация

- **Максимум попыток:** 5 (по умолчанию) с одного IP.
- **Интервал:** 1 минута (по умолчанию).
- При превышении — `429 Too Many Requests` с телом `{"error":"too_many_requests"}`.
- При успешном входе счётчик для IP сбрасывается.

### Очистка

- Фоновый cleanup запускается каждые 10 минут, удаляя истёкшие записи.
- Используется `sync.Mutex` для потокобезопасности.

---

## 8. Logout / JWT blacklist

Возможность принудительного завершения сессии с отзывом JWT-токена.

### API

```
POST /api/logout
Authorization: Bearer <token>

→ 200 {"message":"logged_out"}
```

### Механизм

1. Клиент отправляет `POST /api/logout` с токеном в `Authorization`.
2. Бэкенд парсит токен, извлекает `jti` (JWT ID) и время жизни (`exp`).
3. `jti` сохраняется в `TokenBlacklist` до истечения срока токена.
4. `JWTAuthMiddleware` проверяет `blacklist.IsRevoked()` при каждом запросе.
5. Фоновый cleanup удаляет истёкшие записи каждые 30 минут.

### Безопасность

- Отозванный токен недействителен немедленно.
- Истёкшие записи автоматически удаляются из памяти.
- Используется `sync.RWMutex` для конкурентного доступа.

---

## 9. OUI Lookup (определение вендора по MAC)

Встроенная таблица производителей сетевых устройств по первым 3 октетам MAC.

### Таблица

Включает ~50 известных OUI: VMware, Apple, Intel, Cisco, Samsung, LG,
Google, Huawei, Xiaomi, Dell, HP, Lenovo, Asus, Acer, Nokia, Ericsson,
Motorola, Broadcom, Qualcomm, Roku, Sonos, TP-Link, D-Link, Netgear,
Linksys, Ubiquiti, MikroTik, Zyxel, Lutron, Philips, Belkin, Amazon,
Raspberry Pi, Arduino, Espressif, Texas Instruments, NXP, Sierra Wireless,
Honeywell, Siemens, Bosch, Panasonic, Sony, Yamaha, Canon, Epson, Nikon

### Формат MAC

Принимает MAC в любом регистре: `aa:bb:cc:dd:ee:ff`, `AA:BB:CC:DD:EE:FF`.

### API (внутренняя)

```go
vendor := LookupOUI("aa:bb:cc:dd:ee:ff") // → "Apple" или ""
```

---

## Изменённые файлы

| Файл | Описание |
|---|---|
| `auth.go` | JWT blacklist, rate-limiter, CRUD пользователей |
| `dnsmasq.go` | SSE broker, file I/O, restore ZIP, new devices |
| `handlers.go` | REST-хендлеры для всех новых функций |
| `main.go` | Инициализация, новые роуты |
| `models.go` | Новые структуры данных |
| `oui.go` | OUI-таблица и LookupOUI |
| `dnsmasq_test.go` | 47 тестов для новых функций |
| `frontend/src/App.vue` | Вкладки «Новые устройства», «Пользователи» |
| `frontend/src/components/NewDevicesTab.vue` | Компонент новых устройств |
| `frontend/src/components/UsersTab.vue` | Компонент управления пользователями |
| `frontend/src/components/leases/LeasesTab.vue` | Bulk lease-to-static UI |
| `frontend/src/store.js` | API вызовы для новых функций |
| `frontend/src/locales/en.json` | Английские переводы |
| `frontend/src/locales/ru.json` | Русские переводы |
