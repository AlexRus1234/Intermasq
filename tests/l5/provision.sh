#!/bin/bash
# tests/l5/provision.sh — idempotent L5 setup на persistent ВМ.
# Гоняется OVER SSH AS ROOT. Авто-detect systemd/openrc.
#
# Поднимает ДВА инстанса intermasq:
#   • intermasq        (root,   port 18081) — root-путь  (UseSudo=false)
#   • intermasq-rootless (user intermasq + sudoers, port 18082) — sudo-путь (UseSudo=true)
# плюс nftables (изоляция br-l5, API только с lo) и изолированный dnsmasq на br-l5.
set -euo pipefail

: "${INTERMASQ_SECRET:?must be set in env}"
BIN=/usr/local/bin/intermasq-ci
CONF_ROOT=/etc/intermasq/conf
CONF_RW=/var/lib/intermasq-rw/conf
L5_CONF=/etc/dnsmasq.d/l5.conf
LOG=/var/log/intermasq.log
LOG_RW=/var/log/intermasq-rw.log
log(){ printf '[provision] %s\n' "$*"; }

detect_init() {
    if [ "$(cat /proc/1/comm)" = "systemd" ] || command -v systemctl >/dev/null 2>&1; then echo systemd
    elif command -v rc-service >/dev/null 2>&1; then echo openrc
    else echo "FATAL: neither systemd nor openrc"; exit 3; fi
}
INIT="$(detect_init)";  log "init: $INIT"

# Если runner scp'нул свежий бинарник в /tmp/intermasq-ci.upload — ставим его.
# Нельзя перезаписать исполняемый файл (ETXTBSY): стопаем сервисы, потом mv
# (атомарный rename — старый inode остаётся живому процессу, новый открываем).
if [ -f /tmp/intermasq-ci.upload ]; then
    log "installing new binary from /tmp/intermasq-ci.upload"
    if [ "$INIT" = systemd ]; then
        systemctl stop intermasq intermasq-rootless 2>/dev/null || true
    else
        rc-service intermasq stop 2>/dev/null || true
        rc-service intermasq-rootless stop 2>/dev/null || true
    fi
    mv -f /tmp/intermasq-ci.upload "$BIN"
    chmod +x "$BIN"
fi

[ -x "$BIN" ] || { log "FATAL: $BIN missing (runner scp first)"; exit 4; }

# ── 1. packages ──────────────────────────────────────────────────────────
case "$INIT" in
    systemd) command -v dnsmasq >/dev/null || pacman -Sy --noconfirm --needed dnsmasq
             command -v dig    >/dev/null || pacman -S  --noconfirm --needed bind-tools
             command -v nft    >/dev/null || pacman -S  --noconfirm --needed nftables
             ;;
    openrc)  command -v dnsmasq >/dev/null || apk add --no-cache dnsmasq
             command -v dig    >/dev/null || apk add --no-cache bind-tools
             command -v nft    >/dev/null || apk add --no-cache nftables
             command -v bash   >/dev/null || apk add --no-cache bash
             command -v ip     >/dev/null || apk add --no-cache iproute2
             ;;
esac

# ── 2. intermasq system user (для rootless-инстанса) ─────────────────────
if ! id intermasq >/dev/null 2>&1; then
    case "$INIT" in
        systemd) useradd -r -s /usr/sbin/nologin intermasq ;;
        openrc)  adduser -S -H -s /sbin/nologin -D intermasq ;;
    esac
    log "created user intermasq"
fi

# ── 3. isolated bridge br-l5 ─────────────────────────────────────────────
mkdir -p /etc/dnsmasq.d /etc/systemd/network 2>/dev/null || true
if [ "$INIT" = systemd ]; then
    cat > /etc/systemd/network/20-br-l5.netdev <<'EOF'
[NetDev]
Name=br-l5
Kind=bridge
EOF
    cat > /etc/systemd/network/20-br-l5.network <<'EOF'
[Match]
Name=br-l5

