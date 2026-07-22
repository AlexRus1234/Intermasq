# tests/suites/34-aliases-csv.sh — GET /api/aliases/csv + POST /api/aliases/csv.
# Validates: export non-empty, import 3 fresh aliases with count, missing file.

if require_jwt "DNS aliases — CSV import/export" 4; then
    # Export current aliases — should return 200 + at least the header line.
    S=$(GET "$JWT" "/api/aliases/csv")
    check "Alias CSV export → 200" 200 "$S" || true
    EXPORT_LINES=$(body | wc -l)
    echo "  alias csv export lines: $EXPORT_LINES"

    # Import a hand-crafted CSV with fresh domains into a fresh file
    # (avoids duplicate conflicts with existing aliases).
    CSV=$(mktemp)
    cat > "$CSV" <<EOF
type,domain,target
A,csv-imptest-1.local,10.9.0.1
A,csv-imptest-2.local,10.9.0.2
CNAME,csv-imptest-3.local,csv-imptest-1.local
EOF
    FRESH_FILE="$CONF_DIR/35-imported-aliases.conf"
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "file=@$CSV" -F "target_file=$FRESH_FILE" "$BASE/api/aliases/csv")
    check "Alias CSV import 3 → 200" 200 "$S" || true
    IMPORT_COUNT=$(body | jval .count)
    check "Alias CSV import count=3" 3 "$IMPORT_COUNT" || true

    # Error: missing file field entirely.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $JWT" -F "target_file=$FRESH_FILE" "$BASE/api/aliases/csv")
    check "Alias CSV import no file → 400" 400 "$S" || true

    rm -f "$CSV"
fi
