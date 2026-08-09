# Plugin System

Intermasq is extended through sidecar plugins that communicate with the panel
over Unix sockets. Each plugin is a separate process with an HTTP server
available through its socket; the panel proxies requests to it.

---

## Directory and manifest

Plugins are searched for in `/etc/intermasq/plugins/` (see
`internal/plugins`). Each plugin is a subdirectory with `manifest.json` and a
binary:

```
/etc/intermasq/plugins/
└── my-plugin/
    ├── manifest.json
    └── plugin-binary     # path in manifest.bin, relative to the directory
```

The `manifest.json` structure is:

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "bin": "./plugin-binary"
}
```

- `id`: unique identifier used in `/plugins/<id>/*` URLs and the socket name. Duplicate IDs are ignored because duplicate Gin routes cause an error.
- `name`: display name.
- `bin`: executable path relative to the plugin directory.

## Lifecycle

At panel startup (`plugins.Load`):

1. `/run/intermasq/sockets/` is created.
2. `manifest.json` is read from every subdirectory. Malformed manifests and missing directories are silently skipped.
3. The binary is started (`cmd.Start()`) with two environment variables:
   - `INTERMASQ_KEY`: the panel secret (`INTERMASQ_SECRET`) for `X-API-Key` API requests;
   - `PLUGIN_SOCKET`: the Unix socket the plugin must listen on (`/run/intermasq/sockets/<id>.sock`).
4. A reverse proxy `r.Any("/plugins/<id>/*any", auth, ...)` is mounted and directs requests to the socket.
5. The manifest is added to `loadedPlugins` and returned by `GET /api/plugins`.

When the panel stops:

- `SIGTERM`/`SIGINT` and `POST /api/restart-self` call `plugins.Stop()`, which
  kills all running plugin processes. This is important for openrc/runit/sysvinit,
  where the supervisor kills only the main PID and would otherwise leave
  orphaned children after a restart.

## Proxying

All `/plugins/<id>/*` requests pass through `auth.Middleware`, with the same
authentication as the rest of the API (JWT or `X-API-Key`). The plugin does not
perform authentication; the panel has already checked the token.

```http
GET  /plugins/my-plugin/            # proxied as GET / to the plugin socket
POST /plugins/my-plugin/api/do-thing
```

The plugin is an ordinary HTTP server listening on the Unix socket from
`PLUGIN_SOCKET`.

## User interface

Loaded plugins (`GET /api/plugins`) are shown in the menu; clicking one opens a
full-screen overlay with `<iframe src="/plugins/<id>/">`.

## Adding and updating a plugin

Plugins are not reloaded automatically. After changing a manifest or binary,
restart the panel using one of:

- UI: `Menu -> Restart Intermasq` (admin);
- API: `POST /api/restart-self` (admin);
- supervisor: `systemctl restart intermasq`.

## Minimal Python plugin example

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

When started with `INTERMASQ_SECRET`, the panel passes `INTERMASQ_KEY` and
`PLUGIN_SOCKET` to the plugin.
