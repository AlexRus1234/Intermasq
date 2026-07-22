# tests/suites/25-hosts-csv-export.sh — CSV export endpoint.

if require_jwt "static hosts — CSV export" 1; then
    S=$(GET "$JWT" "/api/hosts/csv")
    check "CSV export" 200 "$S" || true
    EXPORT_LINES=$(body | wc -l)
    echo "  csv export lines: $EXPORT_LINES"
fi
