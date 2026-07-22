# tests/suites/24-hosts-bulk.sh — bulk-add hosts via JSON, count regression (A6).

if require_jwt "static hosts — bulk add (no count regression)" 2; then
    S=$(POST "$JWT" "/api/hosts/bulk" "{\"file\":\"$FILE\",\"hosts\":[{\"mac\":\"aa:bb:cc:dd:ee:20\",\"ip\":\"10.0.0.30\",\"hostname\":\"bulk1\"},{\"mac\":\"aa:bb:cc:dd:ee:21\",\"ip\":\"10.0.0.31\",\"hostname\":\"bulk2\"}]}")
    check "Bulk add 2 hosts → 200" 200 "$S" || true
    BULK_COUNT=$(body | jval .count)
    check "Bulk JSON response has count field" 2 "$BULK_COUNT" A6 || true
fi
