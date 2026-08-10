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

**Русский** | [English](README.en.md) |

# Дистрибутив лаборатории Intermasq

Этот каталог — контекст сборки одноразовой учебной ISO-лаборатории Intermasq.

## Структура

- `manifest.yaml` — топология VM и лаборатории.
- `MANUAL.md` — пошаговое руководство для проверки GUI.
- `build.sh` — команда сборки для Linux/macOS.
- `build.ps1` — команда сборки для Windows.
- `Containerfile` — воспроизводимая среда сборки.
- `build-inside.sh` — создание initramfs и загрузочного ISO.
- `build.env` — фиксация версий Alpine и бинарника Intermasq.
- `rootfs/etc/intermasq-lab/lab.conf` — пути и параметры экземпляров.
- `rootfs/etc/intermasq-lab/seed/` — конфиги dnsmasq для каждого профиля.
- `rootfs/etc/intermasq-lab/dnsmasq/` — базовые конфигурации dnsmasq.
- `rootfs/etc/intermasq-lab/devices/` — статические mock-устройства и DNS.
- `rootfs/etc/intermasq-lab/templates/` — шаблоны хостов.
- `rootfs/etc/intermasq/plugins/` — демонстрационный плагин (Unix-сокет).
- `rootfs/etc/init.d/intermasq-lab` — создание неймспейсов и запуск лаборатории.
- `rootfs/usr/local/sbin/intermasq-lab-heartbeat` — удержание mock-устройств
  в ARP-таблице.

Среда выполнения: Alpine Linux с `iproute2`, `dnsmasq`, `socat`, `openssl`
и бинарником Intermasq в `/usr/local/lib/intermasq`.

Сервис рассчитан на одноразовую VM. При первом старте генерируются секреты
для каждого экземпляра; всё изменяемое состояние хранится в
`/var/lib/intermasq-lab`. Учётная запись `admin` создаётся автоматически.
Учётные данные сохраняются в
`/var/lib/intermasq-lab/<instance>/data/credentials.txt` с правами `0600`.
Лаборатория использует публичные тестовые учётные данные:
`admin` / `intermasq-lab`.

В каждой сети два mock-устройства: первое активно (зелёная лампочка online),
второе намеренно offline.

## Сборка ISO

Требуется Podman `6.0.2` или новее. Go, Node.js, Packer, Docker, Podman
Compose и локальный Alpine SDK не нужны.

- **Linux**: любой запущенный движок Podman (system или rootless).
- **macOS**: Podman Machine с провайдером по умолчанию.
- **Windows**: требуется WSL2; скрипт использует Podman-машину по умолчанию
  и автоматически создаёт WSL-машину при отсутствии. Hyper-V не нужен.

Linux / macOS:

```sh
./distro/build.sh
```

Windows PowerShell:

```powershell
.\distro\build.ps1
```

Результат записывается в `distro/output/` (ISO ≈ 88 МБ). Сборка скачивает
проверенный release-бинарник Intermasq, проверяет SHA-256, создаёт Alpine
rootfs, упаковывает в initramfs и формирует загрузочный ISO через Syslinux
и xorriso.

## Запуск ISO

Загрузите ISO в любом гипервизоре. Сетевой адаптер VM должен быть
паравиртуализированным:

| Гипервизор | Тип адаптера |
|---|---|
| Proxmox / KVM / QEMU | **VirtIO (paravirtualized)** — по умолчанию |
| VMware | **VMXNET3** |
| VirtualBox 6+ | **Paravirtualized Network (virtio-net)** |
| Hyper-V | **Default Network Adapter (synthetic)** |

Ядро не содержит драйверов e1000/pcnet32/rtl8139.

После загрузки VM автоматически определяет сетевой интерфейс, получает адрес
по DHCP и выводит на консоль имя интерфейса, IP-адрес и URL всех трёх панелей.

## Доступ

| Профиль | Порт | Сеть | Назначение |
|---|---:|---|---|
| office | `8082` | `10.10.1.0/24` | DHCP, статика и DNS |
| lab | `8083` | `10.10.2.0/24` | редактор конфигурации и восстановление |
| demo | `8084` | `10.10.3.0/24` | discovery, SSE, плагин и метрики |

Вход в панели (все экземпляры):

```text
admin / intermasq-lab
operator / operator-lab   (RBAC-пользователь, ограниченные права)
```

SSH (консоль VM):

```text
root / intermasq-lab
```

## Сброс лаборатории

Обычный restart сохраняет данные:

```sh
rc-service intermasq-lab restart
```

Для полного возврата к исходному состоянию: остановить сервис, удалить
`/var/lib/intermasq-lab`, запустить заново. Это удаляет users, audit,
history, templates и изменения конфигурации, но не трогает ISO.
