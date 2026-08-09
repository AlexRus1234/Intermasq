# Intermasq Lab Distribution

This directory is the build context for the disposable Intermasq laboratory.
The ISO/image builder is intentionally not included yet.

## Layout

- `manifest.yaml` describes the VM and laboratory topology.
- `rootfs/etc/intermasq-lab/lab.conf` contains runtime paths and instance data.
- `rootfs/etc/intermasq-lab/dnsmasq/` contains the seed dnsmasq configurations.
- `rootfs/etc/intermasq-lab/devices/` contains static mock devices and DNS names.
- `rootfs/etc/init.d/intermasq-lab` creates namespaces and starts the lab.

The runtime expects Alpine Linux with `iproute2`, `dnsmasq`, `socat`, `openssl`,
and the Intermasq binary installed at `/usr/local/lib/intermasq`.

The service is designed for a disposable VM. It generates per-instance secrets
on first start and keeps all mutable state below `/var/lib/intermasq-lab`.

## Planned build flow

```text
manifest.yaml + rootfs/ + Intermasq binary
    -> future image builder
    -> ISO / qcow2 / OVA
```

The current files can be installed manually into the prototype VM before the
image builder is implemented.
