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

[Русский](README.md) | **English** |

# Intermasq Lab Distribution

This directory is the build context for the disposable Intermasq laboratory ISO.

## Layout

- `manifest.yaml` describes the VM and laboratory topology.
- `MANUAL.en.md` is the step-by-step GUI coverage walkthrough.
- `build.sh` is the one-command Linux/macOS entrypoint.
- `build.ps1` is the one-command Windows entrypoint.
- `Containerfile` defines the reproducible open-tool build environment.
- `build-inside.sh` creates the Alpine initramfs and bootable ISO.
- `build.env` pins the Alpine release and Intermasq release artifact.
- `rootfs/etc/intermasq-lab/lab.conf` contains runtime paths and instance data.
- `rootfs/etc/intermasq-lab/seed/` contains per-profile managed config files.
- `rootfs/etc/intermasq-lab/dnsmasq/` contains the seed dnsmasq configurations.
- `rootfs/etc/intermasq-lab/devices/` contains static mock devices and DNS names.
- `rootfs/etc/intermasq-lab/templates/` contains host-template examples.
- `rootfs/etc/intermasq/plugins/` contains the Unix-socket demo plugin.
- `rootfs/etc/init.d/intermasq-lab` creates namespaces and starts the lab.
- `rootfs/usr/local/sbin/intermasq-lab-heartbeat` keeps selected mock devices
  visible in the ARP table as online.

The runtime expects Alpine Linux with `iproute2`, `dnsmasq`, `socat`, `openssl`,
and the Intermasq binary installed at `/usr/local/lib/intermasq`.

The service is designed for a disposable VM. It generates per-instance secrets
on first start and keeps all mutable state below `/var/lib/intermasq-lab`.
It also creates the first `admin` user automatically. Generated credentials are
stored in `/var/lib/intermasq-lab/<instance>/data/credentials.txt` with mode
`0600`. This disposable lab intentionally uses public test credentials:
`admin` / `intermasq-lab`.

Each network has four static mock devices: the first is active and should show
the green online indicator; the other three intentionally remain offline. An
unknown-ARP entry and a fresh DHCP lease are also generated for the Discovery
tab.

## Build ISO

Requirements: Podman `6.0.2` or newer. No Go, Node.js, Packer, Docker, Podman
Compose, or local Alpine SDK is required.

- **Linux**: any running Podman engine (system or rootless).
- **macOS**: Podman Machine with the default provider.
- **Windows**: WSL2 must be installed; the script uses the default Podman
  machine and creates a WSL-backed one automatically when none exists.
  Hyper-V is not required.

Linux / macOS:

```sh
./distro/build.sh
```

Windows PowerShell:

```powershell
.\distro\build.ps1
```

The result is written to `distro/output/` (ISO is approximately 88 MB).
The build downloads the pinned published Intermasq binary, verifies its
SHA-256, creates an Alpine rootfs, embeds it in an initramfs, and packages
it with Syslinux and xorriso.

## Running the ISO

Boot the ISO in any hypervisor. The VM network adapter must be set to a
paravirtualized type:

| Hypervisor | Adapter type |
|---|---|
| Proxmox / KVM / QEMU | **VirtIO (paravirtualized)** — default |
| VMware | **VMXNET3** |
| VirtualBox 6+ | **Paravirtualized Network (virtio-net)** |
| Hyper-V | **Default Network Adapter (synthetic)** |

The kernel does not include e1000/pcnet32/rtl8139 drivers.

After boot, the VM detects the first network interface, obtains an address
through DHCP, and prints that interface, the address, and all three panel
URLs on the console.

## Access

| Profile | Panel | Network | Purpose |
|---|---:|---|---|
| office | `8082` | `10.10.1.0/24` | DHCP, static hosts and DNS |
| lab | `8083` | `10.10.2.0/24` | config editor and restore |
| demo | `8084` | `10.10.3.0/24` | discovery, SSE, plugin and metrics |

Panel login (all instances):

```text
admin / intermasq-lab
operator / operator-lab   (RBAC user, restricted)
```

SSH (VM console):

```text
root / intermasq-lab
```

## Resetting the lab

A normal restart preserves all data:

```sh
rc-service intermasq-lab restart
```

For a full reset to demo state, stop the service, delete
`/var/lib/intermasq-lab`, then start it again. This removes users, audit,
history, templates and configuration changes, but does not modify the ISO.