[Network]
Address=10.5.0.1/24
ConfigureWithoutCarrier=yes
LinkLocalAddressing=no
DHCP=no
IPForward=no
EOF
    systemctl enable --now systemd-networkd >/dev/null 2>&1 || true
    networkctl reload >/dev/null 2>&1 || systemctl restart systemd-networkd || true
else
    cat > /etc/init.d/br-l5 <<'EOF'
#!/sbin/openrc-run
name="br-l5"; description="Isolated bridge for L5 tests"
depend() { after net; before dnsmasq; before intermasq; before intermasq-rootless; }
start() { ebegin "Creating br-l5"; ip link add name br-l5 type bridge 2>/dev/null || true
          ip addr replace 10.5.0.1/24 dev br-l5; ip link set br-l5 up; eend $?; }
stop()  { ebegin "Removing br-l5"; ip link del br-l5 2>/dev/null; eend 0; }
EOF
    chmod +x /etc/init.d/br-l5
    rc-update add br-l5 default >/dev/null 2>&1 || true
    rc-service br-l5 restart >/dev/null 2>&1 || rc-service br-l5 start >/dev/null 2>&1 || true
fi
sleep 1
ip link replace br-l5 type bridge 2>/dev/null || true
ip addr replace 10.5.0.1/24 dev br-l5 2>/dev/null || true
ip link set br-l5 up 2>/dev/null || true

# ── 4. nftables (изоляция; API только с lo, dnsmasq только с br-l5) ───────
# ВАЖНО: сервис грузит РАЗНЫЕ файлы — systemd: /etc/nftables.conf,
# Alpine openrc: /etc/nftables.nft (init.d default ${rules_file}).
# Если написать не туда — на restart/ребуте вернётся дефолтный ruleset
# (Alpine: table inet filter, policy drop, БЕЗ SSH:22 → потеря доступа).
if [ "$INIT" = systemd ]; then NFT_FILE=/etc/nftables.conf
else                            NFT_FILE=/etc/nftables.nft; fi
EXT_IF=$(ip route show default 2>/dev/null | awk '/default/ {print $5; exit}')
[ -n "$EXT_IF" ] || EXT_IF=ens18
# Restrictive: policy drop + явные allow (lo/established/br-l5/SSH/icmp).
# Sensitive-порты на внешнем iface гасим явно (defense-in-depth; policy drop и так их режет).
cat > "$NFT_FILE" <<EOF
#!/usr/sbin/nft -f
flush ruleset

table inet l5 {
    chain input {
        type filter hook input priority filter; policy drop;
        iifname "lo" accept comment "loopback (intermasq API via 127.0.0.1)"
        ct state established,related accept
        ct state invalid drop
        iifname "br-l5" accept comment "isolated test bridge (dnsmasq/DHCP local)"
        tcp dport 22 accept comment "SSH (runner) — обязательно, иначелок"
        ip protocol icmp accept
        meta l4proto ipv6-icmp accept
        iifname "$EXT_IF" tcp dport { 18081, 18082 } drop comment "intermasq API: localhost only"
        iifname "$EXT_IF" udp dport { 53, 67 } drop comment "dnsmasq: br-l5 only"
        iifname "$EXT_IF" tcp dport 53 drop
    }
    chain forward { type filter hook forward priority filter; policy drop; }
    chain output  { type filter hook output  priority filter; policy accept; }
}
EOF
if [ "$INIT" = systemd ]; then
    systemctl enable nftables >/dev/null 2>&1 || true
    nft -f "$NFT_FILE" && log "nft applied ($NFT_FILE)"
    systemctl restart nftables >/dev/null 2>&1 || true
else
    rc-update add nftables default >/dev/null 2>&1 || true
    nft -f "$NFT_FILE" && log "nft applied ($NFT_FILE)"
    rc-service nftables restart >/dev/null 2>&1 || true
fi

