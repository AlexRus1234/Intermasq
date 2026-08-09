# API, аутентификација и контрола приступа

Intermasq даје REST API на Gin. Основни пут јест `/api`; Swagger UI јест на
`http://<host>:<port>/swagger/index.html`.

## Аутентификација

За заштићене endpoint-и подржани сут `Authorization: Bearer <JWT>` дља браузера
и `X-API-Key: <INTERMASQ_SECRET>` дља скриптов, плугинов и Prometheus. JWT користи
HS256, живе 72 часа и содржи `sub`, `exp`, `jti`, `ver` и `role`.

## RBAC

Први уживатељ создан чез `/api/setup` јест `admin`; познєји чез `/api/users` сут
`user`. Auth-уровен допускаје чтение, добавјенје хостов/DNS, шаблони, историју,
backup-download, audit и CSV. Admin додатно може reload, rollback, restore,
raw-запис, управјенје уживатељами и restart.

## Endpoint-и

Главни endpoint-и сут `/api/status`, `/api/setup`, `/api/login`, `/api/hosts`,
`/api/aliases`, `/api/leases`, `/api/arp`, `/api/config`, `/api/files/:name`,
`/api/templates`, `/api/history`, `/api/backup`, `/api/reload`, `/api/users`,
`/api/events`, `/api/audit`, `/api/plugins` и `/api/restart-self`.

`/metrics` даје Prometheus метрики, `/plugins/<id>/*` јест аутентификовани
reverse-proxy, а `/swagger/*any` доступни јест без аутентификације.
