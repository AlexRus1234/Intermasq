# API, autentifikacija i kontrola pristupa

Intermasq daje REST API na Gin. Osnovny put jest `/api`; Swagger UI jest na
`http://<host>:<port>/swagger/index.html`.

## Autentifikacija

Za zaštičene endpointy podržany sut `Authorization: Bearer <JWT>` za brauzer i
`X-API-Key: <INTERMASQ_SECRET>` za skripty, plugini i Prometheus. JWT koristi
HS256, živi 72 časa i soderži `sub`, `exp`, `jti`, `ver` i `role`.

## RBAC

Prvy użytkateľ sozdany črez `/api/setup` jest `admin`; pozdnejši sozdany črez
`/api/users` sut `user`. Auth-uroven dopušča čtenje, dodavanje hostov/DNS,
šablony, istoriju, backup-download, audit i CSV. Admin dodatkovo može reload,
rollback, restore, raw-zapis, upravjenje użytkateľami i restart.

## Endpointy

Najvažnějši endpointy: `/api/status`, `/api/setup`, `/api/login`, `/api/hosts`,
`/api/aliases`, `/api/leases`, `/api/arp`, `/api/config`, `/api/files/:name`,
`/api/templates`, `/api/history`, `/api/backup`, `/api/reload`, `/api/users`,
`/api/events`, `/api/audit`, `/api/plugins` i `/api/restart-self`.

`/metrics` prihaja s Prometheus metriki, `/plugins/<id>/*` jest autentifikovany
reverse-proxy, a `/swagger/*any` jest dostupan bez autentifikacije.
