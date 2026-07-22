# tests/suites/92-discovery.sh — read-only discovery endpoints.
# /api/leases, /api/arp, /api/new-devices, /api/hosts/next-ip.
# These endpoints have no side effects; we only assert 200 + JSON shape.

if require_jwt "discovery endpoints" 5; then
    # /api/leases — likely empty in CI (no /tmp/leases file created).
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
    S=$(GET "$JWT" "/api/new-devices")
    check "GET /api/new-devices → 200" 200 "$S" || true
    DEV_COUNT=$(body | jq 'length' 2>/dev/null || echo "?")
    echo "  new devices: $DEV_COUNT"

    # /api/hosts/next-ip — returns a free IP from the given CIDR.
    S=$(GET "$JWT" "/api/hosts/next-ip?range=10.99.0.0/24")
    check "GET /api/hosts/next-ip → 200" 200 "$S" || true
    NEXT_IP=$(body | jval .ip)
    echo "  next free ip: $NEXT_IP"
fi
