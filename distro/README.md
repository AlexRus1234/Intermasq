# Intermasq Lab Distribution

This directory is the build context for the disposable Intermasq laboratory.
The ISO/image builder is intentionally not included yet.

## Layout

- `manifest.yaml` describes the VM and laboratory topology.
- `MANUAL.md` is the step-by-step GUI coverage walkthrough.
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

Each network has two static mock devices. The first device is active and should
show the green online indicator; the second intentionally remains offline.

## Planned build flow

```text
manifest.yaml + rootfs/ + Intermasq binary
    -> future image builder
    -> ISO / qcow2 / OVA
```

The current files can be installed manually into the prototype VM before the
image builder is implemented.
