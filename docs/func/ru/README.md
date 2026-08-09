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

# Документация Intermasq

Каталог содержит функциональную документацию Intermasq. Корневой `README.md`
предоставляет обзор системы и инструкцию по первоначальному запуску; сведения,
требующие детального описания, приведены в настоящем каталоге.

| Файл | О чём |
|---|---|
| [os-setup.md](os-setup.md) | Развёртывание в Linux, права доступа, sudo, init-системы, systemd-юнит и каталоги. |
| [api.md](api.md) | Эндпоинты, аутентификация (JWT и X-API-Key), разграничение прав (RBAC). |
| [features.md](features.md) | Функции DHCP/DNS, редактирование конфигурации, история, массовые операции и безопасность. |
| [plugins.md](plugins.md) | Система плагинов, манифест, окружение, reverse proxy и жизненный цикл. |
| [metrics.md](metrics.md) | Эндпоинт `/metrics`, проверка DNS-доступности и примеры алертов Prometheus. |

Исторические документы по версиям находятся в [`docs/`](../../), в том числе
`v1.0-features.md`, `v3.1-features.md`, `version-history.md` и
`new-features.md`. Настоящий каталог содержит актуальную пользовательскую
документацию.
