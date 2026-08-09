# Intermasq - Web panel for dnsmasq
# Copyright (C) 2026 AlexRus1234
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU Affero General Public License for more details.
#
# You should have received a copy of the GNU Affero General Public License
# along with this program. If not, see <https://www.gnu.org/licenses/>.

# tests/suites/92-discovery.sh — read-only discovery endpoints.
# /api/leases, /api/arp, /api/new-devices, /api/hosts/next-ip.
# These endpoints have no side effects; we only assert 200 + JSON shape.

if require_jwt "discovery endpoints" 5; then
    # /api/leases — likely empty in CI (no /tmp/leases file created).
    # P2.1: known empty in CI (no real dnsmasq writing leases), so only the
    # 200 + JSON-array shape is asserted; a length-assert would be
    # meaningless here.
    S=$(GET "$JWT" "/api/leases")
    check "GET /api/leases → 200" 200 "$S" || true
    LEASE_COUNT=$(body | jq 'length' 2>/dev/null || echo "?")
    echo "  leases: $LEASE_COUNT"

    # /api/arp — object map of mac→bool from /proc/net/arp (or fixture).
    # CI runs with -arp-file tests/fixtures/arp-sample.txt, which has 4
    # valid MACs (the 00:00:... entry is filtered by parseArpContent).
    S=$(GET "$JWT" "/api/arp")
    check "GET /api/arp → 200" 200 "$S" || true
    ARP_COUNT=$(body | jq 'length' 2>/dev/null || echo "?")
    echo "  arp entries: $ARP_COUNT"
    if [ "${ARP_COUNT:-0}" -ge 4 ]; then
        check "ARP table has expected 4+ entries" 0 0 || true
    else
        check "ARP table has expected 4+ entries" 0 1 || true
    fi

    # /api/new-devices — macs in ARP not in static hosts or active leases.
    # P2.1: env-dependent — requires the CI arp fixture
    # (-arp-file=tests/fixtures/arp-sample.txt). With it, >=1 new device is
    # expected; without it (local run) the list is correctly empty, so we
    # only assert 200 + shape and keep the count informational.
    S=$(GET "$JWT" "/api/new-devices")
    check "GET /api/new-devices → 200" 200 "$S" || true
    DEV_COUNT=$(body | jq 'length' 2>/dev/null || echo "?")
    echo "  new devices: $DEV_COUNT"

    # /api/hosts/next-ip — returns a free IP from the given CIDR.
    # P2.1: deterministic given the CIDR — assert the ip field is non-empty
    # (catches a regression returning {} or {"ip":""} with 200).
    S=$(GET "$JWT" "/api/hosts/next-ip?range=10.99.0.0/24")
    check "GET /api/hosts/next-ip → 200" 200 "$S" || true
    NEXT_IP=$(body | jval .ip)
    echo "  next free ip: $NEXT_IP"
    if [ -n "$NEXT_IP" ] && [ "$NEXT_IP" != "null" ]; then
        check "next-ip returns non-empty ip" 1 1 || true
    else
        check "next-ip returns non-empty ip" 1 0 || true
    fi
fi
