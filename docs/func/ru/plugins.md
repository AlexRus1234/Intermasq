# Плагины

Intermasq расширяется sidecar-плагинами, которые общаются с панелью через
**Unix-сокеты**. Каждый плагин — отдельный процесс со своим HTTP-сервером на
сокете; панель проксирует к нему запросы.

---

## Каталог и manifest

Плагины ищутся в `/etc/intermasq/plugins/` (см. `internal/plugins`). Каждый
плагин — отдельный подкаталог с `manifest.json` и бинарником:

```
/etc/intermasq/plugins/
└── my-plugin/
    ├── manifest.json
    └── plugin-binary     # путь указан в manifest.bin, относительно каталога
```

`manifest.json`:

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "bin": "./plugin-binary"
}
```

- `id` — уникален; используется в URL `/plugins/<id>/*` и в имени сокета.
  Дубликаты `id` пропускаются (плюрализация роутов в gin паникует).
- `name` — отображается в UI.
- `bin` — путь к бинарнику относительно каталога плагина.

## Жизненный цикл

При старте панели (`plugins.Load`):

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

При остановке панели:

- `SIGTERM`/`SIGINT` и `POST /api/restart-self` вызывают `plugins.Stop()`,
  который `Kill`'ит все запущенные процессы-плагины. Это критично для
  openrc/runit/sysvinit, где супервизор убивает только главный PID, оставляя
  children осиротевшими (после restart-self они дублировались бы).

## Проксирование

Все запросы `/plugins/<id>/*` идут под `auth.Middleware` — аутентификация такая
же, как у остального API (JWT или `X-API-Key`). Сам плагин аутентификацией не
занимается — панель уже проверила токен до проксирования.

```http
GET  /plugins/my-plugin/            # прокинется как GET / на сокет плагина
POST /plugins/my-plugin/api/do-thing
```

Плагин отвечает как обычный HTTP-сервер,listening на Unix-сокете из
`PLUGIN_SOCKET`.

## UI

Загруженные плагины (`GET /api/plugins`) отображаются в меню; клик открывает
полноэкранный overlay с `<iframe src="/plugins/<id>/">`.

## Добавление/обновление плагина

На лету плагины не перезагружаются. После изменения manifest/бинарника —
перезапустите панель:

- через UI: `Меню → 🔄 Рестарт Intermasq` (admin);
- через API: `POST /api/restart-self` (admin);
- через супервизор: `systemctl restart intermasq`.

## Минимальный пример плагина (Python)

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
print(f"[plugin] listening on {sock}")
httpd.serve_forever()
```

Запускайте с `INTERMASQ_SECRET` в среде — панель сама передаст `INTERMASQ_KEY`
и `PLUGIN_SOCKET`.
