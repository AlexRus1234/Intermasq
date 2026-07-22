# tests/suites/23-hosts-csv.sh — CSV import path with count field (A6 regression).

if require_jwt "static hosts — CSV import (A6)" 2; then
    CSV=$(mktemp)
    cat > "$CSV" <<EOF
mac,ip,hostname
aa:bb:cc:dd:ee:10,10.0.0.20,csv1
aa:bb:cc:dd:ee:11,10.0.0.21,csv2
aa:bb:cc:dd:ee:12,10.0.0.22,csv3
EOF
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@$CSV" -F "target_file=$FILE" "$BASE/api/hosts/csv")
    check "CSV import 3 hosts" 200 "$S" || true
    CSV_COUNT=$(body | jval .count)
    check "A6: CSV response has count=3" 3 "$CSV_COUNT" A6 || true
    rm -f "$CSV"
fi
