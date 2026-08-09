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

# Опциональные поля в dhcp-host

Начиная с v3.0 в Intermasq поле `IP` и `Hostname` в форме добавления
хоста — **необязательны**. Раньше UI требовал все три поля (MAC + IP +
Hostname), что закрывало два типичных домашних сценария:

- «Дать телефону DNS-имя, но не фиксировать IP — пусть DHCP выдаёт сам».
- «Зарезервировать IP за MAC'ом без DNS-имени — для устройств, у которых
  имя не нужно».

Теперь поддержаны все четыре канонические формы `dhcp-host` dnsmasq.

---

## Четыре формы dhcp-host

| Форма в dnsmasq | Что значит | Когда использовать |
|---|---|---|
| `dhcp-host=aa:bb:cc:dd:ee:ff` | Только MAC — infinite lease, без IP, без имени | Гостевое устройство с фиксированным MAC |
| `dhcp-host=aa:bb:cc:dd:ee:ff,phone` | MAC + имя — IP выдаёт DHCP, имя резолвится через dnsmasq | Телефоны / ноутбуки / IoT — почти всегда это |
| `dhcp-host=aa:bb:cc:dd:ee:ff,192.168.1.10` | MAC + IP — статический IP без DNS-имени | Принтеры, устройства без нужды в DNS-имени |
| `dhcp-host=aa:bb:cc:dd:ee:ff,nas,192.168.1.10` | Полная запись — имя и IP зафиксированы | Серверы, NAS, всё что должно иметь предсказуемый IP и имя |

> **Рекомендация для HomeLab:** для большинства устройств подходит форма
> `MAC + hostname` (третья строка таблицы). dnsmasq выдаст IP из
> `dhcp-range`, имя будет резолвится. Меньше шансов на конфликт IP и
> проще обслуживать.

---

## UI

В форме добавления хоста (вкладка «Статика»):

- **MAC** — обязателен. Прошёл валидацию или нет, оценивает placeholder
  и форма подсветит ошибку.
- **IP** — placeholder показывает `IP (опц.)` (RU) / `IP (optional)` (EN).
  Кнопка `🎲` (Auto-IP) по-прежнему работает — она подставляет свободный IP
  из диапазона `dhcp-range`.
- **Hostname** — placeholder показывает `Имя (опц.)` / `Hostname (optional)`.

Под формой — **live preview итоговой dnsmasq-строки**. Например, при
заполненном только MAC:

```
dhcp-host=aa:bb:cc:dd:ee:ff
```

---

## Bulk-импорт сырым текстом

В режиме «Импорт списком» поддерживаются строки с 1–3 токенами:

```
aa:bb:cc:dd:ee:ff                              # только MAC
aa:bb:cc:dd:ee:ff phone                        # MAC + hostname
aa:bb:cc:dd:ee:ff 192.168.1.10                 # MAC + IP
aa:bb:cc:dd:ee:ff phone 192.168.1.10           # MAC + hostname + IP
aa:bb:cc:dd:ee:ff 192.168.1.10 phone           # MAC + IP + hostname (порядок любой)
```

Парсер определяет тип токена по содержимому:
- Похож на MAC-regex → MAC.
- Похож на IPv4 → IP.
- Остальное → hostname.

**Любой невалидный токен** (например, кривой IP) → запрос целиком
отклоняется с 400 и указанием MAC, в котором проблема. Раньше такие
строки молча пропускались — теперь явная ошибка.

---

## CSV-импорт

CSV-формат: 3 обязательные колонки + опциональная 4-я `lease_time`.
IP и hostname могут быть пустыми. Старые 3-колонные CSV (без `lease_time`)
по-прежнему импортируются — поле просто остаётся пустым:

```csv
mac,ip,hostname,lease_time
aa:bb:cc:dd:ee:ff,,
aa:bb:cc:dd:ee:ff,,phone
aa:bb:cc:dd:ee:ff,192.168.1.10,
aa:bb:cc:dd:ee:ff,192.168.1.10,nas
aa:bb:cc:dd:ee:ff,192.168.1.10,nas,12h
aa:bb:cc:dd:ee:ff,192.168.1.10,nas,infinite
```

Экспорт (`GET /api/hosts/csv`) всегда пишет заголовок из 4 колонок, так что
экспорт → импорт сохраняет `lease_time`. Мусор в 4-й колонке (не `IsLeaseTime`)
молча игнорируется — хост импортируется без lease-time.

---

## API

`POST /api/hosts` и `POST /api/hosts/bulk` принимают `ip` и `hostname`
как опциональные поля. Если поле присутствует — оно валидируется.
Контракт:

| Поле | Тип | Валидация |
|------|-----|-----------|
| `mac` | string (обязательный) | regex `^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$` |
| `ip` | string (опциональный) | если указан — `net.ParseIP` должен вернуть не-nil |
| `hostname` | string (опциональный) | если указан — `validHostname` (RFC 952/1123/1034) |
| `file` | string (обязательный) | путь внутри `-conf-dir` |
| `tags` | []string (опциональный) | `set:…` / `id:…` |
| `lease_time` | string (опциональный) | если указан — `IsLeaseTime` (`12h`, `3600`, `infinite`, …); дописывается в `dhcp-host` последним полем |

### Примеры

```http
POST /api/hosts
Authorization: Bearer <token>
Content-Type: application/json

{
  "mac": "aa:bb:cc:dd:ee:ff",
  "hostname": "phone",
  "file": "/etc/dnsmasq.d/hosts.conf"
}
```

→ 200 `{"status":"ok"}`, в файле появляется `dhcp-host=aa:bb:cc:dd:ee:ff,phone`.

```http
POST /api/hosts
{
  "mac": "aa:bb:cc:dd:ee:ff",
  "file": "/etc/dnsmasq.d/hosts.conf"
}
```

→ 200, в файле `dhcp-host=aa:bb:cc:dd:ee:ff` (только MAC).

```http
POST /api/hosts
{
  "mac": "aa:bb:cc:dd:ee:ff",
  "ip": "192.168.1.10",
  "hostname": "phone",
  "lease_time": "12h",
  "file": "/etc/dnsmasq.d/hosts.conf"
}
```

→ 200, в файле `dhcp-host=aa:bb:cc:dd:ee:ff,phone,192.168.1.10,12h` (lease-time
последним полем, перекрывает глобальный `dhcp-range`).

### Duplicate-проверка

- **MAC-дубли** проверяются всегда (включая MAC-only хосты).
- **IP-дубли** проверяются только если IP указан. Если у существующего
  хоста IP пустой, а у нового — указан, это не конфликт.

---

## Внутренняя реализация (кратко)

- `validateHostFields(mac, ip, hostname string) bool` в `dnsmasq.go` —
  единый хелпер валидации. Используется в `addHostHandler`,
  `bulkAddHostsHandler`, `parseCSVHosts`.
- `formatDhcpHostLine(h HostEntry) string` — порядок `mac[, hostname][, ip][, tags...][, lease-time]`;
  пустые поля пропускаются, lease-time (если задан) дописывается последним.
  Round-trip parse→format сохраняет все поля, включая lease-time.
- `addHostHandler` — `findHostsByIP(req.Ip, req.Mac)` вызывается только
  если `req.Ip != ""`. `findHostsByMac` — всегда. `lease_time` (если задан)
  валидируется через `IsLeaseTime` → иначе 400 `invalid_lease_time`.
- `bulkAddHostsHandler` — внутри-batch cross-check IP пропускается если
  IP пустой. Раньше невалидные строки silently skip'ались через
  `continue`; теперь — 400 с указанием проблемного MAC.
