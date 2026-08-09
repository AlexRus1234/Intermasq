[**Interslavjanski (lat.)**] | [**Кирилица**](README.isv-cyrl.md)

<div align="center">

<h1>Intermasq</h1>

**Veb-panel dlja upravjenja dnsmasq**

Intermasq jest samostojna veb-prikladka dlja administracije `dnsmasq`. Frontend,
serverna logika i API sut spojeny v jedin izvršajemy fajl. Danny se hranet v
fajlovoj sisteme; vnêšnja baza dannyh i kontejnerna infrastruktura ne sut
potrebny.

[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg?style=flat-square)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg?style=flat-square)](https://vuejs.org/)
[![Bootstrap](https://img.shields.io/badge/Bootstrap-5-7952B3.svg?style=flat-square)](https://getbootstrap.com/)

</div>

---

## Sadržaj

- [Snimky](#snimky)
- [Vlastnosti](#vlastnosti)
- [Brz start](#brz-start)
- [Konfiguracija](#konfiguracija)
- [Pristup](#pristup)
- [API, plugini i metriki](#api-plugini-i-metriki)
- [Struktura projekta](#struktura-proekta)
- [Tehnologičny stek](#tehnologičny-stek)
- [Licencija](#licencija)


## Vlastnosti

### DHCP i DNS
- Operacije s `dhcp-host=` i validacija MAC/IP/hostname, tagov `set:` i `lease-time`.
- Predloženje slêdujučego svobodnogo IP iz `dhcp-range`.
- Šablony hostov, DNS-zapisy `A` / `CNAME` / `PTR` / `TXT`, CSV import/export.
- Pregled arend, ARP-ustrojstv i masovy prenos arendy v statičny zapis.
- Otkryvanje neznanyh ARP-ustrojstv s identifikacijeju proizvoditelja (OUI).

### Konfiguracija i bezopasnost
- Vizualny redaktor `dhcp-range`, `dhcp-option`, `server=`, PXE i mrežnoj zagruzky.
- Raw-redaktor `.conf` s proverkoju `dnsmasq --test`.
- Istorija verzij, diff, vosstanovjenje, ZIP-backup i audit-log.
- Zaštita ot path traversal: zapis jest možny jedino vnutri `-conf-dir`.

### Eksploatacija
- Odin binar (`go:embed`) s podporoju systemd, systemd-user, OpenRC, runit i sysvinit.
- SSE-obnovjenja ARP i statusa dnsmasq v realnom vremene.
- JWT dlja brauzera, `X-API-Key` dlja skriptov i pluginov, RBAC `admin` / `user`.
- Unix-socket plugini, `/metrics` dlja Prometheus, Swagger, temna i svetla tema.

## Brz start

### Potrebnosti

| Komponent | Verzija | Namêrenje |
|---|---|---|
| **Go** | 1.25+ | Skladanje binara |
| **Node.js** | 22+ | Skladanje frontenda |
| **dnsmasq** | vsaka | Na ciljevoj mašine |

```bash
make build
export INTERMASQ_SECRET="$(openssl rand -hex 32)"
sudo ./intermasq -port 8081 \
  -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

Pri prvom zapusku se pokaže forma za sozdanje administratorskogo računa.
Primer systemd-junitu i zapusk od posebnoj učetnoj zapisi sut v
[`docs/func/isv/os-setup.md`](docs/func/isv/os-setup.md).

## Konfiguracija

Glavne opcije: `-port` (8081), `-conf-dir` (`/etc/dnsmasq.d`), `-leases`
(`/var/lib/misc/dnsmasq.leases`), `-arp-file`, `-db`, `-audit-log`, `-templates`,
`-history-dir`, `-history-depth`, `-init-system` i `-ci-mode`. Obavezna
promenna sredy jest `INTERMASQ_SECRET`; ona podpisuje JWT i služi jako
`X-API-Key`.

## Pristup

Pri zapusku od `root` systemd i `dnsmasq --test` se vyzyvajut neposredno. Pri
zapusku od obyčnogo użytkateľa upravjenje servisa idet črez `sudo -n`; potrebno
razrešiti samo konkretne komandy i prava na `conf-dir` i fajl arend.

## API, plugini i metriki

Swagger UI: `http://<host>:<port>/swagger/index.html`.
Podrobnosti: [`api.md`](docs/func/isv/api.md), [`plugins.md`](docs/func/isv/plugins.md)
i [`metrics.md`](docs/func/isv/metrics.md).

## Struktura proekta

`main.go` jest točka vhoda; `internal/` soderži modele, validatore, DNS/DHCP
logiku, autentifikaciju, metriki, plugini i HTTP API. `frontend/` jest Vue 3 SPA,
`docs/` soderži dokumentaciju, a `tests/` testove i smoke-suity.

## Tehnologičny stek

Backend: Go 1.25, Gin, JWT, bcrypt, Swagger i `go:embed`.
Frontend: Vue 3, Vite 7, Bootstrap 5, vue-i18n, Axios i SSE.

## Licencija

Projekt se rasprostranja pod **[GNU Affero General Public License v3.0](LICENSE)**.

Intermasq - Web panel for dnsmasq
Copyright (C) 2026  AlexRus1234
