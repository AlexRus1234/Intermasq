# tests/suites/25-hosts-csv-export.sh — CSV export endpoint.

if require_jwt "static hosts — CSV export" 1; then
    S=$(GET "$JWT" "/api/hosts/csv")
    check "CSV export" 200 "$S" || true
    # P2.1: CSV is plain text, not JSON — guard with wc -l via check_lines
    # (>= comparison; check itself is exact-equality and wrong here). At
    # least header + 1 data row. By now (suites 20/22/23 ran) there are
    # >=5 static hosts, so >=2 lines is the robust non-empty guard.
    check_lines "CSV export has header + >=1 data row" 2
fi
