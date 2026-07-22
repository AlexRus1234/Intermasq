# tests/suites/32-aliases-delete.sh — delete A + second-delete (A2-dependent).

if require_jwt "DNS aliases — delete" 3; then
    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"A\",\"domain\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete A record" 200 "$S" || true

    # Second delete: depends on A2 being fixed. While A2 allows duplicates,
    # there are 2 nas.local A records in file, so second delete finds the
    # other one and returns 200 instead of 404. Mark as KNOWN(A2) — will
    # become a clean pass once A2 is fixed.
    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"A\",\"domain\":\"nas.local\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete again → 404 (depends on A2 fix)" 404 "$S" A2 || true

    S=$(POST "$JWT" "/api/aliases/delete" "{\"type\":\"PTR\",\"domain\":\"5.0.0.10.in-addr.arpa\",\"file\":\"$ALIAS_FILE\"}")
    check "Delete PTR rejected (UI only supports A/CNAME)" 400 "$S" || true
fi