# ── 5. dnsmasq: изолированный конф + conf-dir chain ──────────────────────
mkdir -p "$CONF_ROOT" "$CONF_RW"
cat > "$L5_CONF" <<'EOF'
# L5 isolated dnsmasq — managed by provision.sh, DO NOT EDIT.
interface=br-l5
bind-interfaces
dhcp-range=10.5.0.10,10.5.0.50,255.255.255.0,12h
domain=l5.test
local=/l5.test/
address=/probe.l5.test/10.5.0.9
no-resolv
conf-dir=/etc/intermasq/conf/,*.conf
EOF
if [ "$INIT" = systemd ]; then
    mkdir -p /etc/systemd/system/dnsmasq.service.d
    cat > /etc/systemd/system/dnsmasq.service.d/l5.conf <<'EOF'
[Service]
Type=simple
ExecStart=
ExecStart=/usr/bin/dnsmasq -k --user=dnsmasq --pid-file --conf-file=/etc/dnsmasq.d/l5.conf
EOF
    systemctl daemon-reload
    systemctl enable dnsmasq >/dev/null 2>&1 || true
    systemctl restart dnsmasq
else
    touch /etc/dnsmasq.conf
    grep -q '^conf-dir=/etc/dnsmasq.d' /etc/dnsmasq.conf \
        || echo 'conf-dir=/etc/dnsmasq.d/,*.conf' >> /etc/dnsmasq.conf
    rc-update add dnsmasq default >/dev/null 2>&1 || true
    rc-service dnsmasq restart
fi

# ── 6. sudoers для rootless-инстанса (пути резолвим из реального $PATH) ──
# caller в system.go зовёт: sudo <bin> {is-active|restart} dnsmasq | restart intermasq
if [ "$INIT" = systemd ]; then
    SCT=$(command -v systemctl)
    cat > /etc/sudoers.d/intermasq <<EOF
# L5 rootless intermasq — NOPASSWD только для нужных init-команд.
intermasq ALL=(root) NOPASSWD: $SCT is-active dnsmasq, $SCT restart dnsmasq, $SCT restart intermasq
EOF
else
    RCS=$(command -v rc-service)
    cat > /etc/sudoers.d/intermasq <<EOF
intermasq ALL=(root) NOPASSWD: $RCS dnsmasq status, $RCS dnsmasq restart, $RCS intermasq restart
EOF
fi
chmod 440 /etc/sudoers.d/intermasq
visudo -cf /etc/sudoers.d/intermasq >/dev/null

# ── 7. intermasq: секрет + ДВА инстанса ──────────────────────────────────
mkdir -p /etc/intermasq/history /var/lib/intermasq-rw/history "$CONF_ROOT" "$CONF_RW"
umask 077
printf 'INTERMASQ_SECRET=%s\n' "$INTERMASQ_SECRET" > /etc/intermasq/intermasq.env
umask 022
# rootless-инстанс пишет в свои dirs под /var/lib/intermasq-rw → chown intermasq
chown -R intermasq:intermasq /var/lib/intermasq-rw
: > /var/lib/intermasq-rw/leases; chown intermasq:intermasq /var/lib/intermasq-rw/leases
# per-run clean (no snapshot)
rm -f /etc/intermasq/users.json /var/lib/intermasq-rw/users.json

if [ "$INIT" = systemd ]; then
    # --- root instance (canonical name "intermasq" → RestartSelf работает) ---
    cat > /etc/systemd/system/intermasq.service <<'EOF'
[Unit]
Description=Intermasq (CI L5, root)
After=network-online.target dnsmasq.service
Wants=network-online.target
[Service]
Type=simple
User=root
EnvironmentFile=/etc/intermasq/intermasq.env
ExecStart=/usr/local/bin/intermasq-ci -port 18081 -conf-dir /etc/intermasq/conf -db /etc/intermasq/users.json -history-dir /etc/intermasq/history -leases /var/lib/misc/dnsmasq.leases -arp-file /proc/net/arp -init-system=auto
Restart=on-failure
RestartSec=2
StandardOutput=append:/var/log/intermasq.log
StandardError=append:/var/log/intermasq.log
[Install]
WantedBy=multi-user.target
EOF
    # --- rootless instance (user intermasq, sudo path) ---
    cat > /etc/systemd/system/intermasq-rootless.service <<'EOF'
