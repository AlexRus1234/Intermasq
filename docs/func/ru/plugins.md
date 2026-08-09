# Система плагинов

Расширение Intermasq выполняется посредством sidecar-плагинов, взаимодействующих
с панелью через Unix-сокеты. Каждый плагин является отдельным процессом с
HTTP-сервером, доступным через соответствующий сокет; панель выполняет
проксирование запросов.

---

## Каталог и манифест

Плагины ищутся в `/etc/intermasq/plugins/` (см. `internal/plugins`). Каждый
плагин — отдельный подкаталог с `manifest.json` и бинарником:

```
/etc/intermasq/plugins/
└── my-plugin/
    ├── manifest.json
    └── plugin-binary     # путь указан в manifest.bin, относительно каталога
```

Файл `manifest.json` имеет следующую структуру:

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "bin": "./plugin-binary"
}
```

- `id` — уникальный идентификатор, используемый в URL `/plugins/<id>/*` и имени
  сокета. Дубликаты `id` игнорируются, поскольку регистрация дублирующихся
  маршрутов в Gin приводит к ошибке.
- `name` — отображаемое имя.
- `bin` — путь к исполняемому файлу относительно каталога плагина.

## Жизненный цикл

При запуске панели (`plugins.Load`) выполняются следующие операции:

1. Создаётся `/run/intermasq/sockets/`.
2. Для каждого подкаталога читается `manifest.json`. Malformed-манифесты и
   отсутствующие каталоги молча пропускаются.
3. Бинарник запускается (`cmd.Start()`) с двумя переменными окружения:
   - `INTERMASQ_KEY` — секрет панели (`INTERMASQ_SECRET`), для `X-API-Key`
     запросов к API;
   - `PLUGIN_SOCKET` — путь Unix-сокета, который плагин должен слушать
     (`/run/intermasq/sockets/<id>.sock`).
4. На роутер монтируется reverse-proxy `r.Any("/plugins/<id>/*any", auth, ...)`,
   который направляет запросы на этот сокет.
5. Манифест попадает в `loadedPlugins` → отдаётся через `GET /api/plugins`.

При остановке панели выполняется следующее:

- `SIGTERM`/`SIGINT` и `POST /api/restart-self` вызывают `plugins.Stop()`,
  который `Kill`'ит все запущенные процессы-плагины. Это критично для
  openrc/runit/sysvinit, где супервизор убивает только главный PID, оставляя
  children осиротевшими (после restart-self они дублировались бы).

## Проксирование

Все запросы `/plugins/<id>/*` проходят через `auth.Middleware`; аутентификация
же, как у остального API (JWT или `X-API-Key`). Сам плагин аутентификацией не
занимается — панель уже проверила токен до проксирования.

```http
GET  /plugins/my-plugin/            # прокинется как GET / на сокет плагина
POST /plugins/my-plugin/api/do-thing
```

Плагин работает как обычный HTTP-сервер и прослушивает Unix-сокет из
`PLUGIN_SOCKET`.

## Пользовательский интерфейс

Загруженные плагины (`GET /api/plugins`) отображаются в меню; клик открывает
полноэкранный overlay с `<iframe src="/plugins/<id>/">`.

## Добавление и обновление плагина

Автоматическая перезагрузка плагинов не выполняется. После изменения манифеста
или исполняемого файла необходимо перезапустить панель одним из способов:

- через интерфейс: `Меню → Рестарт Intermasq` (admin);
- через API: `POST /api/restart-self` (admin);
- через супервизор: `systemctl restart intermasq`.

## Минимальный пример плагина на Python

```python
import os, socketserver, http.server

class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.end_headers()
        self.wfile.write(b"<h1>Hello from plugin</h1>")
    def log_message(self, *a): pass

sock = os.environ["PLUGIN_SOCKET"]
os.makedirs(os.path.dirname(sock), exist_ok=True)
try: os.unlink(sock)
except FileNotFoundError: pass

httpd = socketserver.UnixStreamServer(sock, H)
os.chmod(sock, 0o660)
print(f"[plugin] socket: {sock}")
httpd.serve_forever()
```

При запуске с переменной `INTERMASQ_SECRET` панель передаёт плагину значения
`INTERMASQ_KEY` и `PLUGIN_SOCKET`.
