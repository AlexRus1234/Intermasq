**[Русский](MANUAL.md)** | **English** |

# Intermasq Lab: manual walkthrough

The laboratory is designed for a quick GUI walkthrough. All three panels are
independent but share the same set of demo scenarios.

## Access

After ISO boot, the network interface is detected automatically; the DHCP
address and all three panel URLs are printed on the VM console. If no address
appears, verify that the VM network adapter is connected and DHCP is enabled;
check current interfaces with `ip addr`.

| Profile | Panel | Network | Purpose |
|---|---:|---|---|
| office | `8082` | `10.10.1.0/24` | DHCP, static hosts and DNS |
| lab | `8083` | `10.10.2.0/24` | config editor and restore |
| demo | `8084` | `10.10.3.0/24` | discovery, SSE, plugin and metrics |

Common test account:

```text
admin / intermasq-lab
```

An RBAC user is also created on first boot:

```text
operator / operator-lab
```

## 1. Common shell

On each panel verify:

1. Log in as `admin`.
2. Confirm `dnsmasq` status is green.
3. Switch Russian/English language.
4. Switch light/dark theme.
5. Open Swagger from the menu.
6. Open `/metrics` in a new tab.
7. Log out and log back in.

Keep the `demo` panel open for SSE checks in later steps.

## 2. Static hosts: office profile

The "Static" tab should contain several hosts with MAC, IP, hostname,
tags and lease-time. `office-laptop` should show a green online indicator;
`office-printer` should remain offline.

Verify:

1. Search by MAC, IP and hostname.
2. Sort by each column.
3. Edit an existing host.
4. Add a new host with `set:iot` and `lease-time=12h`.
5. Next-free-IP hint.
6. CSV export.
7. CSV import into a separate config file.
8. Bulk-edit IP prefix and hostname suffix.
9. Bulk-move to a different `.conf`.
10. Bulk-delete one test host.
11. Open history of a modified file.
12. View diff and restore a version.
13. One-step rollback via `.bak`.

In the templates modal, check `office-laptop` and `office-iot`, then apply
one template to a test MAC.

## 3. DNS: office and demo profiles

The "DNS" tab contains all four record types:

- `A`: `laptop.office.test`, `printer.office.test`;
- `CNAME`: `printer-alias.office.test`;
- `PTR`: reverse record for `10.10.1.10`;
- `TXT`: `lab-info.office.test`.

Verify:

1. Filtering and sorting.
2. Directive preview in the form.
3. Add and delete a test record.
4. Bulk import of several DNS records.
5. CSV export/import.
6. History and diff of the DNS file.

## 4. Discovery: demo profile

The "Discovery" tab contains two independent groups:

1. Unknown ARP devices: the heartbeat device should be in statics and not
   appear as unknown.
2. New DHCP leases: a lease from a device not in `dhcp-host` should appear
   in the lower table.

Verify:

1. ARP online/offline indicators.
2. OUI/vendor lookup.
3. Transition an unknown ARP device into the static form.
4. Select one lease and convert to static.
5. Select multiple leases and bulk-convert.
6. Yellow warning about a separate Apply step.
7. Press Apply after bulk lease-to-static.

For SSE: keep the tab open and restart the heartbeat or `dnsmasq` — ARP/status
changes should arrive without manually refreshing the page.

## 5. Configuration: lab profile

The "dnsmasq settings" tab should show several files:

- `lab-main.conf` — DHCP range, domain and base directives;
- `lab-devices.conf` — hosts and DNS records;
- `lab-options.conf` — RFC 2132 DHCP options;
- `lab-pxe.conf` — TFTP/PXE;
- `lab-forwarder.conf` — `server=` and domain forwarding.

Verify:

1. Switch files via tabs.
2. Visual editing of `dhcp-range`.
3. RFC 2132 presets for DHCP options.
4. Add `server=/domain/upstream`.
5. PXE/network boot directives.
6. Create a new file via each preset:
   `empty`, `basic-dhcp`, `forwarder`, `pxe`, `aliases`.
7. Delete a test file.
8. Raw read of a file.
9. Raw write of a valid directive.
10. Raw write of an invalid directive and automatic rollback.
11. History, diff, restore and `.bak` rollback.

## 6. Backup, audit and templates

In the "Safety" tab:

1. Open the host templates list.
2. Create and delete a test template.
3. Download a ZIP backup.
4. Modify a test `.conf`.
5. Restore the ZIP.
6. Verify that a `.restore.bak` is created before restore.
7. Open the audit log.

Audit should contain actions for hosts, DNS, templates, history, backup and
restore.

## 7. RBAC: any profile

1. Log in as `operator / operator-lab`.
2. Verify the Users tab is absent.
3. Verify Apply, Restore, Raw write and restart are hidden.
4. Open available read/add operations.
5. Try calling an admin endpoint via Swagger: should be denied.
6. Log in as `admin`.
7. Create a temporary user.
8. Delete the temporary user.
9. Change the admin password and verify old JWT revocation.

## 8. Plugin and operations: demo profile

The menu should show `Intermasq Lab Plugin`.

1. Open the plugin in fullscreen overlay.
2. Verify it responds via Unix socket.
3. Restart Intermasq from the menu.
4. Reopen the plugin and verify the sidecar is not duplicated.
5. Check `/metrics` before and after a lease/ARP change.
6. Check `/swagger/index.html`.

## 9. Negative tests

In the `lab` profile, safely perform on temporary files:

1. Invalid MAC.
2. Invalid IP.
3. Duplicate MAC.
4. Filename without `.conf`.
5. Filename containing `/` or `\\`.
6. Path outside `conf-dir`.
7. Invalid raw dnsmasq syntax.
8. Corrupted ZIP.
9. Empty bulk import.
10. More than 10 failed logins from one IP.

## 10. Resetting the lab

A normal restart preserves data:

```sh
rc-service intermasq-lab restart
```

For a full reset to demo state, stop the service, delete only
`/var/lib/intermasq-lab`, then start it again. This removes users, audit,
history, templates and configuration changes, but does not modify the ISO.