[Unit]
Description=Intermasq (CI L5, rootless/sudo)
After=network-online.target dnsmasq.service
Wants=network-online.target
[Service]
Type=simple
User=intermasq
Group=intermasq
EnvironmentFile=/etc/intermasq/intermasq.env
ExecStart=/usr/local/bin/intermasq-ci -port 18082 -conf-dir /var/lib/intermasq-rw/conf -db /var/lib/intermasq-rw/users.json -history-dir /var/lib/intermasq-rw/history -leases /var/lib/intermasq-rw/leases -arp-file /proc/net/arp -init-system=auto
Restart=on-failure
RestartSec=2
StandardOutput=append:/var/log/intermasq-rw.log
StandardError=append:/var/log/intermasq-rw.log
[Install]
WantedBy=multi-user.target
EOF
    : > "$LOG"; : > "$LOG_RW"; chown intermasq:intermasq "$LOG_RW" 2>/dev/null || true
    systemctl daemon-reload
    systemctl enable intermasq intermasq-rootless >/dev/null 2>&1 || true
    systemctl restart intermasq-rootless
    systemctl restart intermasq
else
    # openrc env (sourced per-svc: /etc/conf.d/<svcname>)
    umask 077
    printf 'export INTERMASQ_SECRET=%s\n' "$INTERMASQ_SECRET" > /etc/conf.d/intermasq
    printf 'export INTERMASQ_SECRET=%s\n' "$INTERMASQ_SECRET" > /etc/conf.d/intermasq-rootless
    umask 022
    cat > /etc/init.d/intermasq <<'EOF'
#!/sbin/openrc-run
name="intermasq"; description="Intermasq (CI L5, root)"
command="/usr/local/bin/intermasq-ci"
command_args="-port 18081 -conf-dir /etc/intermasq/conf -db /etc/intermasq/users.json -history-dir /etc/intermasq/history -leases /var/lib/misc/dnsmasq.leases -arp-file /proc/net/arp -init-system=auto"
command_background=true
pidfile="/run/${RC_SVCNAME}.pid"
output_log="/var/log/intermasq.log"
error_log="/var/log/intermasq.log"
depend() { need net; after br-l5 dnsmasq; }
EOF
    cat > /etc/init.d/intermasq-rootless <<'EOF'
#!/sbin/openrc-run
name="intermasq-rootless"; description="Intermasq (CI L5, rootless/sudo)"
command="/usr/local/bin/intermasq-ci"
command_args="-port 18082 -conf-dir /var/lib/intermasq-rw/conf -db /var/lib/intermasq-rw/users.json -history-dir /var/lib/intermasq-rw/history -leases /var/lib/intermasq-rw/leases -arp-file /proc/net/arp -init-system=auto"
command_user="intermasq:intermasq"
command_background=true
pidfile="/run/${RC_SVCNAME}.pid"
output_log="/var/log/intermasq-rw.log"
error_log="/var/log/intermasq-rw.log"
depend() { need net; after br-l5 dnsmasq; }
EOF
    chmod +x /etc/init.d/intermasq /etc/init.d/intermasq-rootless
    : > "$LOG"; : > "$LOG_RW"; chown intermasq:intermasq "$LOG_RW" 2>/dev/null || true
    rc-update add intermasq default >/dev/null 2>&1 || true
    rc-update add intermasq-rootless default >/dev/null 2>&1 || true
    rc-service intermasq-rootless restart >/dev/null 2>&1 || rc-service intermasq-rootless start
    rc-service intermasq restart >/dev/null 2>&1 || rc-service intermasq start
fi

# ── 8. wait for both APIs ────────────────────────────────────────────────
for P in 18081 18082; do
    for i in $(seq 1 15); do
        curl -s -m 2 "http://127.0.0.1:$P/api/status" >/dev/null 2>&1 && { log ":$P up after ${i}s"; break; }
        sleep 1
    done
done
log "done. INIT=$INIT; root:18081 detect=$(grep -m1 '\[INIT\]' "$LOG"|sed 's/.*System: //'); rootless:18082 detect=$(grep -m1 '\[INIT\]' "$LOG_RW"|sed 's/.*System: //')"
