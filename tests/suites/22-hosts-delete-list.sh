# tests/suites/22-hosts-delete-list.sh — delete host, list hosts.

if require_jwt "static hosts — delete & list" 3; then
    S=$(DELETE "$JWT" "/api/hosts/aa:bb:cc:dd:ee:01?file=$FILE")
    check "Delete host by MAC" 200 "$S" || true

    S=$(DELETE "$JWT" "/api/hosts/aa:bb:cc:dd:ee:01?file=$FILE")
    check "Delete again → 404" 404 "$S" || true

    S=$(GET "$JWT" "/api/hosts")
    check "GET /api/hosts" 200 "$S" || true
    # P2.1: length-assert — getHostsHandler returning [] with 200 (e.g. a
    # read/parse regression) must fail here. By this point suites 20+21 have
    # left >=3 hosts (ee:03/04/05; ee:01 just deleted above). >=1 is the
    # robust non-empty guard.
    check_length "GET /api/hosts body non-empty" 1
fi
