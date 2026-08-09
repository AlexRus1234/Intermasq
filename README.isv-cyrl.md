[ISV латиница](README.isv.md) | [Русский](README.md) | [English](README.en.md) | **ISV кирилица**

<div align="center">

<h1>Intermasq</h1>

**Веб-панел дља управјенја dnsmasq**

Intermasq јест самостојна веб-прикладка дља администрације `dnsmasq`. Frontend,
серверна логика и API сут спојени в једин извршајеми фајл. Дани се хранет в
фајловој системе; внєшња база даних и контејнерна инфраструктура не сут
потребни.

[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg?style=flat-square)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D.svg?style=flat-square)](https://vuejs.org/)
[![Bootstrap](https://img.shields.io/badge/Bootstrap-5-7952B3.svg?style=flat-square)](https://getbootstrap.com/)

</div>

---

## Содржај

- [Снимки](#снимки)
- [Властивости](#властивости)
- [Брз старт](#брз-старт)
- [Конфигурација](#конфигурација)
- [Приступ](#приступ)
- [API, плугини и метрики](#api-плуғини-и-метрики)

> Подробна документација јест в [`docs/func/isv-cyrl/`](docs/func/isv-cyrl/README.md).

## Властивости

### DHCP и DNS
- Операције с `dhcp-host=` и валидација MAC/IP/hostname, тагов `set:` и `lease-time`.
- Предложенје слєдујучего свободного IP из `dhcp-range`.
- Шаблони хостов, DNS-записи `A` / `CNAME` / `PTR` / `TXT`, CSV импорт/експорт.
- Преглед аренд, ARP-устројств и масови пренос аренди в статични запис.

### Конфигурација и безопастност
- Визуални редактор `dhcp-range`, `dhcp-option`, `server=`, PXE и мрежној загрузки.
- Raw-редактор `.conf` с проверкоју `dnsmasq --test`.
- Историја верзиј, diff, восстановјенје, ZIP-backup и audit-log.
- Заштита от path traversal: запис јест можни јединок в `-conf-dir`.

### Експлоатација
- Једин бинар с поддержкоју systemd, systemd-user, OpenRC, runit и sysvinit.
- SSE-обновјенја ARP и статуса dnsmasq в реалном времене.
- JWT дља браузера, `X-API-Key` дља скриптов и плугинов, RBAC `admin` / `user`.

## Брз старт

```bash
make build
export INTERMASQ_SECRET="$(openssl rand -hex 32)"
sudo ./intermasq -port 8081 -conf-dir /etc/dnsmasq.d \
  -leases /var/lib/misc/dnsmasq.leases
```

При првом запуску се покаже форма за создание административного аккаунта.
Пример systemd-юнита јест в [`docs/func/isv-cyrl/os-setup.md`](docs/func/isv-cyrl/os-setup.md).

## Конфигурација

Главне опције сут `-port`, `-conf-dir`, `-leases`, `-arp-file`, `-db`,
`-audit-log`, `-templates`, `-history-dir`, `-history-depth`, `-init-system` и
`-ci-mode`. Обавезна промена средине јест `INTERMASQ_SECRET`; она подписује
JWT и служи како `X-API-Key`.

## API, плугини и метрики

Swagger UI: `http://<host>:<port>/swagger/index.html`.
Подробности: [`api.md`](docs/func/isv-cyrl/api.md), [`plugins.md`](docs/func/isv-cyrl/plugins.md)
и [`metrics.md`](docs/func/isv-cyrl/metrics.md).

## Лиценција

Пројект се распростира под **[GNU Affero General Public License v3.0](LICENSE)**.

Intermasq - Web panel for dnsmasq
Copyright (C) 2026  AlexRus1234
