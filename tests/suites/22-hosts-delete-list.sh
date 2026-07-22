# tests/suites/22-hosts-delete-list.sh — delete host, list hosts.

if require_jwt "static hosts — delete & list" 3; then
    S=$(DELETE "$JWT" "/api/hosts/aa:bb:cc:dd:ee:01?file=$FILE")
    check "Delete host by MAC" 200 "$S" || true

    S=$(DELETE "$JWT" "/api/hosts/aa:bb:cc:dd:ee:01?file=$FILE")
    check "Delete again → 404" 404 "$S" || true

    S=$(GET "$JWT" "/api/hosts")
    check "GET /api/hosts" 200 "$S" || true
    HOST_COUNT=$(body | jq 'length' 2>/dev/null || echo "?")
    echo "  current host count: $HOST_COUNT"
fi
